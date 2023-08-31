package ux

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/sys"
)

const (
	AppID = "ux" + sys.AppFamilyDomain
)

func UID() arc.UID {
	return arc.FormUID(0x5440da3a712f4550, 0x80d755186f311729)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.App{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "UI/UX",
		Version: "v1.2023.2",
		NewAppInstance: func() arc.AppInstance {
			return nil
		},
	})
}
