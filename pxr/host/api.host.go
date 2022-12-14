package host

import "github.com/arcspace/go-arcspace/pxr"

type HostOpts struct {
	Label     string // label of this host
	StatePath string // local fs path where user and state data is stored
	CachePath string // local fs path where purgeable data is stored
}

func DefaultHostOpts() HostOpts {
	opts := HostOpts{
		Label:     "Host",
		StatePath: "~/_.archost",
	}
	return opts
}

// StartNewHost starts a new host with the given opts
func StartNewHost(opts HostOpts) (pxr.Host, error) {
	return startNewHost(opts)
}
