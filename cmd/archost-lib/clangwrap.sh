#!/bin/sh

# clangwrap.sh

SDK_PATH=`xcrun --sdk $SDK --show-sdk-path`
CLANG=`xcrun --sdk $SDK --find clang`

if [ "$SDK" == "iphoneos" ]; then
    SDK_ARGS=" -mios-version-min=12.0 "
elif [ "$SDK" == "macosx" ]; then
    SDK_ARGS=" -mmacosx-version-min=10.15 "
fi

if [ "$GOARCH" == "amd64" ]; then
    CARCH="x86_64"
elif [ "$GOARCH" == "arm64" ]; then
    CARCH="arm64"
fi

exec $CLANG -arch $CARCH -isysroot $SDK_PATH $SDK_ARGS "$@"
