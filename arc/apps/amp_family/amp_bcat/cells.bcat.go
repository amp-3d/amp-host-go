package amp_bcat

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
)

type stationCategories struct {
	//process.Context
	arc.CellID

	app            *appCtx
	catsByServerID map[uint32]*category
	catsByCellID   map[arc.CellID]*category
}

func (categories *stationCategories) loadCategories(cellReq *arc.CellReq) error {

	if err := categories.app.makeReady(); err != nil {
		return err
	}

	params := url.Values{}
	params.Add("subtype", "S")

	url := fmt.Sprintf("%s%s?%s", "https://amp.soundspectrum.com/v1/", "categories/", params.Encode())
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
		var entry amp.CategoryInfo
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
			CellID:       categories.app.IssueCellID(),
			CategoryInfo: entry,
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
	return amp.CellDataModel_Playlist
}

func (categories *stationCategories) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	schema := req.ContentSchema

	if schema == nil {
		return nil
	}

	req.PushAttr(categories.CellID, schema, amp.Attr_Title, arc.AttrStr("Internet Radio"))

	if len(categories.catsByCellID) == 0 {
		categories.loadCategories(req)
	}

	for _, cat := range categories.catsByCellID {
		cat.PushCellState(req, arc.PushAsChild)
	}

	return nil
}

// TODO: use generics
func (categories *stationCategories) PinCell(req *arc.CellReq) (arc.Cell, error) {
	if req.PinCell == categories.CellID {
		return categories, nil
	}

	cat := categories.catsByCellID[req.PinCell]
	if cat == nil {
		return nil, arc.ErrCellNotFound
	}

	return cat, nil
}

type category struct {
	//playlist
	arc.CellID

	amp.CategoryInfo

	app       *appCtx
	itemsByID map[arc.CellID]*station
}

func (cat *category) ID() arc.CellID {
	return cat.CellID
}

func (cat *category) CellDataModel() string {
	return amp.CellDataModel_Playlist
}

func (cat *category) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	var schema *arc.AttrSchema
	if opts.PushAsChild() {
		schema = req.GetChildSchema(amp.CellDataModel_Playlist)
	} else {
		schema = req.ContentSchema
	}
	if schema == nil {
		return nil
	}

	if opts.PushAsChild() {
		req.PushInsertCell(cat.CellID, schema)
	}

	req.PushAttr(cat.CellID, schema, amp.Attr_Title, arc.AttrStr(cat.CategoryInfo.Title))
	req.PushAttr(cat.CellID, schema, amp.Attr_Subtitle, arc.AttrStr(cat.CategoryInfo.Description))

	glyph := arc.AssetRef{
		URI:    cat.CategoryInfo.Image,
		Scheme: arc.URIScheme_File,
	}
	req.PushAttr(cat.CellID, schema, amp.Attr_Glyph, &glyph)

	if opts.PushAsParent() {
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
			item.PushCellState(req, arc.PushAsChild)
		}
	}

	return nil
}

func (cat *category) PinCell(req *arc.CellReq) (arc.Cell, error) {

	if len(cat.itemsByID) == 0 {
		err := cat.loadItems(req)
		if err != nil {
			return nil, err
		}
	}

	if req.PinCell == cat.CellID {
		return cat, nil // FUTURE: a pinned dir returns more detailed attrs (e.g. reads mpeg tags)
	}

	cell := cat.itemsByID[req.PinCell]
	return cell, nil
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

	url := fmt.Sprintf("%s%s?%s", "https://amp.soundspectrum.com/v1/", "bookmarks/", params.Encode())
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
		var entry amp.StationInfo
		err := jsonBody.Decode(&entry)
		if err != nil {
			log.Print(err)
			break
		}
		sta := &station{
			app:         cat.app,
			CellID:      cat.app.IssueCellID(),
			StationInfo: entry,
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
	arc.CellID
	amp.StationInfo
	app *appCtx
}

func (sta *station) ID() arc.CellID {
	return sta.CellID
}

func (sta *station) CellDataModel() string {
	return amp.CellDataModel_Playable
}

func (sta *station) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	var schema *arc.AttrSchema
	if opts.PushAsChild() {
		schema = req.GetChildSchema(amp.CellDataModel_Playable)
	} else {
		schema = req.ContentSchema
	}
	if schema == nil {
		return nil
	}

	if opts.PushAsChild() {
		req.PushInsertCell(sta.CellID, schema)
	}

	req.PushAttr(sta.CellID, schema, amp.Attr_Title, arc.AttrStr(sta.StationInfo.Title))
	req.PushAttr(sta.CellID, schema, amp.Attr_Subtitle, arc.AttrStr(sta.StationInfo.Summary))

	// FIXME: the server prefixes /icons/ to the image string and we use that in the client to route to the _resources_ dir in the app crate, whew!
	glyph := arc.AssetRef{
		URI:    sta.StationInfo.Image,
		Scheme: arc.URIScheme_File,
	}
	req.PushAttr(sta.CellID, schema, amp.Attr_Glyph, &glyph)

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

		req.PushAttr(sta.CellID, schema, amp.Attr_Playable, &playable)
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

func (sta *station) PinCell(req *arc.CellReq) (arc.Cell, error) {
	// if err := file.setPathnameUsingParent(req); err != nil {
	// 	return err
	// }

	// In the future pinning a file can do fancy things but for now, just use the same item
	if req.PinCell == sta.CellID {
		return sta, nil
	}

	return nil, arc.ErrCellNotFound
}
