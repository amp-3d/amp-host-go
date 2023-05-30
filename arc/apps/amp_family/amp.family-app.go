package amp_family

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp_bcat"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp_filesys"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp_spotify"
)

const (
	AppURI = amp.AppFamily + "_/v1"
)

func UID() arc.UID {
	return arc.FormUID(0x05e2e9b7c27641a6, 0xad1f1ff42f420add)
}

func RegisterFamily(reg arc.Registry) {
	amp_bcat.RegisterApp(reg)
	amp_filesys.RegisterApp(reg)
	amp_spotify.RegisterApp(reg)

	reg.RegisterApp(&arc.AppModule{
		URI:        AppURI,
		UID:        UID(),
		Desc:       "Arc Media Player (AMP)",
		Version:    "v1.2023.2",
		DataModels: amp.DataModels,
		Dependencies: []arc.UID{
			amp_bcat.UID(),
			amp_filesys.UID(),
			amp_spotify.UID(),
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
	provider, _ := req.GetKwArg(amp.KwArg_Provider)
	
	var err error
	appID := arc.UID{}
	switch provider {
	case amp.Provider_Amp:
		appID = amp_bcat.UID()
	case amp.Provider_FileSys:
		appID = amp_filesys.UID()
	case amp.Provider_Spotify:
		appID = amp_spotify.UID()
	default:
		return nil, arc.ErrCode_CellNotFound.Errorf("invalid %q arg: %q", amp.KwArg_Provider, provider)
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
