package amp_spotify

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	arc_sdk "github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp/api"
	respot "github.com/arcspace/go-librespot/librespot/api-respot"
	"github.com/zmb3/spotify/v2"
)

type ampAttr struct {
	attrID string
	val    interface{}
	valSI  string
}

type ampCell struct {
	arc.CellID
	app      *appCtx
	loadedAt arc.TimeFS
	attrs    []ampAttr
	self     AmpCell

	childCells []AmpCell
	//childByID     map[arc.CellID]arc.AppCell
}

func (cell *ampCell) init(self AmpCell, app *appCtx) {
	cell.app = app
	cell.self = self
	cell.CellID = app.IssueCellID()
	cell.attrs = make([]ampAttr, 0, 4)
}

func (cell *ampCell) ID() arc.CellID {
	return cell.CellID
}

func (cell *ampCell) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	var schema *arc.AttrSchema
	if opts.PushAsChild() {
		schema = req.GetChildSchema(cell.self.CellDataModel())
		req.PushInsertCell(cell.CellID, schema)
	} else if opts.PushAsParent() {
		schema = req.ContentSchema
	}
	if schema == nil {
		return nil
	}

	// Push this cells attributes according to what is requested by the schema
	// Future: use a channel to push attrs for better pipelining
	for _, attr := range cell.attrs {
		req.PushAttr(cell.CellID, schema, attr.attrID, attr.val)
	}

	if opts.PushAsParent() {
		if cell.loadedAt == 0 {
			cell.loadedAt = arc.TimeNowFS()
			cell.childCells = cell.childCells[:0]

			err := cell.self.loadChildren(req)
			if err != nil {
				return err
			}
		}

		for _, child := range cell.childCells {
			//child := cell.childByID[subID]
			err := child.PushCellState(req, arc.PushAsChild)
			if err != nil {
				cell.app.Warnf("pushCellState: %v", err)
			}
		}
	}

	return nil
}

func (cell *ampCell) PinCell(req *arc.CellReq) (arc.AppCell, error) {
	if req.PinCell == cell.CellID {
		return cell.self.pinSelf(req)
	}

	// TODO: build a map on demand when needed
	for _, child := range cell.childCells {
		if child.ID() == req.PinCell {
			return child.pinSelf(req)
		}
	}
	return nil, arc.ErrCellNotFound
}

func (cell *ampCell) pinSelf(req *arc.CellReq) (arc.AppCell, error) {
	return cell.self, nil
}

func (cell *ampCell) SetAttr(attrID string, val interface{}) {
	for i, attr := range cell.attrs {
		if attr.attrID == attrID {
			cell.attrs[i] = ampAttr{
				attrID: attrID,
				val:    val,
			}
			return
		}
	}
	cell.AddAttr(attrID, val)
}

func (cell *ampCell) AddAttr(attrID string, val interface{}) {
	cell.attrs = append(cell.attrs, ampAttr{
		attrID: attrID,
		val:    val,
	})
}

type AmpCell interface {
	arc.AppCell

	pinSelf(req *arc.CellReq) (arc.AppCell, error)
	loadChildren(req *arc.CellReq) error
}

type rootCell struct {
	ampCell
}

func newRootCell(app *appCtx) *rootCell {
	cell := &rootCell{}
	cell.init(cell, app)
	return cell
}

func (cell *rootCell) CellDataModel() string {
	return api.CellDataModel_Dir
}

func (cell *rootCell) loadChildren(req *arc.CellReq) error {
	app := cell.app
	if err := app.waitForSession(); err != nil {
		return err
	}

	lists, err := app.client.CurrentUsersPlaylists(app)
	if err != nil {
		return arc.ErrCode_ProviderErr.Errorf("failed to get current user: %v", err)
	}

	for _, list := range lists.Playlists {
		child := newPlaylistCell(app, list)
		cell.childCells = append(cell.childCells, child)
	}
	return nil
}

const kGlyphPixelSz = 128

type playlistCell struct {
	ampCell
	playlist spotify.SimplePlaylist
}

func newPlaylistCell(app *appCtx, playlist spotify.SimplePlaylist) *playlistCell {
	cell := &playlistCell{
		playlist: playlist,
	}
	cell.init(cell, app)

	cell.AddAttr(api.Attr_Title, playlist.Name)
	cell.AddAttr(api.Attr_Subtitle, playlist.Description)
	if glyph := chooseBestGlyph(playlist.Images, kGlyphPixelSz); glyph != nil {
		cell.AddAttr(api.Attr_Glyph, glyph)
	}
	return cell
}

func (cell *playlistCell) CellDataModel() string {
	return api.CellDataModel_Playlist
}

func (cell *playlistCell) loadChildren(req *arc.CellReq) error {
	app := cell.app
	if err := app.waitForSession(); err != nil {
		return err
	}

	itemPage, err := app.client.GetPlaylistItems(app, cell.playlist.ID)
	if err != nil {
		return arc.ErrCode_ProviderErr.Errorf("failed to get playlist: %v", err)
	}

	for _, item := range itemPage.Items {
		var child AmpCell
		if item.Track.Track != nil {
			child = newTrackCell(app, item.Track.Track)
		} else if item.Track.Episode != nil {
			// TODO: handle episodes
		}
		if child != nil {
			cell.childCells = append(cell.childCells, child)
		}
	}
	return nil
}

type trackCell struct {
	ampCell
	assetURI string
	track    *spotify.FullTrack
}

func newTrackCell(app *appCtx, track *spotify.FullTrack) *trackCell {
	if track.IsPlayable != nil && !*track.IsPlayable {
		return nil
	}
	cell := &trackCell{
		track: track,
	}
	cell.init(AmpCell(cell), app)
	cell.AddAttr(api.Attr_Title, track.Name)
	{
		artistDesc := ""
		if len(track.Artists) > 0 {
			artistDesc = track.Artists[0].Name
			for _, artist := range track.Artists[1:] {
				artistDesc += ", " + artist.Name
			}
		}
		if len(artistDesc) > 0 {
			cell.AddAttr(api.Attr_ArtistDesc, artistDesc)
			cell.AddAttr(api.Attr_Subtitle, artistDesc)
		}
	}

	if len(track.Album.Name) > 0 {
		cell.AddAttr(api.Attr_AlbumDesc, track.Album.Name)
	}

	if glyph := chooseBestGlyph(track.Album.Images, kGlyphPixelSz); glyph != nil {
		cell.AddAttr(api.Attr_Glyph, glyph)
	}

	// cell.AddAttr(api.Attr_Playable, &arc.AssetRef{
	// 	MediaType: "audio/x-spotify",
	// 	URI:       string(track.URI),
	// })

	return cell
}

func (cell *trackCell) CellDataModel() string {
	return api.CellDataModel_Playable
}

func (cell *trackCell) pinSelf(req *arc.CellReq) (arc.AppCell, error) {
	asset, err := cell.app.respot.PinTrack(string(cell.track.URI), respot.PinOpts{})
	if err != nil {
		return nil, err
	}

	assetRef := &arc.AssetRef{}
	assetRef.URI, err = cell.app.PublishAsset(asset, arc_sdk.PublishOpts{})
	if err != nil {
		return nil, err
	}
	assetRef.MediaType = asset.MediaType()
	cell.SetAttr(api.Attr_Playable, assetRef)

	return cell.self, nil
}

// Must be present for above PinCell() override to work
func (cell *trackCell) loadChildren(req *arc.CellReq) error {
	return nil
}

/**********************************************************
*  Helpers
 */

func chooseBestGlyph(images []spotify.Image, closestSize int) *arc.AssetRef {
	if len(images) == 0 {
		return nil
	}
	bestImg := -1
	bestDiff := 0x7fffffff

	for i, img := range images {
		diff := img.Width - closestSize

		// If the image is smaller than what we're looking for, make differences matter more
		if diff < 0 {
			diff *= -4
		}

		if diff < bestDiff {
			bestImg = i
			bestDiff = diff
		}
	}
	return &arc.AssetRef{
		MediaType: "image/x-spotify",
		URI:       images[bestImg].URL,
		PixWidth:  int32(images[bestImg].Width),
		PixHeight: int32(images[bestImg].Height),
	}
}
