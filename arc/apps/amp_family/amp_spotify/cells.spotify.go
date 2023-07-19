package amp_spotify

import (
	"fmt"
	"strings"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
	respot "github.com/arcspace/go-librespot/librespot/api-respot"
	"github.com/zmb3/spotify/v2"
)

type Pinner func(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error

type spotifyCell struct {
	amp.CellBase[*appCtx]
	spotifyID spotify.ID
	pinner    Pinner
	info      arc.CellInfo
}

type playlistCell struct {
	spotifyCell
	amp.MediaPlaylist
}

type artistCell struct {
	spotifyCell
}

type albumCell struct {
	spotifyCell
}

type trackCell struct {
	spotifyCell
	amp.MediaInfo
	playable *arc.AssetRef // non-nil when pinned
}

func (cell *spotifyCell) MarshalAttrs(app *appCtx, dst *arc.CellTx) error {
	dst.Marshal(app.CellInfoAttr, 0, &cell.info)
	return nil
}

func (cell *spotifyCell) Label() string {
	return cell.info.Title
}

func (cell *spotifyCell) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	return cell.pinner(dst, cell)
}

func (cell *playlistCell) MarshalAttrs(app *appCtx, dst *arc.CellTx) error {
	cell.spotifyCell.MarshalAttrs(app, dst)
	dst.Marshal(app.MediaPlaylistAttr, 0, &cell.MediaPlaylist)
	return nil
}

func (cell *trackCell) MarshalAttrs(app *appCtx, dst *arc.CellTx) error {
	cell.spotifyCell.MarshalAttrs(app, dst)
	dst.Marshal(app.MediaInfoAttr, 0, &cell.MediaInfo)
	if cell.playable != nil {
		dst.Marshal(app.PlayableAssetAttr, 0, cell.playable)
	}
	return nil
}

func (cell *trackCell) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	app := dst.App
	asset, err := app.respot.PinTrack(string(cell.spotifyID), respot.PinOpts{})
	if err != nil {
		return err
	}
	url, err := app.PublishAsset(asset, arc.PublishOpts{
		HostAddr: app.Session().LoginInfo().HostAddr,
	})
	if err != nil {
		return err
	}

	cell.playable = &arc.AssetRef{
		MediaType: asset.MediaType(),
		URI:       url,
	}
	return nil
}

func pin_appHome(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {

	{
		child := addChild_dir(dst, "Followed Playlists")
		child.info.Glyph = &arc.AssetRef{
			URI:    "/icons/ui/providers/playlists.png",
			Scheme: arc.URIScheme_File,
		}
		child.pinner = func(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {
			resp, err := dst.App.client.CurrentUsersPlaylists(dst.App)
			if err != nil {
				return err
			}
			for i := range resp.Playlists {
				addChild_Playlist(dst, resp.Playlists[i])
			}
			return nil
		}
	}

	{
		child := addChild_dir(dst, "Followed Artists")
		child.info.Glyph = &arc.AssetRef{
			URI:    "/icons/ui/providers/artists.png",
			Scheme: arc.URIScheme_File,
		}
		child.pinner = func(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {
			resp, err := dst.App.client.CurrentUsersFollowedArtists(dst.App)
			if err != nil {
				return err
			}
			for i := range resp.Artists {
				addChild_Artist(dst, resp.Artists[i])
			}
			return nil
		}
	}

	{
		child := addChild_dir(dst, "Recently Played")
		child.info.Glyph = &arc.AssetRef{
			URI:    "/icons/ui/providers/tracks.png",
			Scheme: arc.URIScheme_File,
		}
		child.pinner = func(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {
			resp, err := dst.App.client.CurrentUsersTopTracks(dst.App)
			if err != nil {
				return err
			}
			for i := range resp.Tracks {
				addChild_Track(dst, resp.Tracks[i])
			}
			return nil
		}
	}

	{
		child := addChild_dir(dst, "Recently Played Artists")
		child.info.Glyph = &arc.AssetRef{
			URI:    "/icons/ui/providers/artists.png",
			Scheme: arc.URIScheme_File,
		}
		child.pinner = func(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {
			resp, err := dst.App.client.CurrentUsersTopArtists(dst.App)
			if err != nil {
				return err
			}
			for i := range resp.Artists {
				addChild_Artist(dst, resp.Artists[i])
			}
			return nil
		}
	}

	{
		child := addChild_dir(dst, "Saved Albums")
		child.info.Glyph = &arc.AssetRef{
			URI:    "/icons/ui/providers/albums.png",
			Scheme: arc.URIScheme_File,
		}
		child.pinner = func(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {
			resp, err := dst.App.client.CurrentUsersAlbums(dst.App)
			if err != nil {
				return err
			}
			for i := range resp.Albums {
				addChild_Album(dst, resp.Albums[i].SimpleAlbum)
			}
			return nil
		}

	}

	// CurrentUsersShows

	return nil
}

func addChild_dir(dst *amp.PinnedCell[*appCtx], title string) *spotifyCell {
	cell := &spotifyCell{}
	cell.AddTo(dst, cell, dst.App.LinkCellSpec)
	cell.info = arc.CellInfo{
		Title: title,
	}
	return cell
}

// func pin_Track(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {
// 	app := dst.App
// 	asset, err := app.respot.PinTrack(string(cell.spotifyID), respot.PinOpts{})
// 	if err != nil {
// 		return err
// 	}
// 	assetRef := &arc.AssetRef{
// 		MediaType: asset.MediaType(),
// 	}
// 	assetRef.URI, err = app.PublishAsset(asset, arc.PublishOpts{
// 		HostAddr: app.Session().LoginInfo().HostAddr,
// 	})
// 	if err != nil {
// 		return err
// 	}
// 	cell.(*trackCell).MediaInfo.URI = assetRef.URI
// 	cell.SetAttr(dst.App, amp.Attr_Playable, assetRef)
// 	return nil
// }

var allAlbumTypes = []spotify.AlbumType{
	spotify.AlbumTypeAlbum,
	spotify.AlbumTypeSingle,
	spotify.AlbumTypeAppearsOn,
	spotify.AlbumTypeCompilation,
}

func addChild_Playlist(dst *amp.PinnedCell[*appCtx], playlist spotify.SimplePlaylist) {
	cell := &playlistCell{}
	cell.spotifyID = playlist.ID
	cell.AddTo(dst, cell, dst.App.PlaylistCellSpec)

	cell.info = arc.CellInfo{
		Title:    playlist.Name,
		Subtitle: playlist.Description,
		Link:     chooseBestLink(playlist.ExternalURLs),
	}
	setGlyphs(playlist.Images, &cell.info)

	cell.MediaPlaylist = amp.MediaPlaylist{
		TotalItems: int32(playlist.Tracks.Total),
	}
}

func (cell *playlistCell) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	app := dst.App
	resp, err := app.client.GetPlaylistItems(app, cell.spotifyID)
	if err != nil {
		return err
	}
	for _, item := range resp.Items {
		if item.Track.Track != nil {
			addChild_Track(dst, *item.Track.Track)
		} else if item.Track.Episode != nil {
			// TODO: handle episodes
		}
	}
	return nil
}

func (cell *artistCell) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	resp, err := dst.App.client.GetArtistAlbums(dst.App, cell.spotifyID, allAlbumTypes)
	if err != nil {
		return err
	}
	for i := range resp.Albums {
		addChild_Album(dst, resp.Albums[i])
	}
	return nil
}

func (cell *albumCell) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	resp, err := dst.App.client.GetAlbum(dst.App, cell.spotifyID)
	if err != nil {
		return err
	}
	for _, track := range resp.Tracks.Tracks {
		addChild_Track(dst, spotify.FullTrack{
			SimpleTrack: track,
			Album:       resp.SimpleAlbum,
		})
	}
	return nil
}

func addChild_Artist(dst *amp.PinnedCell[*appCtx], artist spotify.FullArtist) {
	cell := &artistCell{}
	cell.spotifyID = artist.ID
	cell.AddTo(dst, cell, dst.App.LinkCellSpec)

	cell.info = arc.CellInfo{
		Title:    artist.Name,
		Subtitle: fmt.Sprintf("%d followers", artist.Followers.Count),
		Link:     chooseBestLink(artist.ExternalURLs),
	}
	setGlyphs(artist.Images, &cell.info)
}

func addChild_Album(dst *amp.PinnedCell[*appCtx], album spotify.SimpleAlbum) {
	cell := &albumCell{}
	cell.spotifyID = album.ID
	cell.AddTo(dst, cell, dst.App.LinkCellSpec)

	cell.info = arc.CellInfo{
		Title:    album.Name,
		Subtitle: formArtistDesc(album.Artists),
		Link:     chooseBestLink(album.ExternalURLs),
	}
	setGlyphs(album.Images, &cell.info)
}

func addChild_Track(dst *amp.PinnedCell[*appCtx], track spotify.FullTrack) {
	if track.IsPlayable != nil && !*track.IsPlayable {
		return
	}
	cell := &trackCell{}
	cell.spotifyID = track.ID
	cell.AddTo(dst, cell, dst.App.PlayableCellSpec)

	artistDesc := formArtistDesc(track.Artists)

	cell.info = arc.CellInfo{
		Title:    track.Name,
		Subtitle: artistDesc,
		About:    track.Album.Name,
		Link:     chooseBestLink(track.ExternalURLs),
	}
	setGlyphs(track.Album.Images, &cell.info)

	cell.MediaInfo = amp.MediaInfo{
		Flags:       amp.HasAudio | amp.IsSeekable | amp.NeedsNetwork,
		Title:       track.Name,
		AuthorDesc:  artistDesc,
		Collection:  track.Album.Name,
		ItemNumber:  int32(track.TrackNumber),
		Duration16:  int64(arc.ConvertMsToUTC(int64(track.Duration))),
		CoverArt:    cell.info.Glyph.URI,
		Popularity:  .01 * float32(track.Popularity), // 0..100 => 0..1
		ReleaseTime: track.Album.ReleaseDateTime().Unix(),
	}
}

/**********************************************************
 *  Helpers
 */

func setGlyphs(images []spotify.Image, info *arc.CellInfo) (glyph *arc.AssetRef, cover *arc.AssetRef) {
	info.Glyph = chooseBestImage(images, 200)
	if info.Glyph != nil {
		if len(images) > 1 {
			info.GlyphLarge = chooseBestImage(images, 800)
		}
	}
	return
}

func chooseBestImage(images []spotify.Image, closestSize int) *arc.AssetRef {
	bestImg := -1
	bestDiff := 0x7fffffff

	for i, img := range images {
		diff := img.Width - closestSize

		// If the image is smaller than what we're looking for, make differences matter more
		if diff < 0 {
			diff *= -2
		}

		if diff < bestDiff {
			bestImg = i
			bestDiff = diff
		}
	}
	if bestImg < 0 {
		return nil
	}
	return &arc.AssetRef{
		MediaType: "image/x-spotify",
		URI:       images[bestImg].URL,
		PixWidth:  int32(images[bestImg].Width),
		PixHeight: int32(images[bestImg].Height),
	}
}

func chooseBestLink(links map[string]string) *arc.AssetRef {
	if url, ok := links["spotify"]; ok {
		return &arc.AssetRef{
			URI: url,
		}
	}
	return nil
}

func formArtistDesc(artists []spotify.SimpleArtist) string {
	switch len(artists) {
	case 0:
		return ""
	case 1:
		return artists[0].Name
	default:
		str := strings.Builder{}
		for i, artist := range artists {
			if i > 0 {
				str.WriteString(amp.ListItemSeparator)
			}
			str.WriteString(artist.Name)
		}
		return str.String()
	}
}
