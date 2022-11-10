MAKEFLAGS += --warn-undefined-variables
SHELL = /bin/bash -o nounset -o errexit -o pipefail
.DEFAULT_GOAL = build
UNITY_ASSETS_DIR = ../unity-app/Assets
UNITY_PLANETXR_DIR = ${UNITY_ASSETS_DIR}/Genesis3/PlanetXR
BUILD_OUTPUT = ${UNITY_ASSETS_DIR}/Plugins/Genesis3/PlanetXR
grpc_csharp_exe="${GOPATH}/bin/grpc_csharp_plugin"

## display this help message
help:
	@echo -e "\033[32m"
	@echo "go-planet"
	@echo
	@awk '/^##.*$$/,/[a-zA-Z_-]+:/' $(MAKEFILE_LIST) | awk '!(NR%2){print $$0p}{p=$$0}' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}' | sort

# ----------------------------------------
# build

GOFILES = $(shell find . -type f -name '*.go')
	
.PHONY: build protos

## build lib-phost
build:  phost lib-phost


#
# https://rogchap.com/2020/09/14/running-go-code-on-ios-and-android/
# TODO: use dynamic linking so that we don't have to restart Unity to load a new binary?
# https://github.com/vladimirvivien/go-cshared-examples  
#


## build dylib for use in Unity, Unreal, and other embedding clients lib-phost.so
## Info on cross-compiling Go: https://freshman.tech/snippets/go/cross-compile-go-programs/
lib-phost: $(GOFILES)
	touch cmd/lib-phost/main.go
	cd cmd/lib-phost && \
		GOOS=darwin GOARCH=amd64 \
		go build -trimpath -o ../../${BUILD_OUTPUT}/lib-phost.so -buildmode=c-shared main.go
	rm ${BUILD_OUTPUT}/lib-phost.h

# build phost command line daemon
phost: $(GOFILES)
	touch cmd/phost/main.go
	cd cmd/phost && \
		GOOS=darwin GOARCH=amd64 \
		go build .
	
## GrpcTools (2.49.1)
## Install protoc & grpc_csharp_plugin:
##      - Download latest Grpc.Tools from https://nuget.org/packages/Grpc.Tools
##      - Extract .nupkg as .zip, move protoc and grpc_csharp_plugin to ${GOPATH}/bin 
## Or, just protoc: https://github.com/protocolbuffers/protobuf/releases
## Links: https://grpc.io/docs/languages/csharp/quickstart/
tools:
	go install github.com/gogo/protobuf/protoc-gen-gogoslick
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go get -d  github.com/gogo/protobuf/proto


## generate .cs and .go from proto files
protos:

	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --csharp_out ${UNITY_ASSETS_DIR} --csharp_opt=base_namespace=   \
	    --grpc_out "${UNITY_PLANETXR_DIR}"   \
	    --plugin=protoc-gen-grpc="${grpc_csharp_exe}" \
	    --proto_path=. \
		planet/planet.proto

	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --csharp_out ${UNITY_ASSETS_DIR} --csharp_opt=base_namespace= \
	    --proto_path=. \
		crates/crates.proto

	# protoc \
	#     --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	#     --csharp_out ${UNITY_ASSETS_DIR} --csharp_opt=base_namespace= \
	#     --proto_path=. \
	# 	planet/client/client.proto
		
	# protoc \
	# 	--go_out=./planet/builtin_types \
	# 	--go_opt=paths=source_relative \
	# 	planet/planet.proto \

		
	# protoc \
	#     --gogoslick_opt=paths=source_relative \
	#     --gogoslick_out=plugins=grpc:. \
	#     --proto_path=. \
	# 	planet/host/state.proto
				
	protoc \
	    --gogoslick_out=plugins=grpc:. --gogoslick_opt=paths=source_relative \
	    --proto_path=. \
		ski/api.ski.proto