package lib_service

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/git-amp/amp-sdk-go/amp"
	"github.com/git-amp/amp-sdk-go/stdlib/task"
)

// libService offers Msg transport over direct dll calls.
type libService struct {
	task.Context
	host amp.Host
	opts LibServiceOpts
}

func (srv *libService) StartService(on amp.Host) error {
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
		fromClient: make(chan *amp.Msg),
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
	hostSess  amp.HostSession
	closed    int32
	mallocs   map[*byte]struct{} // retains allocations so they are not GCed
	mallocsMu sync.Mutex

	// TODO: reimplement below using sync.Cond
	//xfer     sync.Cond
	//xferMu   sync.Mutex
	fromClient chan *amp.Msg
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
func (sess *libSession) RecvMsg() (*amp.Msg, error) {
	select {
	case msg := <-sess.fromClient:
		return msg, nil
	case <-sess.closing:
		return nil, amp.ErrStreamClosed
	}
}

// Executed on a client thread
func (sess *libSession) EnqueueIncoming(msg *amp.Msg) error {
	select {
	case sess.fromClient <- msg:
		return nil
	case <-sess.closing:
		return amp.ErrStreamClosed
	}
}

///////////////////////// host -> client /////////////////////////

// Executed on a host thread
func (sess *libSession) SendMsg(tx *amp.Msg) error {

	// Serialize the outgoing msg into an existing buffer (or allocate a new one)
	txLen := tx.Size() + int(amp.TxHeader_Size)
	var msg_pb []byte
	select {
	case msg_pb = <-sess.free:
	default:
	}
	sess.Realloc(&msg_pb, int64(txLen))
	if err := tx.MarshalToTxBuffer(msg_pb); err != nil {
		return err
	}

	select {
	case sess.toClient <- msg_pb:
		return nil
	case <-sess.closing:
		return amp.ErrStreamClosed
	}
}

// Executed on a client thread
func (sess *libSession) DequeueOutgoing(msg_pb *[]byte) error {

	// 1) Retain the given ready (free) buffer
	// If the free pool is full, reclaim the buffer now
	if len(sess.free) == 0 {
		sess.free <- *msg_pb
	} else {
		sess.Realloc(msg_pb, 0)
	}

	// 2) Block until the next outgoing msg appears (or stream is closed)
	select {
	case *msg_pb = <-sess.toClient:
		return nil
	case <-sess.closing:
		return amp.ErrStreamClosed
	}
}
