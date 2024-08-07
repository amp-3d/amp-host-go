package tcp_service

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
)

// tcpServer implements amp.HostService and makes calls to amp.Host.StartNewSession() when a tcp client connects.
type tcpServer struct {
	task.Context
	host    amp.Host
	opts    TcpServerOpts
	lis     net.Listener
	stopped atomic.Bool

	mu       sync.Mutex            // guards following
	muCond   *sync.Cond            // signaled when connections close for GracefulStop
	sessions map[*tcpSess]struct{} // contains all active client sessions
}

func (srv *tcpServer) StartService(on amp.Host) error {
	if srv.host != nil || srv.lis != nil || srv.Context != nil {
		panic("already started")
	}
	srv.host = on

	var err error
	srv.lis, err = net.Listen(srv.opts.ListenNetwork, srv.opts.ListenAddr)
	if err != nil {
		return errors.Errorf("failed to listen: %v", err)
	}

	srv.Context, err = srv.host.StartChild(&task.Task{
		Info: task.Info{
			Label:     fmt.Sprint("tcp.HostService ", srv.lis.Addr().String()),
			IdleClose: time.Nanosecond,
		},
		OnRun: func(ctx task.Context) {
			srv.Log().Infof(0, "Serving on \x1b[1;32m%v %v\x1b[0m", srv.opts.ListenNetwork, srv.opts.ListenAddr)
			srv.Serve()
			srv.Log().Infof(2, "Serve COMPLETE")
		},
		OnClosing: func() {
			srv.Stop()
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (srv *tcpServer) Serve() {

	var errDelay time.Duration // how long to sleep on Accept failure
	for {
		conn, err := srv.lis.Accept()
		if err != nil {
			if errDelay == 0 {
				errDelay = 5 * time.Millisecond
			} else {
				errDelay *= 2
			}
			if max := 1 * time.Second; errDelay > max {
				errDelay = max
			}
			srv.Log().Warnf("Accept error: %v; retrying in %v", err, errDelay)

			timer := time.NewTimer(errDelay)
			select {
			case <-timer.C:
			case <-srv.Context.Closing():
				timer.Stop()
				return
			}
			continue
		}

		errDelay = 0

		srv.addClient(conn)
	}

}

func (srv *tcpServer) GracefulStop() {
	srv.Stop()
	<-srv.Context.Done()
}

func (srv *tcpServer) Stop() {
	if srv.lis == nil || srv.Context == nil {
		return
	}

	if srv.stopped.CompareAndSwap(false, true) {
		srv.mu.Lock()
		for sess := range srv.sessions {
			srv.tryCloseSess(sess, false)
		}
		srv.mu.Unlock()

		if srv.Context != nil {
			srv.Context.Close()
		}

		if srv.lis != nil {
			srv.lis.Close()
		}
	}
}

func (srv *tcpServer) addClient(conn net.Conn) {
	sess := &tcpSess{
		label: fmt.Sprint("tcp ", conn.RemoteAddr().String()),
		srv:   srv,
		conn:  conn,
	}

	srv.mu.Lock()
	{
		if srv.stopped.Load() {
			conn.Close()
			return
		}

		// conn.SetDeadline(time.Time{})
		srv.sessions[sess] = struct{}{}
	}
	srv.mu.Unlock()

	var err error
	sess.session, err = srv.host.StartNewSession(srv, sess)
	if err != nil {
		srv.tryCloseSess(sess, true)
	}

}

func (srv *tcpServer) tryCloseSess(sess *tcpSess, needsLock bool) {
	if needsLock {
		srv.mu.Lock()
	}

	if _, ok := srv.sessions[sess]; ok {
		delete(srv.sessions, sess)
		sess.conn.Close()
		if len(srv.sessions) == 0 && srv.stopped.Load() {
			srv.muCond.Broadcast()
		}
	}

	if needsLock {
		srv.mu.Unlock()
	}
}

type tcpSess struct {
	label    string
	srv      *tcpServer
	conn     net.Conn
	session amp.Session
	txBuf    []byte // TODO: use a pool?
}

func (sess *tcpSess) Label() string {
	return sess.label
}

func (sess *tcpSess) Close() error {
	if sess != nil && sess.srv != nil {
		sess.srv.tryCloseSess(sess, true)
	}
	return nil
}

func (sess *tcpSess) SendTx(tx *amp.TxMsg) error {
	err := tx.MarshalToWriter(&sess.txBuf, sess.conn)
	return filterErr(err)
}

func (sess *tcpSess) RecvTx() (*amp.TxMsg, error) {
	tx, err := amp.ReadTxMsg(sess.conn)
	return tx, filterErr(err)
}

func filterErr(err error) error {
	if err == io.EOF {
		err = amp.ErrStreamClosed
	}
	return err
}
