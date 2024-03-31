MAKEFLAGS += --warn-undefined-variables
SHELL = /bin/bash -o nounset -o errexit -o pipefail
.DEFAULT_GOAL = build
BUILD_PATH  := $(patsubst %/,%,$(abspath $(dir $(lastword $(MAKEFILE_LIST)))))
PARENT_PATH := $(patsubst %/,%,$(dir $(BUILD_PATH)))
UNITY_PROJ := ${PARENT_PATH}/arcspace.unity-app
UNITY_PATH := $(shell python3 ${UNITY_PROJ}/arc-utils.py UNITY_PATH "${UNITY_PROJ}")
UNITY_ARC_LIBS = ${UNITY_PROJ}/Assets/Plugins/AMP/Plugins
ARC_UNITY_PATH = ${UNITY_PROJ}/Assets/AMP
LIB_PROJ := ${BUILD_PATH}/cmd/libarchost
OSX_OUT := ${UNITY_ARC_LIBS}/OSX

ANDROID_NDK := ${UNITY_PATH}/PlaybackEngines/AndroidPlayer/NDK
ANDROID_CC := ${ANDROID_NDK}/toolchains/llvm/prebuilt/darwin-x86_64/bin

ARC_SDK_PKG  :=github.com/arcspace/go-arc-sdk
ARC_SDK_PATH :=$(shell go list -m -f '{{.Dir}}' $(ARC_SDK_PKG))

## prints this message
help:
	@echo -e "\033[32m"
	@echo "go-archost"
	@echo "  PARENT_PATH:     ${PARENT_PATH}"
	@echo "  ARC_SDK_PATH:    ${ARC_SDK_PATH}"
	@echo "  BUILD_PATH:      ${BUILD_PATH}"
	@echo "  UNITY_PROJ:      ${UNITY_PROJ}"
	@echo "  UNITY_ARC_LIBS:  ${UNITY_ARC_LIBS}"
	@echo "  UNITY_PATH:      ${UNITY_PATH}"
	@echo "  ANDROID_NDK:     ${ANDROID_NDK}"
	@echo "  ANDROID_CC:      ${ANDROID_CC}"
	@echo
	@awk '/^##.*$$/,/[a-zA-Z_-]+:/' $(MAKEFILE_LIST) | awk '!(NR%2){print $$0p}{p=$$0}' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m  %-32s\033[0m %s\n", $$1, $$2}' | sort

# ----------------------------------------
# build

GOFILES = $(shell find . -type f -name '*.go')
	
.PHONY: build generate tools

## builds both archost & libarchost
build:  archost libarchost


#
# https://rogchap.com/2020/09/14/running-go-code-on-ios-and-android/
# TODO: use dynamic linking so that we don't have to restart Unity to load a new binary?
# https://github.com/vladimirvivien/go-cshared-examples  
#


## builds libarchost for OSX -- build on x86_64 mac for now
libarchost-osx:
# Beware of a Unity bug where *not* selecting "Any CPU" causes the app builder to not add the .dylib to the app bundle!
# Also note that a .dylib is identical to the binary in an OS X .bundle.  Also: https://stackoverflow.com/questions/2339679/what-are-the-differences-between-so-and-dylib-on-macos 
# Info on cross-compiling Go: https://freshman.tech/snippets/go/cross-compile-go-programs/
# Note: for the time being, we are currently x86_64 (amd64) only, so the archost.dylib should only be compiled on an x86_64 machine!
	OUT_DIR="${UNITY_ARC_LIBS}"     CC="${LIB_PROJ}/clangwrap.sh" \
	PLATFORM=OSX                    GOARCH=amd64        "${LIB_PROJ}/build.sh"
# arm64
	OUT_DIR="${UNITY_ARC_LIBS}"     CC="${LIB_PROJ}/clangwrap.sh" \
	PLATFORM=OSX                    GOARCH=arm64        "${LIB_PROJ}/build.sh"
# make fat binary
	makefat "${OSX_OUT}/archost.dylib" "${OSX_OUT}/archost.amd64.dylib" "${OSX_OUT}/archost.arm64.dylib"
	rm                                 "${OSX_OUT}/archost.amd64.dylib" "${OSX_OUT}/archost.arm64.dylib"


## builds libarchost for iOS -- build on x86_64 mac for now
libarchost-ios:
	OUT_DIR="${UNITY_ARC_LIBS}"     CC="${LIB_PROJ}/clangwrap.sh" \
	PLATFORM=iOS                    GOARCH=arm64        "${LIB_PROJ}/build.sh"

## builds libarchost for arm64-v8a
libarchost-android-arm64-v8a:
	OUT_DIR="${UNITY_ARC_LIBS}"     CC="${ANDROID_CC}/aarch64-linux-android27-clang" \
	PLATFORM=Android/arm64-v8a      GOARCH=arm64        "${LIB_PROJ}/build.sh"

## builds libarchost for armeabi-v7a 
libarchost-android-armeabi-v7a:
	OUT_DIR="${UNITY_ARC_LIBS}"     CC="${ANDROID_CC}/armv7a-linux-androideabi27-clang" \
	PLATFORM=Android/armeabi-v7a    GOARCH=arm          "${LIB_PROJ}/build.sh"

## builds libarchost for armeabi-x86_64
libarchost-android-x86_64_:
	OUT_DIR="${UNITY_ARC_LIBS}"     CC="${ANDROID_CC}/x86_64-linux-android27-clang" \
	PLATFORM=Android/x86_64         GOARCH=amd64        "${LIB_PROJ}/build.sh"


## builds libarchost for all platforms for embeddeding in a Unity or Unreal project
libarchost:  libarchost-osx libarchost-ios libarchost-android-arm64-v8a libarchost-android-armeabi-v7a libarchost-android-x86_64_


## builds archost headless executable
archost:
	cd cmd/archost && touch main.go \
	&& go build -trimpath .




## generate .cs and .go files from .proto
generate:
#   download protoc: https://github.com/protocolbuffers/protobuf/releases
	protoc \
	    -I"${ARC_SDK_PATH}/apis" \
	    --gogoslick_out=plugins:. --gogoslick_opt=paths=source_relative \
	    --csharp_out "${ARC_UNITY_PATH}/amp.sheet.av/" \
	    --proto_path=. \
		apps/av/av.proto
	
	protoc \
	    --gogoslick_out=plugins:. --gogoslick_opt=paths=source_relative \
	    --proto_path=. \
		ski/api.ski.proto
	



## builds fmod playback toy (experiment)
play:
#   https://stackoverflow.com/questions/75666660/how-can-i-specify-a-relative-dylib-path-in-cgo-on-macos
	cd cmd/play && touch main.go \
	&& go build -trimpath . \
	&& install_name_tool -change @rpath/libfmod.dylib @executable_path/libfmod.dylib play
	&& cd cmd/play \
	&& ./play


setup:
	go install  github.com/randall77/makefat 