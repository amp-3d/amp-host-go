#!/bin/sh

# build.sh

rm -rf tmp ||: && mkdir tmp

if [ "$PLATFORM" == "iOS" ]; then
    SDK="iphoneos"
    GOOS="darwin"
elif [ "$PLATFORM" == "OSX" ]; then
    SDK="macosx"
    GOOS="darwin"
fi

CGO_ENABLED=1 \
SDK=${SDK} \
GOARCH=${GOARCH} \
GOOS=${GOOS} \
CC=/Users/aomeara/git.arcspace/go-arcspace/cmd/archost-lib/clangwrap.sh \
CGO_CFLAGS="-fembed-bitcode" \
go build -buildmode=c-shared -o tmp/archost.dylib ./cmd/archost-lib

mv tmp/archost.dylib "${OUT_DIR}/${PLATFORM}/archost.dylib"
otool -hv            "${OUT_DIR}/${PLATFORM}/archost.dylib"

rm -rf tmp