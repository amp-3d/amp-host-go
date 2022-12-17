#!/bin/sh

# build.sh

rm -rf tmp ||: && mkdir tmp

NAME="archost"

if [ "$PLATFORM" == "iOS" ]; then
    SDK="iphoneos"
    GOOS="darwin"
    BUILDMODE="c-archive"
    NAME="${NAME}.a"
elif [ "$PLATFORM" == "OSX" ]; then
    SDK="macosx"
    GOOS="darwin"
    BUILDMODE="c-shared"
    NAME="${NAME}.dylib"
fi

CGO_ENABLED=1 \
SDK=${SDK} \
GOARCH=${GOARCH} \
GOOS=${GOOS} \
CC=/Users/aomeara/git.arcspace/go-arcspace/cmd/archost-lib/clangwrap.sh \
CGO_CFLAGS="-fembed-bitcode" \
go build -buildmode=${BUILDMODE} -o tmp/archost.bin ./cmd/archost-lib

mv tmp/archost.bin "${OUT_DIR}/${PLATFORM}/${NAME}"
otool -hv          "${OUT_DIR}/${PLATFORM}/${NAME}"

rm -rf tmp