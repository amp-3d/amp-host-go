package go_http

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/services"
)

const (
	AppID = "go_http" + services.AppFamilyDomain
)

func UID() arc.UID {
	return arc.FormUID(0x5daf891a099341a7, 0x9c222f3a4a6b0346)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "Go's http server",
		Version: "v1.2023.2",
		NewAppInstance: func() arc.AppInstance {
			return nil //&appCtx{}
		},
	})
}
