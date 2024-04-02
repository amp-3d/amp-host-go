// Implements a tcp server that attaches to a amp.Host instance as a transport layer.
package tcp_service

import (
	"fmt"
	"sync"

	"github.com/git-amp/amp-sdk-go/amp"
)

// TcpServerOpts exposes tcp server options and params
type TcpServerOpts struct {
	ListenNetwork string
	ListenAddr    string
}

// DefaultTcpServerOpts returns the default options for a TcpServer
// Fun fact: using "127.0.0.1" specifically binds to localhost, so incoming outside connections will be refused.
// Until then, we want to need to accept incoming outside connections, go by default 0.0.0.0 will accept all incoming connections.
func DefaultTcpServerOpts(listenPort int) TcpServerOpts {
	return TcpServerOpts{
		ListenNetwork: "tcp",
		ListenAddr:    fmt.Sprintf("0.0.0.0:%v", listenPort),
	}
}

func (opts TcpServerOpts) NewTcpServer() amp.HostService {
	srv := &tcpServer{
		opts:     opts,
		sessions: make(map[*tcpSess]struct{}),
	}
	srv.muCond = sync.NewCond(&srv.mu)
	return srv
}
