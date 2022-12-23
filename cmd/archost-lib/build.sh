#!/bin/sh

# build.sh

rm -rf tmp ||: && mkdir tmp

NAME="archost"
export CGO_ENABLED=1
export BUILDMODE="c-shared"
VERIFY="stat -l "
GO_ARCHOST_LIB="./cmd/archost-lib"

if [[ $PLATFORM =~ ^Android ]]; then
    export GOOS=android
    NAME="${NAME}.so"
fi


if [ $PLATFORM == iOS ]; then
    export GOOS=ios
    export SDK=iphoneos
    export BUILDMODE="c-archive"
    NAME="${NAME}.a"
    VERIFY="otool -hv "
elif [ $PLATFORM == OSX ]; then
    export GOOS=darwin
    export SDK=macosx
    NAME="${NAME}.dylib"
    VERIFY="otool -hv "
elif [ $PLATFORM == Android/armeabi-v7a ]; then
    export GOARM=7
fi

# make sure the compiler doesn't say notihing new to do
touch ${GO_ARCHOST_LIB}/main.go

rm -f              "${OUT_DIR}/${PLATFORM}/${NAME}" || true

CGO_CFLAGS="-fembed-bitcode" \
go build -buildmode=${BUILDMODE} -o tmp/archost.bin ${GO_ARCHOST_LIB}

mv tmp/archost.bin "${OUT_DIR}/${PLATFORM}/${NAME}"
$VERIFY            "${OUT_DIR}/${PLATFORM}/${NAME}"

rm -rf tmp
