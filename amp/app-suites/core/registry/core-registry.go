package core_apps

import (
	"github.com/amp-3d/amp-sdk-go/amp"
)

// How an amp.App registers itself during compile time
// and is a singleton that an amp.Host forks during startup
func Registry() amp.Registry {
	if gRegistry == nil {
		gRegistry = amp.NewRegistry()
	}
	return gRegistry
}

var gRegistry amp.Registry
