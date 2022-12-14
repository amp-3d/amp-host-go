package grpc_service

import (
	"fmt"

	"github.com/arcspace/go-arcspace/pxr"
)

// GrpcServerOpts exposes grpc server options and params
type GrpcServerOpts struct {
	ServiceURI    string
	ListenNetwork string
	ListenAddr    string
}

// DefaultGrpcServerOpts returns the default options for a GrpcServer
// Fun fact: using "127.0.0.1" specifically binds to localhost, so incoming outside connections will be refused.
// Until then, we want to need to accept incoming outside connections, go by default 0.0.0.0 will accept all incoming connections.
func DefaultGrpcServerOpts(listenPort int) GrpcServerOpts {
	return GrpcServerOpts{
		ServiceURI:    "grpc",
		ListenNetwork: "tcp",
		ListenAddr:    fmt.Sprintf("0.0.0.0:%v", listenPort),
	}
}

func (opts GrpcServerOpts) NewGrpcServer() pxr.HostService {
	return &grpcServer{
		opts: opts,
	}
}
