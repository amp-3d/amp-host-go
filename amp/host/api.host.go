package host

import (
	"time"

	"github.com/arcspace/go-archost/amp/assets"
	arc_av "github.com/arcspace/go-archost/apps/av/apps"
	arc_bridges "github.com/arcspace/go-archost/apps/bridges/apps"
	arc_services "github.com/arcspace/go-archost/apps/services/apps"
	arc_sys "github.com/arcspace/go-archost/apps/sys/apps"
	"github.com/git-amp/amp-sdk-go/amp"
)

type Opts struct {
	assets.AssetServer
	Desc         string        // label for this host
	StatePath    string        // local fs path where user and state data is stored
	CachePath    string        // local fs path where purgeable data is stored
	Debug        bool          // enable debug mode
	AppIdleClose time.Duration // how long to wait before closing an idle app
	LoginTimeout time.Duration // how long to wait for a login request
	Registry     amp.Registry  // registry to use for this host
}

func DefaultOpts(assetPort int, debugMode bool) Opts {
	opts := Opts{
		Desc:         "amp.Host",
		StatePath:    "~/_.archost",
		Registry:     amp.NewRegistry(),
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

// StartNewHost starts a new host with the given opts
func StartNewHost(opts Opts) (amp.Host, error) {
	reg := opts.Registry
	amp.RegisterBuiltInTypes(reg)
	arc_sys.RegisterFamily(reg)
	arc_bridges.RegisterFamily(reg)
	arc_services.RegisterFamily(reg)
	arc_av.RegisterFamily(reg)
	return startNewHost(opts)
}
