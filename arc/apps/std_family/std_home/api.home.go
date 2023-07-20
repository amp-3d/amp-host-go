package std_home

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/std_family/std"
)

const (
	AppID = "home" + std.AppFamilyDomain
)

func UID() arc.UID {
	return arc.FormUID(0x5440da3a712f4550, 0x80d755186f311729)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "links & bookmarks",
		Version: "v1.2023.2",
		// DataModels: arc.DataModelMap{
		// 	ModelsByID: map[string]arc.DataModel{
		// 		CellDataModel_Dir:  {},
		// 		CellDataModel_Link: {},
		// 	},
		// },
		NewAppInstance: func() arc.AppInstance {
			return nil
		},
	})
}
