MAKEFLAGS += --warn-undefined-variables
SHELL = /bin/bash -o nounset -o errexit -o pipefail
.DEFAULT_GOAL = build
BUILD_PATH  := $(patsubst %/,%,$(abspath $(dir $(lastword $(MAKEFILE_LIST)))))
PARENT_PATH := $(patsubst %/,%,$(dir $(BUILD_PATH)))
UNITY_PROJ := ${PARENT_PATH}/arcspace.unity-app
ARC_LIBS = ${UNITY_PROJ}/Assets/Plugins/Arcspace/Plugins
ARC_UNITY_PATH = ${UNITY_PROJ}/Assets/Arcspace
grpc_csharp_exe="${GOPATH}/bin/grpc_csharp_plugin"
LIB_PROJ := ${BUILD_PATH}/cmd/archost-lib

#UNITY_PATH = "${HOME}/Applications/2021.3.16f1"
UNITY_PATH := $(shell python3 ${UNITY_PROJ}/arc-utils.py UNITY_PATH "${UNITY_PROJ}")

ANDROID_NDK := ${UNITY_PATH}/PlaybackEngines/AndroidPlayer/NDK
ANDROID_CC := ${ANDROID_NDK}/toolchains/llvm/prebuilt/darwin-x86_64/bin


## display this help message
help:
	@echo -e "\033[32m"
	@echo "go-arcspace"
	@echo "  PARENT_PATH:     ${PARENT_PATH}"
	@echo "  BUILD_PATH:      ${BUILD_PATH}"
	@echo "  UNITY_PROJ:      ${UNITY_PROJ}"
	@echo "  ARC_LIBS:        ${ARC_LIBS}"
	@echo "  UNITY_PATH:      ${UNITY_PATH}"
	@echo "  ANDROID_NDK:     ${ANDROID_NDK}"
	@echo "  ANDROID_CC:      ${ANDROID_CC}"
	@echo
	@awk '/^##.*$$/,/[a-zA-Z_-]+:/' $(MAKEFILE_LIST) | awk '!(NR%2){print $$0p}{p=$$0}' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m  %-32s\033[0m %s\n", $$1, $$2}' | sort

# ----------------------------------------
# build

GOFILES = $(shell find . -type f -name '*.go')
	
.PHONY: build protos tools

## build archost and archost-lib
build:  archost archost-lib


#
# https://rogchap.com/2020/09/14/running-go-code-on-ios-and-android/
# TODO: use dynamic linking so that we don't have to restart Unity to load a new binary?
# https://github.com/vladimirvivien/go-cshared-examples  
#


## build archost.dylib for OSX (build on x86_64 mac)
archost-lib-osx:
# Beware of a Unity bug where *not* selecting "Any CPU" causes the app builder to not add the .dylib to the app bundle!
# Also note that a .dylib is identical to the binary in an OS X .bundle.  Also: https://stackoverflow.com/questions/2339679/what-are-the-differences-between-so-and-dylib-on-macos 
# Info on cross-compiling Go: https://freshman.tech/snippets/go/cross-compile-go-programs/
# Note: for the time being, we are currently x86_64 (amd64) only, so the archost.dylib should only be compiled on an x86_64 machine!
	OUT_DIR="${ARC_LIBS}"           CC="${LIB_PROJ}/clangwrap.sh" \
	PLATFORM=OSX                    GOARCH=amd64        "${LIB_PROJ}/build.sh"

## build archost.a for iOS (build on x86_64 mac)
archost-lib-ios:
	OUT_DIR="${ARC_LIBS}"           CC="${LIB_PROJ}/clangwrap.sh" \
	PLATFORM=iOS                    GOARCH=arm64        "${LIB_PROJ}/build.sh"


## build archost-lib for arm64-v8a
archost-lib-android-arm64-v8a:
	OUT_DIR="${ARC_LIBS}"           CC="${ANDROID_CC}/aarch64-linux-android27-clang" \
	PLATFORM=Android/arm64-v8a      GOARCH=arm64        "${LIB_PROJ}/build.sh"

## build archost-lib for armeabi-v7a 
archost-lib-android-armeabi-v7a:
	OUT_DIR="${ARC_LIBS}"           CC="${ANDROID_CC}/armv7a-linux-androideabi27-clang" \
	PLATFORM=Android/armeabi-v7a    GOARCH=arm          "${LIB_PROJ}/build.sh"
		
## build archost-lib for armeabi-v7a 
archost-lib-android-x86_64:
	OUT_DIR="${ARC_LIBS}"           CC="${ANDROID_CC}/x86_64-linux-android27-clang" \
	PLATFORM=Android/x86_64         GOARCH=amd64        "${LIB_PROJ}/build.sh"


## build archost.dylib/so/.a for all platforms
archost-lib:  archost-lib-osx archost-lib-ios archost-lib-android-arm64-v8a archost-lib-android-armeabi-v7a archost-lib-android-x86_64


## build archost "headless" daemon
archost:
	cd cmd/archost && touch main.go && \
	go build -trimpath .

	
## install tools
tools:
	go install github.com/gogo/protobuf/protoc-gen-gogoslick
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go get -d  github.com/gogo/protobuf/proto


## generate .cs and .go from proto files
protos:
#   GrpcTools (2.49.1)
#   Install protoc & grpc_csharp_plugin:
#      - Download latest Grpc.Tools from https://nuget.org/packages/Grpc.Tools
#      - Extract .nupkg as .zip, move protoc and grpc_csharp_plugin to ${GOPATH}/bin 
#   Or, just protoc: https://github.com/protocolbuffers/protobuf/releases
#   Links: https://grpc.io/docs/languages/csharp/quickstart/
	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --csharp_out "${ARC_UNITY_PATH}/Arc" \
	    --grpc_out   "${ARC_UNITY_PATH}/Arc" \
	    --plugin=protoc-gen-grpc="${grpc_csharp_exe}" \
	    --proto_path=. \
		arc/arc.proto

	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --csharp_out "${ARC_UNITY_PATH}/Crates" \
	    --proto_path=. \
		crates/crates.proto

	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --csharp_out "${ARC_UNITY_PATH}/Arc/Apps/amp" \
	    --proto_path=. \
		arc/apps/amp/api/amp.proto
				
	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --proto_path=. \
		ski/api.ski.proto


## build fmod play toy
play:
#   https://stackoverflow.com/questions/75666660/how-can-i-specify-a-relative-dylib-path-in-cgo-on-macos
	cd cmd/play && touch main.go && \
	go build -trimpath . && \
	install_name_tool -change @rpath/libfmod.dylib @executable_path/libfmod.dylib play
	cd cmd/play && ./play

