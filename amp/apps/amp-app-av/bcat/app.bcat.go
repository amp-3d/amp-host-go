package bcat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/av"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/basic"
	"github.com/amp-3d/amp-sdk-go/amp/registry"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
)

var (
	AppSpec         = tag.FormSpec(av.AppSpec, "bookmark-catalog")
	LoginInfoAttrID = tag.FormSpec(AppSpec, "LoginInfo").ID
)

func init() {
	reg := registry.Global()

	reg.RegisterApp(&amp.App{
		AppSpec: AppSpec,
		Desc:    "bookmark catalog service",
		Version: "v1.2023.2",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			app := &appInst{}
			app.Instance = app
			app.AppContext = ctx
			return app, nil
		},
	})
}

type appInst struct {
	basic.App[*appInst]
	client *http.Client
	cats   categories
}

func (app *appInst) readStoredToken() error {

	// Pins the named cell relative to the user's home planet and appID (guaranteeing app and user scope)
	login := &av.LoginInfo{}
	err := app.GetAppAttr(LoginInfoAttrID, login)
	if err != nil {

	}

	return nil
}

func (app *appInst) resetLogin() {

}

func (app *appInst) MakeReady(req amp.Requester) error {
	return app.makeReady()
}

func (app *appInst) ServeRequest(op amp.Requester) (amp.Pin, error) {
	return app.PinAndServe(&app.cats, op)
}

const (
	kTokenHack = "cd19b0da9069086d1ec3b4acf01d7bd77110a333"
	kUsername  = "DrewZ"
	kPassword  = "trdtrtvrtretttetrbrtbertb"
)

func (app *appInst) makeReady() error {
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

func (app *appInst) doReq(endpoint string, params url.Values) (*json.Decoder, error) {
	if err := app.makeReady(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s?%s", "https://amp.soundspectrum.com/v1/", endpoint, params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err // SERVER DOWN error
	}
	req.Header = map[string][]string{
		"Authorization": {"Token " + kTokenHack},
	}
	resp, err := app.client.Do(req)
	if err != nil {
		return nil, err // SERVER DOWN error
	}

	jsonDecoder := json.NewDecoder(resp.Body)
	return jsonDecoder, nil
}
