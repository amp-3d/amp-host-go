package lib_service

import "github.com/arcspace/go-arc-sdk/apis/arc"

// LibServiceOpts exposes options and settings
type LibServiceOpts struct {
}

func DefaultLibServiceOpts() LibServiceOpts {
	return LibServiceOpts{
	}
}

type LibService interface {
	arc.HostService

	NewLibSession() (LibSession, error)
}

type LibSession interface {
	Close() error

	Realloc(buf *[]byte, newLen int64)

	// Blocking calls to send/recv Msgs to the host
	EnqueueIncoming(txMsg *arc.TxMsg) error
	DequeueOutgoing(txMsg *[]byte) error
}

func (opts LibServiceOpts) NewLibService() LibService {
	return &libService{
		opts: opts,
	}
}
