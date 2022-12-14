package archost

import (
	"log"

	"github.com/arcspace/go-arcspace/pxr"
	"github.com/arcspace/go-arcspace/pxr/apps/filesys"
	"github.com/arcspace/go-arcspace/pxr/apps/vibe"
	"github.com/arcspace/go-arcspace/pxr/host"
)

// StartNewHost starts a new host with the given opts
func StartNewHost(opts host.HostOpts) pxr.Host {
	h, err := host.StartNewHost(opts)
	if err != nil {
		log.Fatalf("failed to start new host: %v", err)
	}

	h.RegisterApp(vibe.NewApp())
	h.RegisterApp(filesys.NewApp())

	return h
}
