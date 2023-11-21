package lib_service

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
)

// libService offers Msg transport over direct dll calls.
type libService struct {
	task.Context
	host arc.Host
	opts LibServiceOpts
}

func (srv *libService) StartService(on arc.Host) error {
	if srv.host != nil || srv.Context != nil {
		panic("already attached")
	}
	srv.host = on

	var err error
	srv.Context, err = srv.host.StartChild(&task.Task{
		Label:     "lib.HostService",
		IdleClose: time.Nanosecond,
	})
	if err != nil {
		return err
	}

	return nil
}

func (srv *libService) NewLibSession() (LibSession, error) {
	sess := &libSession{
		srv:        srv,
		mallocs:    make(map[*byte]struct{}),
		fromClient: make(chan *arc.TxMsg),
		toClient:   make(chan []byte),
		free:       make(chan []byte, 1),
		closing:    make(chan struct{}),
	}
	var err error
	sess.hostSess, err = srv.host.StartNewSession(srv, sess)
	if err != nil {
		return nil, err
	}

	return sess, nil
}

func (srv *libService) GracefulStop() {
	if srv.Context != nil {
		srv.Context.Close()
	}
}

type libSession struct {
	srv       *libService
	hostSess  arc.HostSession
	closed    int32
	mallocs   map[*byte]struct{} // retains allocations so they are not GCed
	mallocsMu sync.Mutex

	// TODO: reimplement below using sync.Cond
	//xfer     sync.Cond
	//xferMu   sync.Mutex
	fromClient chan *arc.TxMsg
	toClient   chan []byte
	closing    chan struct{}
	free       chan []byte
	// //outgoing   []byte
	// //idk sync.Mutex
	// outgoing  [2][]byte

	// bufFree atomic.Value
}

func (sess *libSession) Label() string {
	return "lib.Session"
}

func (sess *libSession) Close() error {
	if atomic.CompareAndSwapInt32(&sess.closed, 0, 1) {
		close(sess.closing)
	}
	return nil
}

// Resizes the given buffer to the requested length.
// If newLen == 0, the buffer is freed and *buf zeroed
func (sess *libSession) Realloc(buf *[]byte, newLen int64) {
	if newLen < 0 {
		newLen = 0
	}

	// only change the len if the buffer is big enough
	capSz := int64(cap(*buf))
	if newLen > 0 && newLen <= capSz {
		*buf = (*buf)[:newLen]
		return
	}

	sess.mallocsMu.Lock()
	{
		// Free prev buffer if allocated
		if capSz > 0 {
			ptr := &(*buf)[0]
			delete(sess.mallocs, ptr)
		}

		// Allocate new buf and place it in our tracker map so to the GC doesn't taketh away
		if newLen > 0 {
			dimSz := (newLen + 0x3FF) &^ 0x3FF
			newBuf := make([]byte, dimSz)
			ptr := &newBuf[0]
			sess.mallocs[ptr] = struct{}{}
			*buf = newBuf[:newLen]
		} else {
			*buf = []byte{}
		}
	}
	sess.mallocsMu.Unlock()
}

///////////////////////// client -> host /////////////////////////

// Executed on a host thread
func (sess *libSession) RecvTx() (*arc.TxMsg, error) {
	select {
	case tx := <-sess.fromClient:
		return tx, nil
	case <-sess.closing:
		return nil, arc.ErrStreamClosed
	}
}

// Executed on a client thread
func (sess *libSession) EnqueueIncoming(tx *arc.TxMsg) error {
	select {
	case sess.fromClient <- tx:
		return nil
	case <-sess.closing:
		return arc.ErrStreamClosed
	}
}

///////////////////////// host -> client /////////////////////////

// Executed on a host thread
func (sess *libSession) SendTx(tx *arc.TxMsg) error {

	// Serialize the outgoing msg into an existing buffer (or allocate a new one)
	var txBuf []byte
	select {
	case txBuf = <-sess.free:
	default:
	}
	
	txBuf = tx.MarshalTo(txBuf[:0])
	select {
	case sess.toClient <- txBuf:
		return nil
	case <-sess.closing:
		return arc.ErrStreamClosed
	}
}

// Executed on a client thread
func (sess *libSession) DequeueOutgoing(txBuf *[]byte) error {

	// 1) Retain the given ready (free) buffer
	// If the free pool is full, reclaim the buffer now
	if len(sess.free) == 0 {
		sess.free <- *txBuf
	} else {
		sess.Realloc(txBuf, 0)
	}

	// 2) Block until the next outgoing msg appears (or stream is closed)
	select {
	case *txBuf = <-sess.toClient:
		return nil
	case <-sess.closing:
		return arc.ErrStreamClosed
	}
}
