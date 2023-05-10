package amp_spotify

import (
	"context"
	"encoding/json"

	"github.com/arcspace/go-arcspace/arc"
	"github.com/arcspace/go-arcspace/arc/apps/amp/api"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"

	respot "github.com/arcspace/go-librespot/librespot/api-respot"
	_ "github.com/arcspace/go-librespot/librespot/core" // bootstrap
)

const kRedirectURL = "http://localhost:5000/callback"
const kSpotifyClientID = "8de730d205474e1490e696adfc10d61c"
const kSpotifyClientSecret = "f7e632155cf445248a2e16e068a78d97"

var (
	gAuth = spotifyauth.New(
		spotifyauth.WithClientID(kSpotifyClientID),
		spotifyauth.WithClientSecret(kSpotifyClientSecret),
		spotifyauth.WithRedirectURL(kRedirectURL),
		spotifyauth.WithScopes(
			spotifyauth.ScopeUserReadPrivate,
			spotifyauth.ScopeUserReadCurrentlyPlaying,
			spotifyauth.ScopeUserReadPlaybackState,
			spotifyauth.ScopeUserModifyPlaybackState,
			spotifyauth.ScopeStreaming,
		))
)

type spotifyApp struct {
	client *spotify.Client // nil if not signed in
	token  oauth2.Token
	user   arc.User
	me     *spotify.PrivateUser
	root   AmpCell
	sess   respot.Session
}

func (app *spotifyApp) AppURI() string {
	return AppURI
}

func (app *spotifyApp) SupportedDataModels() []string {
	return api.SupportedDataModels
}

func (app *spotifyApp) PinCell(req *arc.CellReq) error {

	if req.CellID == 0 {
		err := app.signIn(req.User)
		if err != nil {
			return err
		}
		if app.root == nil {
			app.root = newRootCell(app)
			//app.root.loadChildren =
		}
		req.Cell = app.root

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

func (app *spotifyApp) Context() context.Context {
	return app.user.Session()
}

func (app *spotifyApp) signIn(user arc.User) error {
	if app.client != nil {
		return nil
	}

	app.root = nil
	app.user = user
	app.token = oauth2.Token{}
	app.me = nil

	//
	const kTokenOfDrew = `{"access_token":"BQCh31qtQvn9wp6Ctf3AdXwho0t_5YgNuYy4A4Ezdfb8Z8Khoeg3ZjRzua-csI1C0UBABkKyEAsgzTyeey8v7XKjtQEYnV4TYfr0E6F85VWTXVhezaDBSwH655TQqkGIrpUudLIpfayV3CTnENitC-FRSO_mFmXtEsYp-xh6AVawOcO3rNBqLDfKYUpmy-Dr_szfHokuLA","token_type":"Bearer","refresh_token":"AQBm8dQz0202tHKl0ss9p83VOt6pTjvckICFgQfHKK8UpRkWwu0jXNlbpp4HK2kXXK_ogA0vAIpDuv5ZFJLaQgL9baPx8KSIMvI4tL7L8nnS52BvclG3NpF3rNOYWg4ZthA","expiry":"2023-05-06T20:59:51.500682-05:00"}`

	err := json.Unmarshal([]byte(kTokenOfDrew), &app.token)
	if err != nil {
		panic(err)
	}
	
	if app.sess == nil {
		if app.sess == nil {
			info := user.LoginInfo()
			ctx := respot.DefaultSessionCtx(info.DeviceLabel)
			ctx.Context = user.Session()
			app.sess, err = respot.StartNewSession(ctx)
			if err != nil {
				return arc.ErrCode_ProviderErr.Errorf("StartSession error: %v", err)
			}
			
			ctx.Login.Username = "1228340827"
			ctx.Login.Password = "ellipse007"
			err = app.sess.Login()
		}

		// err = app.sess.LoginOAuthToken(app.token.AccessToken)
		// if err != nil {
		// 	app.token.AccessToken, err = respot.LoginOAuth(kSpotifyClientID, kSpotifyClientSecret, kRedirectURL)
		// 	if err == nil {
		// 		err = app.sess.LoginOAuthToken(app.token.AccessToken)
		// 	}
		// }
	}
	
	if err != nil {
		return err
	}

	// use the token to get an authenticated client
	app.client = spotify.New(gAuth.Client(user.Session(), &app.token))

	app.me, err = app.client.CurrentUser(app.Context())
	if err != nil {
		app.client = nil
		return arc.ErrCode_ProviderErr.Error("failed to get current user")
	}

	return nil
}

func (app *spotifyApp) signOut(user arc.User) {
	if app.client != nil {
		app.client = nil
	}
	app.me = nil
}

type ampAttr struct {
	attrID string
	val    interface{}
	valSI  string
}

type ampCell struct {
	arc.CellID
	app      *spotifyApp
	loadedAt arc.TimeFS
	attrs    []ampAttr
	self     AmpCell

	childCells []AmpCell
	//childByID     map[arc.CellID]arc.AppCell
}

func (cell *ampCell) init(self AmpCell, app *spotifyApp) {
	cell.app = app
	cell.self = self
	cell.CellID = app.user.Session().IssueCellID()
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
				req.User.Session().Warnf("pushCellState: %v", err)
			}
		}
	}

	return nil
}

// func (cell *ampCell) Context() context.Context {
// 	return cell.app.Context()
// }

func (cell *ampCell) PinCell(req *arc.CellReq) error {
	if req.CellID == cell.CellID {
		return cell.self.pinSelf(req)
	}

	// TODO: build a map on demand when needed
	for _, child := range cell.childCells {
		if child.ID() == req.CellID {
			return child.pinSelf(req)
		}
	}
	return arc.ErrCode_InvalidCell.Error("child cell not found")
}

func (cell *ampCell) pinSelf(req *arc.CellReq) error {
	req.Cell = cell.self
	return nil
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

	pinSelf(req *arc.CellReq) error
	loadChildren(req *arc.CellReq) error
}

type rootCell struct {
	ampCell
}

func newRootCell(app *spotifyApp) *rootCell {
	cell := &rootCell{}
	cell.init(cell, app)
	return cell
}

func (cell *rootCell) CellDataModel() string {
	return api.CellDataModel_Dir
}

func (cell *rootCell) loadChildren(req *arc.CellReq) error {
	app := cell.app
	lists, err := app.client.CurrentUsersPlaylists(app.Context())
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

func newPlaylistCell(app *spotifyApp, playlist spotify.SimplePlaylist) *playlistCell {
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
	itemPage, err := app.client.GetPlaylistItems(app.Context(), cell.playlist.ID)
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
	track *spotify.FullTrack
}

func newTrackCell(app *spotifyApp, track *spotify.FullTrack) *trackCell {
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

	cell.AddAttr(api.Attr_Playable, &arc.AssetRef{
		MediaType: "audio/x-spotify",
		URI:       string(track.URI),
	})

	return cell
}

func (cell *trackCell) CellDataModel() string {
	return api.CellDataModel_Playable
}

func (cell *trackCell) pinSelf(req *arc.CellReq) error {
	asset, err := cell.app.sess.PinTrack(string(cell.track.URI), respot.PinOpts{})
	if err != nil {
		return err
	}
	
	assetRef := &arc.AssetRef{}
	assetRef.URI, err = req.User.Session().AssetServer().PublishAsset(asset)
	if err != nil {
		return err
	}
	assetRef.MediaType = asset.MediaType()
	cell.SetAttr(api.Attr_Playable, assetRef)

	req.Cell = cell.self 
	return nil
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
