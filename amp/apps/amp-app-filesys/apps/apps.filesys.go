package filesys

import (
	core_suite "github.com/amp-3d/amp-host-go/amp/app-suites/core/registry"
	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-filesys/ipfs"
	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-filesys/posix"
)


func init() {
	reg := core_suite.Registry()
	
	posix.RegisterApp(reg)
	ipfs.RegisterApp(reg)
}

