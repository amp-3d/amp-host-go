package grpc_server

import (
	"net"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	grpc_codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/arcverse/go-arcverse/planet"
	"github.com/arcverse/go-cedar/process"
)

// grpcServer is the GRPC implementation of repo.proto
type grpcServer struct {
	process.Context

	host planet.Host
	//shutdown      <-chan struct{}
	server *grpc.Server
	opts   GrpcServerOpts
}

func (srv *grpcServer) attachAndStart(label string) error {

	if srv.server != nil {
		panic("GrpcServer already started")
	}

	lis, err := net.Listen(srv.opts.ListenNetwork, srv.opts.ListenAddr)
	if err != nil {
		return errors.Errorf("failed to listen: %v", err)
	}

	srv.server = grpc.NewServer(
	// 		grpc.StreamInterceptor(srv.StreamServerInterceptor()),
	// 		grpc.UnaryInterceptor(srv.UnaryServerInterceptor()),
	)
	planet.RegisterHostGrpcServer(srv.server, srv)

	srv.Context, err = srv.host.StartChild(&process.Task{
		Label:     label,
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
		srv.Info(2, "GracefulStop COMPLETE")
	}
}

type grpcSess struct {
	process.Context

	srv      *grpcServer
	rpc      planet.HostGrpc_HostSessionServer
	hostSess planet.HostSession
}

// HostSession is a callback for when a new grpc session instance a client opens.
// Multiple pipes can be open at any time by the same client or multiple clients.
func (srv *grpcServer) HostSession(rpc planet.HostGrpc_HostSessionServer) error {
	sess := &grpcSess{
		srv: srv,
		rpc: rpc,
	}

	var err error
	sess.hostSess, err = srv.host.StartNewSession()
	if err != nil {
		return err
	}

	// Possible paths:
	//   - If the grpcSess is closing, initiate hostSess.Close()
	//   - If the grpcSess (rpc) is cancelled, initiate hostSess.Close()
	//   - On when the hostSess is done, close and exit the rpc stream
	sess.Context, _ = srv.Context.StartChild(&process.Task{
		Label:     "grpcSess",
		IdleClose: time.Nanosecond,
		OnClosing: func() {
			sess.Info(2, "sess.hostSess.Close()")
			sess.hostSess.Close()
		},
	})

	sess.Context.StartChild(&process.Task{
		Label:     "grpc <- hostSess.Outbox",
		IdleClose: time.Nanosecond,
		OnRun: func(ctx process.Context) {
			sessOutbox := sess.hostSess.Outbox()
			sessDone := sess.hostSess.Done()

			// Forward outgoing msgs from the host to the grpc outlet until the host session says its completely done.
			for running := true; running; {
				select {
				case msg := <-sessOutbox:
					if msg != nil {
						if msg.ValBufIsShared {
							msg.ValBufIsShared = false
							err = sess.rpc.Send(msg)
							msg.ValBufIsShared = true
						} else {
							err = sess.rpc.Send(msg)
						}
						msg.Reclaim()
						msg = nil
						if err != nil {
							ctx.Warnf("sess.rpc.Send() err: %v", err)
						}
					}
				case <-sessDone:
					ctx.Info(2, "<-hostDone")
					running = false
				}
			}
		},
	})

	sess.Context.StartChild(&process.Task{
		Label:     "grpc -> hostSess.Inbox",
		IdleClose: time.Nanosecond,
		OnRun: func(ctx process.Context) {
			sessInbox := sess.hostSess.Inbox()
			sessDone := sess.hostSess.Done()

			for running := true; running; {
				msg := planet.NewMsg()
				err := sess.rpc.RecvMsg(msg)
				if err != nil {
					msg.Reclaim()
					msg = nil
					if status.Code(err) == grpc_codes.Canceled {
						ctx.Info(2, "grpc_codes.Canceled")
						sess.Context.Close()
						running = false
					} else {
						ctx.Warnf("sess.rpc.Recv() err: %v", err)
					}
				}

				// Forward incoming Msgs to the host or until the host session says its done
				if msg != nil {
					select {
					case sessInbox <- msg:
					case <-sessDone:
						ctx.Info(2, "hostSession done")
						running = false
					}
				}
			}
		},
	})

	<-sess.Done()

	return nil
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
	return func(server interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
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
