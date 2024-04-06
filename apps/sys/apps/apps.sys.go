package reg

import (
	"github.com/amp-space/amp-host-go/apps/sys/planet"
	"github.com/amp-space/amp-host-go/apps/sys/ux"
	"github.com/amp-space/amp-sdk-go/amp"
)

func RegisterFamily(reg amp.Registry) {
	planet.RegisterApp(reg)
	ux.RegisterApp(reg)
}
