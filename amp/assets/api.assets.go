package assets

import (
	"fmt"
	"time"

	"github.com/amp-3d/amp-sdk-go/stdlib/media"
	"github.com/amp-3d/amp-sdk-go/stdlib/task"
)

// Consumed by an amp.App wishing to post an data asset for steaming (e.g. audio, video).
type AssetServer interface {
	task.Context
	media.Publisher
	StartService(host task.Context) error
	GracefulStop()
}

// HttpServerOpts exposes options and params
type HttpServerOpts struct {
	IdleExpire time.Duration
	ListenAddr string
}

// DefaultHttpServerOpts returns the default options for a AssetServer
func DefaultHttpServerOpts(listenPort int) HttpServerOpts {
	return HttpServerOpts{
		IdleExpire: 20 * time.Second,
		ListenAddr: fmt.Sprintf(":%v", listenPort),
	}
}

func NewAssetServer(opts HttpServerOpts) AssetServer {
	return newHttpServer(opts)
}
