MAKEFLAGS += --warn-undefined-variables
SHELL = /bin/bash -o nounset -o errexit -o pipefail
.DEFAULT_GOAL = build
BUILD_DIR  := $(patsubst %/,%,$(abspath $(dir $(lastword $(MAKEFILE_LIST)))))
PARENT_DIR := $(patsubst %/,%,$(dir $(BUILD_DIR)))
UNITY_ASSETS_DIR = ${PARENT_DIR}/arcspace.unity-app/Assets
ARCXR_UNITY_DIR = ${UNITY_ASSETS_DIR}/ArcXR
BUILD_OUTPUT = ${UNITY_ASSETS_DIR}/Plugins/ArcXR/Arc
grpc_csharp_exe="${GOPATH}/bin/grpc_csharp_plugin"

## display this help message
help:
	@echo -e "\033[32m"
	@echo "go-arcspace"
	@echo "  BUILD_DIR:       ${BUILD_DIR}"
	@echo "  PARENT_DIR:      ${PARENT_DIR}"
	@echo "  BUILD_OUTPUT:    ${BUILD_OUTPUT}"
	@echo
	@awk '/^##.*$$/,/[a-zA-Z_-]+:/' $(MAKEFILE_LIST) | awk '!(NR%2){print $$0p}{p=$$0}' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m  %-16s\033[0m %s\n", $$1, $$2}' | sort

# ----------------------------------------
# build

GOFILES = $(shell find . -type f -name '*.go')
	
.PHONY: build protos

## build archost and lib-archost
build:  archost lib-archost


#
# https://rogchap.com/2020/09/14/running-go-code-on-ios-and-android/
# TODO: use dynamic linking so that we don't have to restart Unity to load a new binary?
# https://github.com/vladimirvivien/go-cshared-examples  
#




## build archost.so (for embedding clients such as Unity and Unreal)
lib-archost: $(GOFILES)
#   Info on cross-compiling Go: https://freshman.tech/snippets/go/cross-compile-go-programs/
	touch cmd/lib-archost/main.go
	cd cmd/lib-archost && \
		GOOS=darwin GOARCH=amd64 \
		go build -trimpath -o "${BUILD_OUTPUT}/lib-archost.so" -buildmode=c-shared main.go
	rm "${BUILD_OUTPUT}/lib-archost.h"

## build archost ("headless" daemon)
archost: $(GOFILES)
	touch cmd/archost/main.go
	cd cmd/archost && \
		GOOS=darwin GOARCH=amd64 \
		go build .
	
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
	    --csharp_out "${ARCXR_UNITY_DIR}/Arc" \
	    --grpc_out   "${ARCXR_UNITY_DIR}/Arc" \
	    --plugin=protoc-gen-grpc="${grpc_csharp_exe}" \
	    --proto_path=. \
		arc/arc.proto

	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --csharp_out "${ARCXR_UNITY_DIR}/Crates" \
	    --proto_path=. \
		crates/crates.proto

				
	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --proto_path=. \
		ski/api.ski.proto