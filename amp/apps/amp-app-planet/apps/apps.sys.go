package reg

import (
	core_suite "github.com/amp-3d/amp-host-go/amp/app-suites/core/registry"
	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-planet/planet"
)

func init() {
	reg := core_suite.Registry()
	
	planet.RegisterApp(reg)
}
