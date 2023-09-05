package assets

import (
	"fmt"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/task"
)

// Consumed by client wishing to post an data asset
type AssetServer interface {
	task.Context
	arc.AssetPublisher
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
