package meet

import (
	"github.com/amp-3d/amp-sdk-go/amp"
)

// https://github.com/jech/galene
func RegisterApp(reg amp.Registry) {
	reg.RegisterApp(&amp.App{
		AppSpec: amp.AppSpec.With("galene"),
		Desc:    "",
		Version: "v1.2024.1",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			return nil, nil
		},
	})
}
