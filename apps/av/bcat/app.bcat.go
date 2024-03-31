package bcat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/av"
)

const (
	AppID = "bookmark-catalog" + av.AppFamilyDomain
)

const kTokenAttrSpec = "LoginInfo:client-login"

func UID() arc.UID {
	return arc.FormUID(0xd2849a95ddb047b3, 0xa787d8a52d039c32)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.App{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "bookmark catalog service",
		Version: "v1.2023.2",
		NewAppInstance: func() arc.AppInstance {
			return &appCtx{}
		},
	})
}

type appCtx struct {
	av.AppBase
	client *http.Client
	cats   []*categoryInfo
}

func (app *appCtx) readStoredToken() error {

	// Pins the named cell relative to the user's home planet and appID (guaranteeing app and user scope)
	login := &av.LoginInfo{}
	err := app.GetAppCellAttr(kTokenAttrSpec, login)
	if err != nil {

	}

	return nil
}

func (app *appCtx) resetLogin() {

}

func (app *appCtx) PinCell(parent arc.PinnedCell, req arc.PinReq) (arc.PinnedCell, error) {

	if app.cats == nil {
		err := app.reloadCategories()
		if err != nil {
			return nil, err
		}
	}

	cats := &categories{
		//cells: make([]*amp.CellBase[*appCtx], 0, 16),
	}
	cats.Self = nil // cats
	return av.NewPinnedCell[*appCtx](app, &cats.CellBase)
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

func (app *appCtx) doReq(endpoint string, params url.Values) (*json.Decoder, error) {
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

func (app *appCtx) reloadCategories() error {
	params := url.Values{}
	params.Add("subtype", "S")

	json, err := app.doReq("categories/", params)
	if err != nil {
		return err
	}

	// read '['
	_, err = json.Token()
	if err != nil {
		return err
	}

	// while the array contains values
	for json.More() {
		var entry av.CategoryInfo
		err := json.Decode(&entry)
		if err != nil {
			return err
		}
		if entry.Title == "Unlisted" {
			continue
		}
		cat := &categoryInfo{
			catID: entry.Id,
		}

		cat.hdr = arc.CellHeader{
			Title: entry.Title,
			About: entry.Description,
			Glyphs: []*arc.AssetTag{
				{
					URI:         "amp:asset/av/" + entry.Image,
					ContentType: arc.GenericImageType,
				},
			},
		}

		if created, err := time.Parse(time.RFC3339, entry.TimestampCreated); err == nil {
			cat.hdr.Created = int64(arc.ConvertToUTC16(created))
		}
		if modified, err := time.Parse(time.RFC3339, entry.TimestampModified); err == nil {
			cat.hdr.Modified = int64(arc.ConvertToUTC16(modified))
		}
		app.cats = append(app.cats, cat)
	}

	// read ']'
	_, err = json.Token()
	if err != nil {
		return err
	}

	return nil
}

type categoryInfo struct {
	hdr   arc.CellHeader
	catID uint32
}
