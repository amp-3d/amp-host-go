package assets

import (
	"fmt"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/process"
)

// Consumed by client wishing to post an data asset
type AssetServer interface {
	process.Context
	StartService(host process.Context) error
	GracefulStop()
	PublishAsset(asset arc.MediaAsset) (URL string, err error)
}

// HttpServerOpts exposes options and params
type HttpServerOpts struct {
	IdleExpire time.Duration
	ListenAddr string
}

// DefaultHttpServerOpts returns the default options for a AssetServer
func DefaultHttpServerOpts(listenPort int) HttpServerOpts {
	return HttpServerOpts{
		IdleExpire: time.Minute,
		ListenAddr: fmt.Sprintf(":%v", listenPort),
	}
}

func NewAssetServer(opts HttpServerOpts) AssetServer {
	return newHttpServer(opts)
}
