package host

import (
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	arc_amp "github.com/arcspace/go-archost/apps/amp/apps"
	arc_bridges "github.com/arcspace/go-archost/apps/bridges/apps"
	arc_services "github.com/arcspace/go-archost/apps/services/apps"
	arc_sys "github.com/arcspace/go-archost/apps/sys/apps"
	"github.com/arcspace/go-archost/arc/assets"
)

type Opts struct {
	assets.AssetServer
	Desc         string        // label for this host
	StatePath    string        // local fs path where user and state data is stored
	CachePath    string        // local fs path where purgeable data is stored
	Debug        bool          // enable debug mode
	AppIdleClose time.Duration // how long to wait before closing an idle app
	LoginTimeout time.Duration // how long to wait for a login request
	Registry     arc.Registry  // registry to use for this host
}

func DefaultOpts(assetPort int, debugMode bool) Opts {
	opts := Opts{
		Desc:         "arc.Host",
		StatePath:    "~/_.archost",
		Registry:     arc.NewRegistry(),
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
func StartNewHost(opts Opts) (arc.Host, error) {
	reg := opts.Registry
	arc.RegisterBuiltInTypes(reg)
	arc_sys.RegisterFamily(reg)
	arc_bridges.RegisterFamily(reg)
	arc_services.RegisterFamily(reg)
	arc_amp.RegisterFamily(reg)
	return startNewHost(opts)
}
