package bs

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/arcspace/go-arcspace/arc"
	"github.com/arcspace/go-arcspace/arc/apps/amp/api"
)

type bsApp struct {
	client *http.Client

	categories *stationCategories
}

func (app *bsApp) AppURI() string {
	return AppURI
}

func (app *bsApp) CellDataModels() []string {
	return []string{
		api.CellDataModel_Playlist,
		api.CellDataModel_Playable,
	}
}

func (app *bsApp) loadTokens(user arc.User) (arc.AppCell, error) {

	// TODO: the right way to do this is like in the Unity client: register all ValTypes and then dynamically build AttrSpecs
	// Since we're not doing that, for onw just build an AttrSpec from primitive types.

	//usr.MakeSchemaForStruct(app, LoginInfo)

	// Pins the named cell relative to the user's home planet and app URI (guaranteeing app and user scope)
	var login api.LoginInfo
	err := user.ReadCell(app, ".bookmark-server-client-login", &login)
	if err != nil {
		if arc.GetErrCode(err) == arc.ErrCode_CellNotFound {

		}
	}

	return nil, nil
}

func (app *bsApp) resetLogin() {

}

func (app *bsApp) PinCell(req *arc.CellReq) error {

	if req.CellID == 0 {
		if app.categories == nil {
			app.categories = &stationCategories{
				app:            app,
				CellID:         req.IssueCellID(),
				catsByServerID: make(map[uint32]*category),
				catsByCellID:   make(map[arc.CellID]*category),
			}
		}
		req.Cell = app.categories
	} else {
		panic("ampApp should have caught this")
		// if req.ParentReq == nil || req.ParentReq.Cell == nil {
		// 	return arc.ErrCode_InvalidCell.Error("missing parent cell")
		// }

		// if err := req.ParentReq.Cell.PinCell(req); err != nil {
		// 	return err
		// }
	}

	return nil
}

const (
	kTokenHack = "cd19b0da9069086d1ec3b4acf01d7bd77110a333"
	kUsername  = "DrewZ"
	kPassword  = "trdtrtvrtretttetrbrtbertb"
)

func (app *bsApp) makeReady() error {

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

type stationCategories struct {
	arc.CellID

	app            *bsApp
	catsByServerID map[uint32]*category
	catsByCellID   map[arc.CellID]*category
}

func (categories *stationCategories) loadCategories(cellReq *arc.CellReq) error {

	if err := categories.app.makeReady(); err != nil {
		return err
	}

	params := url.Values{}
	params.Add("subtype", "S")

	url := fmt.Sprintf("%s%s?%s", "https://api.soundspectrum.com/v1/", "categories/", params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err // SERVER DOWN error
	}
	req.Header = map[string][]string{
		"Authorization": {"Token " + kTokenHack},
	}
	resp, err := categories.app.client.Do(req)
	if err != nil {
		return err // SERVER DOWN error
	}

	jsonBody := json.NewDecoder(resp.Body)

	// read '['
	_, err = jsonBody.Token()
	if err != nil {
		log.Fatal(err)
	}

	// while the array contains values
	for jsonBody.More() {
		var entry api.CategoryInfo
		err := jsonBody.Decode(&entry)
		if err != nil {
			log.Print(err)
			break
		}
		if entry.Title == "Unlisted" {
			continue
		}
		cat := &category{
			app:          categories.app,
			CategoryInfo: entry,
			CellID:       cellReq.IssueCellID(),
		}
		categories.catsByServerID[entry.Id] = cat
		categories.catsByCellID[cat.CellID] = cat
	}

	// read ']'
	_, err = jsonBody.Token()
	if err != nil {
		log.Print(err)
	}

	return nil
}

func (categories *stationCategories) ID() arc.CellID {
	return categories.CellID
}

func (categories *stationCategories) CellDataModel() string {
	return api.CellDataModel_Playlist
}

func (categories *stationCategories) PushCellState(req *arc.CellReq) error {
	schema := req.ContentSchema

	if schema == nil {
		return nil
	}

	req.PushAttr(categories.CellID, schema, api.Attr_Title, "Internet Radio")

	if len(categories.catsByCellID) == 0 {
		categories.loadCategories(req)
	}

	for _, cat := range categories.catsByCellID {
		cat.pushCellState(req, true)
	}

	return nil
}

// TODO: use generics
func (categories *stationCategories) PinCell(req *arc.CellReq) error {
	if req.CellID == categories.CellID {
		req.Cell = categories // FUTURE: a pinned dir returns more detailed attrs (e.g. reads mpeg tags)
		return nil
	}

	cat := categories.catsByCellID[req.CellID]
	if cat == nil {
		return arc.ErrCode_InvalidCell.Error("invalid child cell")
	}

	req.Cell = cat
	return nil
}

type category struct {
	//playlist
	arc.CellID

	api.CategoryInfo

	app       *bsApp
	itemsByID map[arc.CellID]*station
}

func (cat *category) ID() arc.CellID {
	return cat.CellID
}

func (cat *category) CellDataModel() string {
	return api.CellDataModel_Playlist
}

func (cat *category) PushCellState(req *arc.CellReq) error {
	return cat.pushCellState(req, false)

	// if len(categories.catsByCellID) == 0 {
	// 	categories.loadCategories(req)
	// }

	// for _, cat := range categories.catsByCellID {
	// 	cat.pushCellState(req, true)
	// }

}

func (cat *category) pushCellState(req *arc.CellReq, asChild bool) error {
	var schema *arc.AttrSchema
	if asChild {
		schema = req.GetChildSchema(api.CellDataModel_Playlist)
	} else {
		schema = req.ContentSchema
	}
	if schema == nil {
		return nil
	}

	if asChild {
		req.PushInsertCell(cat.CellID, schema)
	}

	req.PushAttr(cat.CellID, schema, api.Attr_Title, cat.CategoryInfo.Title)
	req.PushAttr(cat.CellID, schema, api.Attr_Subtitle, cat.CategoryInfo.Description)

	glyph := arc.AssetRef{
		URI:    cat.CategoryInfo.Image,
		Scheme: arc.URIScheme_File,
	}
	req.PushAttr(cat.CellID, schema, api.Attr_Glyph, &glyph)

	if !asChild {
		if len(cat.itemsByID) == 0 {
			err := cat.loadItems(req)
			if err != nil {
				return err
			}
		}

		// // Refresh if first time or too old
		// now := time.Now()
		// if dir.lastRefresh.Before(now.Add(-time.Minute)) {
		// 	dir.lastRefresh = now
		// 	dir.readDir(req)
		// }

		// // Push the dir as the content item (vs child)
		// dir.pushCellState(req, false)

		// // Push each dir sub item as a child cell
		for _, item := range cat.itemsByID {
			item.pushCellState(req, true)
		}
	}

	return nil
}

func (cat *category) PinCell(req *arc.CellReq) error {

	if len(cat.itemsByID) == 0 {
		err := cat.loadItems(req)
		if err != nil {
			return err
		}
	}

	if req.CellID == cat.CellID {
		req.Cell = cat // FUTURE: a pinned dir returns more detailed attrs (e.g. reads mpeg tags)
		return nil
	}

	req.Cell = cat.itemsByID[req.CellID]
	return nil
}

func (cat *category) loadItems(cellReq *arc.CellReq) error {

	if cat.itemsByID == nil {
		cat.itemsByID = map[arc.CellID]*station{}
	}
	if err := cat.app.makeReady(); err != nil {
		return err
	}

	params := url.Values{}
	params.Add("subtype", "S")
	params.Add("categories", strconv.Itoa(int(cat.Id)))

	url := fmt.Sprintf("%s%s?%s", "https://api.soundspectrum.com/v1/", "bookmarks/", params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err // SERVER DOWN error
	}
	req.Header = map[string][]string{
		"Authorization": {"Token " + kTokenHack},
	}
	resp, err := cat.app.client.Do(req)
	if err != nil {
		return err // SERVER DOWN error
	}

	jsonBody := json.NewDecoder(resp.Body)

	// read '['
	_, err = jsonBody.Token()
	if err != nil {
		log.Fatal(err)
	}

	// while the array contains values
	for jsonBody.More() {
		var entry api.StationInfo
		err := jsonBody.Decode(&entry)
		if err != nil {
			log.Print(err)
			break
		}
		sta := &station{
			app:         cat.app,
			StationInfo: entry,
			CellID:      cellReq.IssueCellID(),
		}
		cat.itemsByID[sta.CellID] = sta

		//fmt.Printf("%v\n", entry)

	}

	// read ']'
	_, err = jsonBody.Token()
	if err != nil {
		log.Print(err)
	}

	return nil
}

type station struct {
	//playlist
	arc.CellID

	api.StationInfo

	app *bsApp

	//items []*station
}

func (sta *station) ID() arc.CellID {
	return sta.CellID
}

func (sta *station) CellDataModel() string {
	return api.CellDataModel_Playable
}

func (sta *station) PushCellState(req *arc.CellReq) error {
	return sta.pushCellState(req, false)
}

func (sta *station) pushCellState(req *arc.CellReq, asChild bool) error {
	var schema *arc.AttrSchema
	if asChild {
		schema = req.GetChildSchema(api.CellDataModel_Playable)
	} else {
		schema = req.ContentSchema
	}
	if schema == nil {
		return nil
	}

	if asChild {
		req.PushInsertCell(sta.CellID, schema)
	}

	req.PushAttr(sta.CellID, schema, api.Attr_Title, sta.StationInfo.Title)
	req.PushAttr(sta.CellID, schema, api.Attr_Subtitle, sta.StationInfo.Summary)

	glyph := arc.AssetRef{
		URI: sta.StationInfo.Image,
	}
	req.PushAttr(sta.CellID, schema, api.Attr_Glyph, &glyph)

	// [<URL>[:::<kbitrate>[:::<MIME type>]];]*
	// Just choose the first URL for now
	URL := sta.StationInfo.Url
	if N := strings.IndexByte(URL, ';'); N > 0 {
		URL = URL[:N]
	}
	parts := strings.Split(URL, ":::")

	if len(parts) > 0 && len(parts[0]) > 0 {
		playable := arc.AssetRef{
			URI: parts[0],
		}
		if len(parts) >= 3 {
			playable.MediaType = parts[2]
		} else {
			playable.MediaType = "audio/unknown"
		}

		req.PushAttr(sta.CellID, schema, api.Attr_Playable, &playable)
	}

	// // Refresh if first time or too old
	// now := time.Now()
	// if dir.lastRefresh.Before(now.Add(-time.Minute)) {
	// 	dir.lastRefresh = now
	// 	dir.readDir(req)
	// }

	// // Push the dir as the content item (vs child)
	// dir.pushCellState(req, false)

	// // Push each dir sub item as a child cell
	// for _, itemID := range dir.items {
	// 	dir.itemsByID[itemID].pushCellState(req, true)
	// }

	return nil
}

func (sta *station) PinCell(req *arc.CellReq) error {
	// if err := file.setPathnameUsingParent(req); err != nil {
	// 	return err
	// }

	// In the future pinning a file can do fancy things but for now, just use the same item
	if req.CellID == sta.CellID {
		req.Cell = sta
		return nil
	}

	return arc.ErrCode_InvalidCell.Error("item is a file; no children to pin")
}
