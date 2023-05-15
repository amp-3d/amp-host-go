package archost

import (
	"log"

	"github.com/arcspace/go-archost/arc"
	"github.com/arcspace/go-archost/arc/apps/amp"
	"github.com/arcspace/go-archost/arc/host"
)

// StartNewHost starts a new host with the given opts
func StartNewHost(opts host.HostOpts) arc.Host {
	h, err := host.StartNewHost(opts)
	if err != nil {
		log.Fatalf("failed to start new host: %v", err)
	}

	h.RegisterApp(amp.NewApp())

	return h
}
