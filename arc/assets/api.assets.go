package assets

import (
	"fmt"
	"io"
	"time"

	"github.com/arcspace/go-cedar/process"
)

// Consumed by client wishing to post an data asset
type AssetServer interface {
	process.Context
	StartService(host process.Context) error
	GracefulStop()
	PublishAsset(asset MediaAsset) (URL string, err error)
}

type MediaAsset interface {

	// Helpful short description of this asset
	Label() string

	// Returns the media (MIME) type of this asset
	MediaType() string

	// OnStart is called when this asset is published in the given context.   ctx.Closing() is signaled if/when:
	//  - the host AssetServer is shutting down
	//  - there are no child AssetReaders after an expiration delay
	// If this asset encounters a fatal error, it should call ctx.Close().
	OnStart(ctx process.Context) error

	// Called when this asset is requested by a client for read access
	NewAssetReader() (AssetReader, error)
}

// Provides read access to its parent PinnedAsset
type AssetReader interface {
	io.ReadSeekCloser
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
