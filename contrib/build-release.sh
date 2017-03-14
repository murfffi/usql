#!/bin/bash

VER=$1
BUILD=$2

if [ -z "$VER" ]; then
  echo "usage: $0 <VER>"
  exit 1
fi

PLATFORM=$(uname|sed -e 's/_.*//'|tr '[:upper:]' '[:lower:]')

TAG=v$VER
SRC=$(realpath $(cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/../)
NAME=$(basename $SRC)

case $PLATFORM in
  mingw64)
    PLATFORM=windows
  ;;
  msys)
    PLATFORM=windows
  ;;
esac

if [ -z "$BUILD" ]; then
  BUILD=$SRC/build
  if [ "$PLATFORM" == "windows" ]; then
    BUILD=$HOME/$NAME
  fi
fi

EXT=tar
if [ "$PLATFORM" == "windows" ]; then
  EXT=zip
fi

DIR=$BUILD/$PLATFORM/$VER
BIN=$DIR/$NAME
OUT=$DIR/usql-$VER-$PLATFORM-amd64.$EXT

rm -rf $DIR
mkdir -p $DIR

if [ "$PLATFORM" == "windows" ]; then
    BIN=$BIN.exe
fi

echo "PLATFORM: $PLATFORM"
echo "VER: $VER"
echo "DIR: $DIR"
echo "BIN: $BIN"
echo "OUT: $OUT"

set -e

pushd $SRC &> /dev/null

go build -ldflags="-X main.name=$NAME -X main.version=$VER" -o $BIN

if [ "$PLATFORM" == "linux" ]; then
  echo "stripping $BIN"
  strip $BIN

  echo "packing $BIN"
  upx -q -q $BIN
fi

echo "compressing $OUT"
case $EXT in
  zip)
    zip $OUT -j $BIN
  ;;
  tar)
    tar -C $DIR -cjvf $OUT.bz2 $(basename $BIN)
  ;;
esac

popd &> /dev/null