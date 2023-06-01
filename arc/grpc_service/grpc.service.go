package grpc_service

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/process"
)

// grpcServer is the GRPC implementation of repo.proto
type grpcServer struct {
	process.Context
	server *grpc.Server
	host   arc.Host
	opts   GrpcServerOpts
}

func (srv *grpcServer) ServiceURI() string {
	return srv.opts.ServiceURI
}

func (srv *grpcServer) Host() arc.Host {
	return srv.host
}

func (srv *grpcServer) StartService(on arc.Host) error {
	if srv.host != nil || srv.server != nil || srv.Context != nil {
		panic("already started")
	}
	srv.host = on

	lis, err := net.Listen(srv.opts.ListenNetwork, srv.opts.ListenAddr)
	if err != nil {
		return errors.Errorf("failed to listen: %v", err)
	}

	srv.server = grpc.NewServer(
	// 		grpc.StreamInterceptor(srv.StreamServerInterceptor()),
	// 		grpc.UnaryInterceptor(srv.UnaryServerInterceptor()),
	)
	arc.RegisterHostGrpcServer(srv.server, srv)

	srv.Context, err = srv.host.StartChild(&process.Task{
		Label:     fmt.Sprintf("%s.HostService %v", srv.ServiceURI(), lis.Addr().String()),
		IdleClose: time.Nanosecond,
		OnRun: func(ctx process.Context) {
			srv.Infof(0, "Serving on \x1b[1;32m%v %v\x1b[0m", srv.opts.ListenNetwork, srv.opts.ListenAddr)
			srv.server.Serve(lis)
			srv.Info(2, "Serve COMPLETE")
		},
		OnClosing: func() {
			if srv.server != nil {
				srv.Info(1, "Stop")
				srv.server.Stop()
				srv.Info(2, "Stop COMPLETE")
			}
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (srv *grpcServer) GracefulStop() {
	if srv.server != nil {
		srv.Info(0, "GracefulStop")
		srv.server.GracefulStop()
	}
}

// HostSession is a callback for when a new grpc session instance a client opens.
// Multiple pipes can be open at any time by the same client or multiple clients.
func (srv *grpcServer) HostSession(rpc arc.HostGrpc_HostSessionServer) error {
	sess := &grpcSess{
		srv:     srv,
		rpc:     rpc,
		closing: make(chan struct{}),
	}

	var err error
	sess.hostSess, err = srv.host.StartNewSession(srv, sess)
	if err != nil {
		return err
	}

	// Block until the associated host session enters a closing state
	select {
	case <-sess.hostSess.Closing():
	case <-sess.closing:
	}
	return nil
}

type grpcSess struct {
	closed   int32
	closing  chan struct{}
	srv      *grpcServer
	rpc      arc.HostGrpc_HostSessionServer
	hostSess arc.HostSession
}

func (sess *grpcSess) Desc() string {
	return sess.srv.ServiceURI()
}

func (sess *grpcSess) Close() {
	if atomic.CompareAndSwapInt32(&sess.closed, 0, 1) {
		close(sess.closing)
	}
}

func (sess *grpcSess) SendMsg(msg *arc.Msg) error {
	err := sess.rpc.SendMsg(msg)
	if err == nil {
		return nil
	}
	if status.Code(err) == codes.Canceled {
		err = arc.ErrStreamClosed
	}
	return err
}

func (sess *grpcSess) RecvMsg() (*arc.Msg, error) {
	msg := arc.NewMsg()
	err := sess.rpc.RecvMsg(msg)
	if err == nil {
		return msg, nil
	}
	if status.Code(err) == codes.Canceled || err == io.EOF {
		err = arc.ErrStreamClosed
	}
	return msg, err
}

/*
func (srv *grpcServer) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		srv.Debugf(trimStr(fmt.Sprintf("[rpc server] %v %+v", info.FullMethod, req), 500))

		x, err := handler(ctx, req)
		if err != nil {
			srv.Errorf(trimStr(fmt.Sprintf("[rpc server] %v %+v %+v", info.FullMethod, req, err), 500))
		}

		srv.Debugf(trimStr(fmt.Sprintf("[rpc server] %v %+v, %+v", info.FullMethod, req, x), 500))
		return x, err
	}
}

// StreamServerInterceptor is a debugging helper
func (srv *grpcServer) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(server interface{}, stream grpc.Transport, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		srv.Infof(2, "[rpc server] %v", info.FullMethod)
		err := handler(server, stream)
		if err != nil {
			srv.Errorf("[rpc server] %+v", err)
		}
		return err
	}
}

func trimStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
*/
