package amp

import (
	"sync/atomic"

	"github.com/arcspace/go-arcspace/arc"
)

type ampApp struct {
	nextID uint64
}

func (app *ampApp) AppURI() string {
	return AppURI
}

var CellDataModels = []string{
	"",
	CellDataModel_Playlist,
	CellDataModel_Playable,
}

func (app *ampApp) CellDataModels() []string {
	return CellDataModels[1:]
}

// Issues a new and unique ID that will persist during runtime
func (app *ampApp) IssueCellID() arc.CellID {
	return arc.CellID(atomic.AddUint64(&app.nextID, 1) + 2701)
}

func (app *ampApp) ResolveRequest(req *arc.CellReq) error {
	var err error

	if req.PinCell == 0 {
		pinRoot, _ := req.GetKwArg(KwArg_PinRoot)

		switch pinRoot {
		case "home":
			//req.PinnedCell, err = app.pinRadioHome(req.User)
			req.PinnedCell = pinSpotifyHome(app)
		case "filesys":
			req.PinnedCell = pinSpotifyHome(app)
		default:
			return arc.ErrCode_InvalidCell.Errorf("invalid %q arg", KwArg_PinRoot)
		}

	} else {
		if req.ParentReq == nil || req.ParentReq.PinnedCell == nil {
			return arc.ErrCode_InvalidCell.Error("missing parent")
		}

		parent, ok := req.ParentReq.PinnedCell.(*playlist)
		if !ok {
			return arc.ErrCode_NotPinnable.Error("invalid parent")
		}

		item := parent.itemsByID[req.PinCell]
		if item == nil {
			return arc.ErrCode_InvalidCell.Error("invalid target cell")
		}

		req.PinnedCell = item
		// item = *itemRef
		// item.pathname = path.Join(parent.pathname, item.name
	}

	return err
}

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

func pushCell(req *arc.CellReq, cell ampCell, asChild bool) error {
	var schema *arc.AttrSchema
	if asChild {
		schema = req.GetChildSchema(cell.CellDataModel())
	} else {
		schema = req.ContentSchema
	}

	if schema == nil {
		return nil
	}

	item := cell.item()

	if asChild {
		req.PushInsertCell(item.CellID, schema)
	}

	req.PushAttr(item.CellID, schema, Attr_ItemName, item.title)
	req.PushAttr(item.CellID, schema, Attr_Subtitle, item.subtitle)
	req.PushAttr(item.CellID, schema, Attr_Glyph, &item.glyph)
	req.PushAttr(item.CellID, schema, Attr_Playable, &item.playable)

	return nil
}

// }

type playlist struct {
	ampItem
	items     []arc.CellID // ordered
	itemsByID map[arc.CellID]ampCell
}

func (pl *playlist) init(app *ampApp) {
	pl.CellID = app.IssueCellID()
	pl.glyph.MediaType = MimeType_Dir
	pl.items = make([]arc.CellID, 0, 32)
	pl.itemsByID = make(map[arc.CellID]ampCell, 32)
}

func (pl *playlist) addItem(cell ampCell) {
	cellID := cell.item().CellID
	pl.items = append(pl.items, cellID)
	pl.itemsByID[cellID] = cell
}

func (pl *playlist) item() ampItem {
	return pl.ampItem
}

func (pl *playlist) CellDataModel() string {
	return CellDataModel_Playlist
}

func (pl *playlist) PushCellState(req *arc.CellReq) error {
	return pl.PushCell(req, false)
}

func (pl *playlist) PushCell(req *arc.CellReq, asChild bool) error {
	if err := pushCell(req, pl, asChild); err != nil {
		return err
	}

	if !asChild {
		for _, itemID := range pl.items {
			item := pl.itemsByID[itemID]
			err := item.PushCell(req, true)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func pinSpotifyHome(app *ampApp) arc.AppPinnedCell {
	sp := &playlist{}

	sp.init(app)
	sp.title = "Music Library"
	sp.glyph.MediaType = MimeType_Dir

	// subs := []string{
	// 	"Playlists", "Artists", "Albums", "Songs", "Genres", "New Releases", "Podcasts", "Internet Radio"}

	// for _, title := range subs {
	// 	item := &playlist{}
	// 	item.init(app)
	// 	item.title = title
	// 	sp.addItem(item)
	// }

	stations := []string{
		"Frisky",
		"feelin frisky?",
		"https://stream.frisky.friskyradio.com/aac_hi",

		"Ancient FM",
		"Musick of the Medieval and Renaissance",
		"http://stream.ancientfm.com:8058/stream",
	}

	for i := 0; i < len(stations); i += 3 {
		item := &playable{}
		item.title = stations[i]
		item.subtitle = stations[i+1]
		//item.glyph.MediaType = "audio/mpegurl"
		item.playable.MediaType = "audio/mpegurl"
		item.playable.URI = stations[i+2]
		item.CellID = app.IssueCellID()
		sp.addItem(item)
	}
	return sp
}

type ampCell interface {
	arc.AppPinnedCell

	item() ampItem
	CellDataModel() string
	PushCell(req *arc.CellReq, asChild bool) error
	
}

type playable struct {
	ampItem
}

func (item *playable) item() ampItem {
	return item.ampItem
}

func (item *playable) CellDataModel() string {
	return CellDataModel_Playable
}

func (item *playable) PushCellState(req *arc.CellReq) error {
	return item.PushCell(req, false)
}

func (item *playable) PushCell(req *arc.CellReq, asChild bool) error {
	return pushCell(req, item, asChild)
}
