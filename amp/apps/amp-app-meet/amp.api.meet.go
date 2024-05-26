package meet

import (
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
)

func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppSpec: tag.FormSpec(amp.AttrSpec, "meet"),
		Desc:    "",
		Version: "v1.2023.2",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			return nil, nil
		},
	})
}
