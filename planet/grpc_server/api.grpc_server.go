package grpc_server

import (
	"fmt"

	"github.com/arcverse/go-arcverse/planet"
)

// HostGrpcServer attaches to a planet.Host as a child process, offering grpc-based connections.
type HostGrpcServer interface {
	planet.Context

	// Blocks until the server has copmpeted a graceful stop (which could be a any amount of time)
	GracefulStop()
}

// GrpcServerOpts exposes grpc server options and params
type GrpcServerOpts struct {
	ListenNetwork string
	ListenAddr    string
}

// DefaultGrpcServerOpts returns the default options for a GrpcServer
// Fun fact: using "127.0.0.1" specifically binds to localhost, so incoming outside connections will be refused.
// Until then, we want to need to accept incoming outside connections, go by default 0.0.0.0 will accept all incoming connections.
func DefaultGrpcServerOpts(listenPort int) GrpcServerOpts {
	return GrpcServerOpts{
		ListenNetwork: "tcp",
		ListenAddr:    fmt.Sprintf("0.0.0.0:%v", listenPort),
	}
}

// AttachNewGrpcServer attaches a child process to the given host, providing grpc-based sessions.
func (opts GrpcServerOpts) AttachNewGrpcServer(host planet.Host) (HostGrpcServer, error) {
	srv := &grpcServer{
		host: host,
		opts: opts,
	}
	if err := srv.attachAndStart("GrpcServer"); err != nil {
		return nil, err
	}
	return srv, nil
}
