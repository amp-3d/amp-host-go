package apps

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/services/go_http"
)

func RegisterFamily(reg arc.Registry) {
	go_http.RegisterApp(reg)
}
