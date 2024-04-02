package reg

import (
	"github.com/arcspace/go-archost/apps/sys/planet"
	"github.com/arcspace/go-archost/apps/sys/ux"
	"github.com/git-amp/amp-sdk-go/amp"
)

func RegisterFamily(reg amp.Registry) {
	planet.RegisterApp(reg)
	ux.RegisterApp(reg)
}
