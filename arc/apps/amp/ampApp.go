package amp

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp/amp_spotify"
	"github.com/arcspace/go-archost/arc/apps/amp/api"
	"github.com/arcspace/go-archost/arc/apps/amp/bs"
	"github.com/arcspace/go-archost/arc/apps/amp/filesys"
)

func RegisterApp(reg arc.Registry) {
	bs.RegisterApp(reg)
	filesys.RegisterApp(reg)
	amp_spotify.RegisterApp(reg)
	
	reg.RegisterApp(&arc.AppModule{
		AppID:      api.AmpAppURI,
		Version:    "v1.2023.2",
		DataModels: api.DataModels,
		Dependencies: []string{
			bs.AppID,
			filesys.AppID,
			amp_spotify.AppID,
		},
		NewAppInstance: func(ctx arc.AppContext) (arc.AppRuntime, error) {
			app := &appCtx{
				AppContext: ctx,
			}
			return app, nil
		},
	})
}

type appCtx struct {
	arc.AppContext
}

func (app *appCtx) HandleMetaMsg(msg *arc.Msg) (handled bool, err error) {
	return false, nil
}

func (app *appCtx) OnClosing() {
}

func (app *appCtx) PinCell(req *arc.CellReq) (arc.AppCell, error) {
	provider, _ := req.GetKwArg(api.KwArg_Provider)

	var err error
	appID := ""
	switch provider {
	case api.Provider_Amp:
		appID = bs.AppID
	case api.Provider_FileSys:
		appID = filesys.AppID
	case api.Provider_Spotify:
		appID = amp_spotify.AppID
	}

	if appID == "" {
		return nil, arc.ErrCode_CellNotFound.Errorf("invalid %q arg: %q", api.KwArg_Provider, provider)
	}

	proxyApp, err := app.User().GetAppContext(appID, true)
	if err != nil {
		return nil, err
	}
	return proxyApp.PinCell(req)

}

/*

TODO: generalize what's in amp_spotify for amp "base" helper structs

type ampItem struct {
	arc.CellID
	title    string
	subtitle string
	glyph    arc.AssetRef
	playable arc.AssetRef
}

func (item *ampItem) ID() arc.CellID {
	return item.CellID
}
*/
