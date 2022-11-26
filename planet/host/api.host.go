package host

import (
	"github.com/arcverse/go-planet/planet"
)


var DefaultHostOpts = HostOpts{
    BasePath: "~/_.phost",
}


type HostOpts struct {
	BasePath string // local file path where planet dbs are stored
}

// StartNewHost starts a new host with the given opts
func StartNewHost(opts HostOpts) (planet.Host, error) {
	return newHost(opts)
}

