package host

import (
	"time"

	"github.com/arcspace/go-archost/arc"
	"github.com/arcspace/go-archost/arc/assets"
)

type HostOpts struct {
	assets.AssetServer
	Label              string // label of this host
	StatePath          string // local fs path where user and state data is stored
	CachePath          string // local fs path where purgeable data is stored
}

func DefaultHostOpts(assetPort int) HostOpts {
	opts := HostOpts{
		Label:     "arc.Host",
		StatePath: "~/_.archost",
	}
	
	if assetPort <= 0 { 
		assetPort = 60000 + (int(time.Now().UnixNano()) & 0x7FF)
	}
	assetSrvOpts := assets.DefaultHttpServerOpts(assetPort)
	opts.AssetServer = assets.NewAssetServer(assetSrvOpts)
	return opts
}

// StartNewHost starts a new host with the given opts
func StartNewHost(opts HostOpts) (arc.Host, error) {
	return startNewHost(opts)
}
