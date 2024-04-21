package host

import (
	_ "github.com/amp-3d/amp-host-go/amp/app-suites/core/apps" // calls init() for apps in suite
	core_apps "github.com/amp-3d/amp-host-go/amp/app-suites/core/registry"
	"github.com/amp-3d/amp-sdk-go/amp"
)

func RootRegistry() amp.Registry {
	if gRegistry == nil {
	    gRegistry = amp.NewRegistry()
	}
	amp.RegisterBuiltinTypes(gRegistry)
	gRegistry.Import(core_apps.Registry())
	return gRegistry
}

var gRegistry amp.Registry