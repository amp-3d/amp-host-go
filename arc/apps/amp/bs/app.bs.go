package bs

import (
	"net/http"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp/api"
)

const (
	AppID = "arcspace.systems.app.amp.bookmark-service"
)

func RegisterApp(reg arc.Registry) { 
	reg.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		Version: "v1.2023.2",
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
	client     *http.Client
	categories *stationCategories
}

func (app *appCtx) HandleMetaMsg(msg *arc.Msg) (handled bool, err error) {
	return false, nil
}

func (app *appCtx) OnClosing() {
}

func (app *appCtx) loadTokens(user arc.User) (arc.AppCell, error) {

	// TODO: the right way to do this is like in the Unity client: register all ValTypes and then dynamically build AttrSpecs
	// Since we're not doing that, for onw just build an AttrSpec from primitive types.

	//usr.MakeSchemaForStruct(app, LoginInfo)

	// Pins the named cell relative to the user's home planet and appID (guaranteeing app and user scope)
	val, err := app.GetAppValue(".bookmark-server-client-login")
	var login api.LoginInfo
	if err == nil {
		err = login.Unmarshal(val)
	}
	if err != nil {
		if arc.GetErrCode(err) == arc.ErrCode_CellNotFound {

		}
	}

	return nil, nil
}

func (app *appCtx) resetLogin() {

}

func (app *appCtx) PinCell(req *arc.CellReq) (arc.AppCell, error) {

	if req.PinCell == 0 {
		if app.categories == nil {
			app.categories = &stationCategories{
				app:            app,
				CellID:         app.IssueCellID(),
				catsByServerID: make(map[uint32]*category),
				catsByCellID:   make(map[arc.CellID]*category),
			}
		}
		return app.categories, nil
	}

	return nil, nil
}

const (
	kTokenHack = "cd19b0da9069086d1ec3b4acf01d7bd77110a333"
	kUsername  = "DrewZ"
	kPassword  = "trdtrtvrtretttetrbrtbertb"
)

func (app *appCtx) makeReady() error {

	if app.client != nil {
		return nil
	}

	app.client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    5 * time.Second,
			DisableCompression: true,
		},
	}

	return nil
}
