package amp

import (
	"github.com/arcspace/go-arcspace/arc"
	"github.com/arcspace/go-arcspace/arc/apps/amp/amp_spotify"
	"github.com/arcspace/go-arcspace/arc/apps/amp/api"
	"github.com/arcspace/go-arcspace/arc/apps/amp/bs"
	"github.com/arcspace/go-arcspace/arc/apps/amp/filesys"
)

func NewApp() arc.App {
	return &ampApp{}
}

type ampApp struct {
	fsApp      arc.App
	bsApp      arc.App
	spotifyApp arc.App
}

func (app *ampApp) AppURI() string {
	return api.AmpAppURI
}

func (app *ampApp) SupportedDataModels() []string {
	return api.SupportedDataModels
}

func (app *ampApp) PinCell(req *arc.CellReq) error {

	if req.CellID == 0 {
		provider, _ := req.GetKwArg(api.KwArg_Provider)

		switch provider {
		case api.Provider_Amp:
			if app.bsApp == nil {
				app.bsApp = bs.NewApp()
			}
			return app.bsApp.PinCell(req)
		case api.Provider_FileSys:
			if app.fsApp == nil {
				app.fsApp = filesys.NewApp()
			}
			return app.fsApp.PinCell(req)
		case api.Provider_Spotify:
			if app.spotifyApp == nil {
				app.bsApp = amp_spotify.NewApp()
			}
			return app.bsApp.PinCell(req)
		default:
			return arc.ErrCode_InvalidCell.Errorf("invalid %q arg: %q", api.KwArg_Provider, provider)
		}

	} else {
		if req.ParentReq == nil || req.ParentReq.Cell == nil {
			return arc.ErrCode_InvalidCell.Error("missing parent")
		}

		if err := req.ParentReq.Cell.PinCell(req); err != nil {
			return err
		}
	}

	return nil
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

func pushCell(req *arc.CellReq, cell ampCell, opts arc.PushCellOpts) error {
	var schema *arc.AttrSchema
	if opts.PushAsChild() {
		schema = req.GetChildSchema(cell.CellDataModel())
	} else {
		schema = req.ContentSchema
	}
	if schema == nil {
		return nil
	}

	item := cell.item()

	if opts.PushAsChild() {
		req.PushInsertCell(item.CellID, schema)
	}
	req.PushAttr(item.CellID, schema, api.Attr_Title, item.title)
	req.PushAttr(item.CellID, schema, api.Attr_Subtitle, item.subtitle)
	req.PushAttr(item.CellID, schema, api.Attr_Glyph, &item.glyph)
	req.PushAttr(item.CellID, schema, api.Attr_Playable, &item.playable)

	return nil
}

// }

type playlist struct {
	ampItem
	items     []arc.CellID // ordered
	itemsByID map[arc.CellID]ampCell
}

func (pl *playlist) init(req *arc.CellReq) {
	pl.CellID = req.IssueCellID()
	pl.glyph.MediaType = api.MimeType_Dir
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
	return api.CellDataModel_Playlist
}

func (pl *playlist) PinCell(req *arc.CellReq) error {
	if req.CellID == pl.CellID {
		req.Cell = pl
		return nil
	}

	item := pl.itemsByID[req.CellID]
	if item == nil {
		return arc.ErrCode_InvalidCell.Error("invalid playlist cell ID")
	}

	req.Cell = item
	return nil
}

func (pl *playlist) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	return pl.PushCell(req, opts)
}

func (pl *playlist) PushCell(req *arc.CellReq, opts arc.PushCellOpts) error {
	if err := pushCell(req, pl, opts); err != nil {
		return err
	}

	if opts.PushAsParent() {
		for _, itemID := range pl.items {
			item := pl.itemsByID[itemID]
			err := item.PushCell(req, arc.PushAsChild)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func pinSpotifyHome(req *arc.CellReq) arc.AppCell {
	sp := &playlist{}

	sp.init(req)
	sp.title = "Music Library"
	sp.glyph.MediaType = api.MimeType_Dir

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
		item.CellID = req.IssueCellID()
		sp.addItem(item)
	}
	return sp
}

// TODO: use generics instead
type ampCell interface {
	arc.AppCell

	item() ampItem

	PushCell(req *arc.CellReq, opts arc.PushCellOpts) error
}

type playable struct {
	ampItem
}

func (item *playable) item() ampItem {
	return item.ampItem
}

func (item *playable) CellDataModel() string {
	return api.CellDataModel_Playable
}

func (item *playable) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	return item.PushCell(req, opts)
}

func (item *playable) PinCell(req *arc.CellReq) error {
	if req.CellID == item.CellID {
		req.Cell = item
		return nil
	}

	return arc.ErrCode_InvalidCell.Error("playable item has no children to pin")
}

func (item *playable) PushCell(req *arc.CellReq, opts arc.PushCellOpts) error {
	return pushCell(req, item, opts)
}
