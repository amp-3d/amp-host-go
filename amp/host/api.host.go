package host

import (
	"time"

	"github.com/amp-3d/amp-host-go/amp/assets"
)

type Opts struct {
	assets.AssetServer
	Desc         string        // label for this host
	StatePath    string        // local fs path where user and state data is stored
	CachePath    string        // local fs path where purgeable data is stored
	Debug        bool          // enable debug mode
	AppIdleClose time.Duration // how long to wait before closing an idle app
	LoginTimeout time.Duration // how long to wait for a login request
}

func DefaultOpts(assetPort int, debugMode bool) Opts {
	opts := Opts{
		Desc:         "amp.Host",
		StatePath:    "~/_.archost",
		Debug:        debugMode,
		AppIdleClose: 5 * time.Minute,
		LoginTimeout: 9 * time.Second,
	}

	if opts.Debug {
		opts.AppIdleClose = 10 * time.Second
		opts.LoginTimeout = 1000 * time.Second
	}

	if assetPort <= 0 {
		assetPort = 60000 + (int(time.Now().UnixNano()) & 0x7FF)
	}

	assetSrvOpts := assets.DefaultHttpServerOpts(assetPort)
	opts.AssetServer = assets.NewAssetServer(assetSrvOpts)

	return opts
}
