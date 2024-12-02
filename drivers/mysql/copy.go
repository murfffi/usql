package mysql

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/xo/usql/drivers"
)

func copyRows(ctx context.Context, db *sql.DB, rows *sql.Rows, table string) (int64, error) {
	localInfileSupported := false
	row := db.QueryRowContext(ctx, "SELECT @@GLOBAL.local_infile")
	err := row.Scan(&localInfileSupported)
	if err == nil && localInfileSupported && !hasBlobColumn(rows) {
		return bulkCopy(ctx, db, rows, table)
	} else {
		return drivers.CopyWithInsert(func(int) string { return "?" })(ctx, db, rows, table)
	}
}

func bulkCopy(ctx context.Context, db *sql.DB, rows *sql.Rows, table string) (int64, error) {
	mysql.RegisterReaderHandler("data", func() io.Reader {
		return toCsvReader(rows)
	})
	defer mysql.DeregisterReaderHandler("data")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	var cnt int64
	csvSpec := " FIELDS TERMINATED BY ',' "
	stmt := fmt.Sprintf("LOAD DATA LOCAL INFILE 'Reader::data' INTO TABLE %s",
		// if there is a column list, csvSpec goes between the table name and the list
		strings.Replace(table, "(", csvSpec+" (", 1))
	// if there wasn't a column list in the table spec, csvSpec goes at the end
	if !strings.Contains(table, "(") {
		stmt += csvSpec
	}
	res, err := tx.ExecContext(ctx, stmt)
	if err != nil {
		tx.Rollback()
	} else {
		err = tx.Commit()
		if err == nil {
			cnt, err = res.RowsAffected()
		}
	}
	return cnt, err
}

func hasBlobColumn(rows *sql.Rows) bool {
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return false
	}
	for _, ct := range columnTypes {
		if ct.DatabaseTypeName() == "BLOB" {
			return true
		}
	}
	return false
}

func toCsvReader(rows *sql.Rows) io.Reader {
	r, w := io.Pipe()
	go writeAsCsv(rows, w)
	return r
}

// writeAsCsv writes the rows in a CSV format compatible with LOAD DATA INFILE
func writeAsCsv(rows *sql.Rows, w *io.PipeWriter) {
	defer w.Close() // noop if already closed
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		w.CloseWithError(err)
		return
	}
	values := make([]interface{}, len(columnTypes))
	valueRefs := make([]reflect.Value, len(columnTypes))
	for i := 0; i < len(columnTypes); i++ {
		valueRefs[i] = reflect.New(columnTypes[i].ScanType())
		values[i] = valueRefs[i].Interface()
	}
	record := make([]string, len(values))
	csvWriter := csv.NewWriter(w)
	for rows.Next() {
		if err = rows.Err(); err != nil {
			break
		}
		err = rows.Scan(values...)
		if err != nil {
			break
		}
		for i, valueRef := range valueRefs {
			// NB: Does not work for BLOBs. Use regular copy if there are BLOB columns
			record[i] = fmt.Sprintf("%v", valueRef.Elem().Interface())
		}
		err = csvWriter.Write(record)
		if err != nil {
			break
		}
	}
	if err == nil {
		csvWriter.Flush()
		err = csvWriter.Error()
	}
	w.CloseWithError(err) // same as w.Close(), if err is nil
}
