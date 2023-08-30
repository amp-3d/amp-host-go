package ipfs

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/bridges"
)

const (
	AppID = "ipfs" + bridges.AppFamilyDomain
)

func UID() arc.UID {
	return arc.FormUID(0x90ebeaf9b36e4c3a, 0xa96acc838ce54829)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "bridge for Protocol Lab's IPFS",
		Version: "v1.2023.2",
		NewAppInstance: func() arc.AppInstance {
			return nil //&appCtx{}
		},
	})
}
