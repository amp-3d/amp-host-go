package amp_spotify

import (
	"fmt"
	"strings"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
	respot "github.com/arcspace/go-librespot/librespot/api-respot"
	"github.com/zmb3/spotify/v2"
)

type ampAttr struct {
	attrID string
	val    interface{}
	valSI  string
}

type ampCell struct {
	//process.Context
	arc.CellID
	app         *appCtx
	loadedAt    arc.TimeFS
	attrs       []ampAttr
	self        AmpCell
	dataModelID string

	childCells []AmpCell
	//childByID     map[arc.CellID]arc.Cell
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

func (cell *ampCell) CellDataModel() string {
	return cell.dataModelID
}

func (cell *ampCell) PushCellState(req *arc.CellReq, opts arc.PushCellOpts) error {
	var schema *arc.AttrSchema
	if opts.PushAsChild() {
		schema = req.GetChildSchema(cell.dataModelID)
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

func (cell *ampCell) PinCell(req *arc.CellReq) (arc.Cell, error) {
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

func (cell *ampCell) pinSelf(req *arc.CellReq) (arc.Cell, error) {
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
	arc.Cell

	pinSelf(req *arc.CellReq) (arc.Cell, error)
	loadChildren(req *arc.CellReq) error
}

const kGlyphPixelSz = 128

func newRootCell(app *appCtx) *spotifyCell {
	cell := &spotifyCell{}
	cell.init(cell, app)
	cell.dataModelID = amp.CellDataModel_Dir
	cell.loader = reloadHome
	return cell
}

func reloadHome(cell *spotifyCell) error {
	app := cell.app
	if err := app.waitForSession(); err != nil {
		return err
	}

	child := newSpotifyCell(app)
	cell.childCells = append(cell.childCells, child)
	child.AddAttr(amp.Attr_Title, "Followed Playlists")
	child.AddAttr(amp.Attr_Glyph, &arc.AssetRef{
		URI:    "/icons/ui/providers/playlists.png",
		Scheme: arc.URIScheme_File,
	})
	child.dataModelID = amp.CellDataModel_Dir
	//child.AddAttr(amp.Attr_Subtitle, playlist.Description)
	// if glyph := chooseBestGlyph(playlist.Images, kGlyphPixelSz); glyph != nil {
	// 	cell.AddAttr(amp.Attr_Glyph, glyph)
	// }
	child.loader = func(parent *spotifyCell) error {
		resp, err := parent.app.client.CurrentUsersPlaylists(parent.app)
		if err != nil {
			return err
		}
		for i := range resp.Playlists {
			parent.newChildPlaylist(resp.Playlists[i])
		}
		return nil
	}

	child = newSpotifyCell(app)
	cell.childCells = append(cell.childCells, child)
	child.AddAttr(amp.Attr_Title, "Followed Artists")
	child.dataModelID = amp.CellDataModel_Dir
	child.AddAttr(amp.Attr_Glyph, &arc.AssetRef{
		URI:    "/icons/ui/providers/artists.png",
		Scheme: arc.URIScheme_File,
	})
	child.loader = func(parent *spotifyCell) error {
		resp, err := parent.app.client.CurrentUsersFollowedArtists(parent.app)
		if err != nil {
			return err
		}
		for i := range resp.Artists {
			parent.newChildArtist(resp.Artists[i])
		}
		return nil
	}

	child = newSpotifyCell(app)
	cell.childCells = append(cell.childCells, child)
	child.AddAttr(amp.Attr_Title, "Recently Played")
	child.dataModelID = amp.CellDataModel_Playlist
	child.AddAttr(amp.Attr_Glyph, &arc.AssetRef{
		URI:    "/icons/ui/providers/tracks.png",
		Scheme: arc.URIScheme_File,
	})
	child.loader = func(parent *spotifyCell) error {
		resp, err := parent.app.client.CurrentUsersTopTracks(parent.app)
		if err != nil {
			return err
		}
		for i := range resp.Tracks {
			parent.newChildTrack(resp.Tracks[i])
		}
		return nil
	}

	child = newSpotifyCell(app)
	cell.childCells = append(cell.childCells, child)
	child.AddAttr(amp.Attr_Title, "Recently Played Artists")
	child.dataModelID = amp.CellDataModel_Dir
	child.AddAttr(amp.Attr_Glyph, &arc.AssetRef{
		URI:    "/icons/ui/providers/artists.png",
		Scheme: arc.URIScheme_File,
	})
	child.loader = func(parent *spotifyCell) error {
		resp, err := parent.app.client.CurrentUsersTopArtists(parent.app)
		if err != nil {
			return err
		}
		for i := range resp.Artists {
			parent.newChildArtist(resp.Artists[i])
		}
		return nil
	}

	child = newSpotifyCell(app)
	cell.childCells = append(cell.childCells, child)
	child.AddAttr(amp.Attr_Title, "Saved Albums")
	child.dataModelID = amp.CellDataModel_Dir
	child.AddAttr(amp.Attr_Glyph, &arc.AssetRef{
		URI:    "/icons/ui/providers/albums.png",
		Scheme: arc.URIScheme_File,
	})
	child.loader = func(parent *spotifyCell) error {
		resp, err := parent.app.client.CurrentUsersAlbums(parent.app)
		if err != nil {
			return err
		}
		for i := range resp.Albums {
			parent.newChildAlbum(resp.Albums[i].SimpleAlbum)
		}
		return nil
	}
	
	// CurrentUsersShows

	return nil
}

type spotifyCell struct {
	ampCell
	spotifyID spotify.ID
	loader    func(cell *spotifyCell) error
}

func newSpotifyCell(app *appCtx) *spotifyCell {
	cell := &spotifyCell{}
	cell.init(cell, app)
	return cell
}

func (cell *spotifyCell) loadChildren(req *arc.CellReq) error {
	app := cell.app
	if err := app.waitForSession(); err != nil {
		return err
	}

	if err := cell.loader(cell); err != nil {
		return err
	}
	return nil
}

type playlistCell struct {
	spotifyCell
	//playlist spotify.SimplePlaylist
}

func playlistLoader(parent *spotifyCell) error {
	resp, err := parent.app.client.GetPlaylistItems(parent.app, parent.spotifyID)
	if err != nil {
		return err
	}
	for _, item := range resp.Items {
		if item.Track.Track != nil {
			parent.newChildTrack(*item.Track.Track)
		} else if item.Track.Episode != nil {
			// TODO: handle episodes
		}
	}
	return nil
}

func (parent *spotifyCell) newChildPlaylist(playlist spotify.SimplePlaylist) *playlistCell {
	cell := &playlistCell{}
	cell.init(cell, parent.app)
	cell.spotifyID = playlist.ID
	cell.loader = playlistLoader
	cell.dataModelID = amp.CellDataModel_Playlist

	cell.AddAttr(amp.Attr_Title, playlist.Name)
	cell.AddAttr(amp.Attr_Subtitle, playlist.Description)
	if glyph := chooseBestGlyph(playlist.Images, kGlyphPixelSz); glyph != nil {
		cell.AddAttr(amp.Attr_Glyph, glyph)
	}

	parent.childCells = append(parent.childCells, cell)
	return cell
}

type artistCell struct {
	spotifyCell
}

var allAlbumTypes = []spotify.AlbumType{
	spotify.AlbumTypeAlbum,
	spotify.AlbumTypeSingle,
	spotify.AlbumTypeAppearsOn,
	spotify.AlbumTypeCompilation,
}

func artistLoader(parent *spotifyCell) error {
	resp, err := parent.app.client.GetArtistAlbums(parent.app, parent.spotifyID, allAlbumTypes)
	if err != nil {
		return err
	}
	for i := range resp.Albums {
		parent.newChildAlbum(resp.Albums[i])
	}
	return nil
}

func albumLoader(parent *spotifyCell) error {
	resp, err := parent.app.client.GetAlbum(parent.app, parent.spotifyID)
	if err != nil {
		return err
	}
	for _, track := range resp.Tracks.Tracks {
		parent.newChildTrack(spotify.FullTrack{
			SimpleTrack: track,
			Album:       resp.SimpleAlbum,
		})
	}
	return nil
}

func (parent *spotifyCell) newChildArtist(artist spotify.FullArtist) *artistCell {
	cell := &artistCell{}
	cell.init(cell, parent.app)
	cell.dataModelID = amp.CellDataModel_Playlist
	cell.loader = artistLoader
	cell.spotifyID = artist.ID
	cell.AddAttr(amp.Attr_Title, artist.Name)
	cell.AddAttr(amp.Attr_Subtitle, fmt.Sprintf("%d followers", artist.Followers.Count))
	if glyph := chooseBestGlyph(artist.Images, kGlyphPixelSz); glyph != nil {
		cell.AddAttr(amp.Attr_Glyph, glyph)
	}

	parent.childCells = append(parent.childCells, cell)
	return cell
}

func (parent *spotifyCell) newChildAlbum(album spotify.SimpleAlbum) *artistCell {
	cell := &artistCell{}
	cell.init(cell, parent.app)
	cell.dataModelID = amp.CellDataModel_Playlist
	cell.loader = albumLoader
	cell.spotifyID = album.ID
	cell.AddAttr(amp.Attr_Title, album.Name)

	str := strings.Builder{}
	for i, artist := range album.Artists {
		if i > 0 {
			str.WriteString(" \u2022 ")
		}
		str.WriteString(artist.Name)
	}
	cell.AddAttr(amp.Attr_Subtitle, str.String())

	if glyph := chooseBestGlyph(album.Images, kGlyphPixelSz); glyph != nil {
		cell.AddAttr(amp.Attr_Glyph, glyph)
	}

	parent.childCells = append(parent.childCells, cell)
	return cell
}

// GET EXCITED: Capn prot will work GREAT with schemas and storing attrs!

type trackCell struct {
	spotifyCell
}

func (parent *spotifyCell) newChildTrack(track spotify.FullTrack) *trackCell {
	if track.IsPlayable != nil && !*track.IsPlayable {
		return nil
	}
	cell := &trackCell{}
	cell.init(AmpCell(cell), parent.app)
	cell.dataModelID = amp.CellDataModel_Playable
	cell.spotifyID = track.ID
	cell.AddAttr(amp.Attr_Title, track.Name)
	{
		artistDesc := ""
		if len(track.Artists) > 0 {
			artistDesc = track.Artists[0].Name
			for _, artist := range track.Artists[1:] {
				artistDesc += ", " + artist.Name
			}
		}
		if len(artistDesc) > 0 {
			cell.AddAttr(amp.Attr_ArtistDesc, artistDesc)
			cell.AddAttr(amp.Attr_Subtitle, artistDesc)
		}
	}

	if len(track.Album.Name) > 0 {
		cell.AddAttr(amp.Attr_AlbumDesc, track.Album.Name)
	}

	if glyph := chooseBestGlyph(track.Album.Images, kGlyphPixelSz); glyph != nil {
		cell.AddAttr(amp.Attr_Glyph, glyph)
	}

	parent.childCells = append(parent.childCells, cell)
	return cell
}

func (cell *trackCell) pinSelf(req *arc.CellReq) (arc.Cell, error) {
	asset, err := cell.app.respot.PinTrack(string(cell.spotifyID), respot.PinOpts{})
	if err != nil {
		return nil, err
	}

	assetRef := &arc.AssetRef{}
	assetRef.URI, err = cell.app.PublishAsset(asset, arc.PublishOpts{})
	if err != nil {
		return nil, err
	}
	assetRef.MediaType = asset.MediaType()
	cell.SetAttr(amp.Attr_Playable, assetRef)

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
