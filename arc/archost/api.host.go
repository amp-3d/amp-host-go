package archost

import (
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps"
	"github.com/arcspace/go-archost/arc/assets"
)

/*
packages

	arc
	    Arcspace interfaces and support utils
	arc/host
	    an implementation of arc.Host & arc.HostSession
	arc/grpc_service
		implements a grpc server that consumes a arc.Host instance
	arc/apps
		implementations of arc.App


	archost task.Context model:
		001 Host
		    002 HostHomePlanet
		        004 HostSession
		        007 cell_101
		    003 grpc.HostService
		        005 grpc <- HostSession(4)
		        006 grpc -> HostSession(4)

	May this project be dedicated to God, for all other things are darkness or imperfection.
	May these hands and this mind be blessed with Holy Spirit and Holy Purpose.
	May I be an instrument for manifesting software that serves the light and used to manifest joy at the largest scale possible.
	May the blocks to this mission dissolve into light amidst God's will.

	~ Dec 25th, 2021

*/

type Opts struct {
	assets.AssetServer
	Desc         string        // label for this host
	StatePath    string        // local fs path where user and state data is stored
	CachePath    string        // local fs path where purgeable data is stored
	AppIdleClose time.Duration // how long to wait before closing an idle app
	Registry     arc.Registry  // registry to use for this host
}

func DefaultOpts(assetPort int) Opts {
	opts := Opts{
		Desc:         "arc.Host",
		StatePath:    "~/_.archost",
		Registry:     arc.NewRegistry(),
		AppIdleClose: time.Minute * 1, // 10 mins?
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
	apps.RegisterStdApps(opts.Registry)
	return startNewHost(opts)
}
