package host

import "github.com/arcverse/go-arcverse/pxr"


var DefaultHostOpts = HostOpts{
    BasePath: "~/_.phost",
}


type HostOpts struct {
	BasePath string // local file path where planet dbs are stored
}

// StartNewHost starts a new host with the given opts
func StartNewHost(opts HostOpts) (pxr.Host, error) {
	return newHost(opts)
}

