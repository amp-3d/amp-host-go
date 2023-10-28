package spotify

import (
	"fmt"
	"strings"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/apps/amp"
	respot "github.com/arcspace/go-librespot/librespot/api-respot"
	"github.com/zmb3/spotify/v2"
)

type Pinner func(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error

type spotifyCell struct {
	amp.CellBase[*appCtx]

	spotifyID spotify.ID
	pinner    Pinner
	hdr       arc.CellHeader
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

func (cell *spotifyCell) GetLogLabel() string {
	return cell.hdr.Title
}

func (cell *spotifyCell) PinInto(dst *amp.PinnedCell[*appCtx]) error {
	return cell.pinner(dst, cell)
}

func (cell *spotifyCell) MarshalAttrs(dst *arc.CellTx, ctx arc.PinContext) error {
	dst.Marshal(ctx.GetAttrID(arc.CellHeaderAttrSpec), 0, &cell.hdr)
	return nil
}

func (cell *playlistCell) MarshalAttrs(dst *arc.CellTx, ctx arc.PinContext) error {
	cell.spotifyCell.MarshalAttrs(dst, ctx)
	dst.Marshal(ctx.GetAttrID(amp.MediaPlaylistAttrSpec), 0, &cell.MediaPlaylist)
	return nil
}

func (cell *trackCell) MarshalAttrs(dst *arc.CellTx, ctx arc.PinContext) error {
	cell.spotifyCell.MarshalAttrs(dst, ctx)
	dst.Marshal(ctx.GetAttrID(amp.MediaInfoAttrSpec), 0, &cell.MediaInfo)
	if cell.playable != nil {
		dst.Marshal(ctx.GetAttrID(amp.PlayableAssetAttrSpec), 0, cell.playable)
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
		Scheme:    arc.AssetScheme_HttpURL,
		URI:       url,
	}
	return nil
}

func pin_appHome(dst *amp.PinnedCell[*appCtx], cell *spotifyCell) error {

	{
		child := addChild_dir(dst, "Followed Playlists", "/icons/ui/providers/playlists.png")
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
		child := addChild_dir(dst, "Followed Artists", "/icons/ui/providers/artists.png")
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
		child := addChild_dir(dst, "Recently Played", "/icons/ui/providers/tracks.png")
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
		child := addChild_dir(dst, "Recently Played Artists", "/icons/ui/providers/artists.png")
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
		child := addChild_dir(dst, "Saved Albums", "/icons/ui/providers/albums.png")
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

func addChild_dir(dst *amp.PinnedCell[*appCtx], title string, iconURI string) *spotifyCell {
	cell := &spotifyCell{}
	cell.hdr = arc.CellHeader{
		Title: title,
		Glyphs: []*arc.AssetRef{
			{
				Scheme: arc.AssetScheme_FilePath,
				Tags:   arc.AssetTags_IsImageMedia,
				URI:    iconURI,
			},
		},
	}
	cell.AddTo(dst, cell)
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
	cell.hdr = arc.CellHeader{
		Title:        playlist.Name,
		Subtitle:     playlist.Description,
		ExternalLink: chooseBestLink(playlist.ExternalURLs),
	}
	addGlyphs(&cell.hdr, playlist.Images, addAll)

	cell.MediaPlaylist = amp.MediaPlaylist{
		TotalItems: int32(playlist.Tracks.Total),
	}

	cell.AddTo(dst, cell)
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
	cell.hdr = arc.CellHeader{
		Title:        artist.Name,
		Subtitle:     fmt.Sprintf("%d followers", artist.Followers.Count),
		ExternalLink: chooseBestLink(artist.ExternalURLs),
	}
	addGlyphs(&cell.hdr, artist.Images, addAll)

	cell.AddTo(dst, cell)
}

func addChild_Album(dst *amp.PinnedCell[*appCtx], album spotify.SimpleAlbum) {
	cell := &albumCell{}
	cell.spotifyID = album.ID
	cell.hdr = arc.CellHeader{
		Title:        album.Name,
		Subtitle:     formArtistDesc(album.Artists),
		ExternalLink: chooseBestLink(album.ExternalURLs),
		Created:      album.ReleaseDateTime().Unix() << 16,
	}
	addGlyphs(&cell.hdr, album.Images, addAll)

	cell.AddTo(dst, cell)
}

func addChild_Track(dst *amp.PinnedCell[*appCtx], track spotify.FullTrack) {
	if track.IsPlayable != nil && !*track.IsPlayable {
		return
	}
	artistDesc := formArtistDesc(track.Artists)
	releaseDate := track.Album.ReleaseDateTime().Unix()

	cell := &trackCell{}
	cell.spotifyID = track.ID
	cell.hdr = arc.CellHeader{
		Title:        track.Name,
		Subtitle:     artistDesc,
		About:        track.Album.Name,
		ExternalLink: chooseBestLink(track.ExternalURLs),
		Created:      releaseDate << 16,
	}
	addGlyphs(&cell.hdr, track.Album.Images, addAll)

	cell.MediaInfo = amp.MediaInfo{
		Flags:       amp.HasAudio | amp.IsSeekable | amp.NeedsNetwork,
		Title:       track.Name,
		AuthorDesc:  artistDesc,
		Collection:  track.Album.Name,
		ItemNumber:  int32(track.TrackNumber),
		Duration16:  int64(arc.ConvertMsToUTC(int64(track.Duration))),
		Popularity:  .01 * float32(track.Popularity), // 0..100 => 0..1
		ReleaseDate: releaseDate,
		CoverArt:    chooseBestImageURL(cell.hdr.Glyphs, 800),
	}
	cell.AddTo(dst, cell)
}

/**********************************************************
 *  Helpers
 */

type imageSelector int

const (
	bestThumbnail imageSelector = iota
	bestCoverArt
	addAll
)

func addGlyphs(dst *arc.CellHeader, images []spotify.Image, selector imageSelector) {

	switch selector {
	case addAll:
		for _, img := range images {
			addImage(dst, img)
		}
		/*
			case bestCoverArt:
				for _, img := range images {
					szTag := sizeTagForImage(img)

					for _, sizePass := range []arc.AssetTag{
						arc.AssetTag_Res720,
						arc.AssetTag_Res1080,
					} {
						if szTag == sizePass {
							addImage(dst, img, szTag)
							return
						}
					}
				}
			case bestThumbnail:
				for _, img := range images {
					szTag := sizeTagForImage(img)

					for _, sizePass := range []arc.AssetTag{
						arc.AssetTag_Res240,
						arc.AssetTag_Res720,
						arc.AssetTag_Res1080,
					} {
						if szTag == sizePass {
							addImage(dst, img, szTag)
							return
						}
					}
				}
		*/
	}

}

/*
func sizeTagForImage(img spotify.Image) (szTag arc.AssetTags) {
	switch {
	case img.Height > 0 && img.Height < 400:
		szTag = arc.AssetTags_Res240
	case img.Height < 800:
		szTag = arc.AssetTags_Res720
	default:
		szTag = arc.AssetTags_Res1080
	}
	return
}
*/

func addImage(dst *arc.CellHeader, img spotify.Image) {
	dst.Glyphs = append(dst.Glyphs, &arc.AssetRef{
		Tags:        arc.AssetTags_IsImageMedia,
		Scheme:      arc.AssetScheme_HttpURL,
		URI:         img.URL,
		PixelHeight: int32(img.Height),
		PixelWidth:  int32(img.Width),
	})
}

func chooseBestImageURL(assets []*arc.AssetRef, closestHeight int32) string {
	img := chooseBestImage(assets, closestHeight)
	if img == nil {
		return ""
	}
	return img.URI
}

func chooseBestImage(assets []*arc.AssetRef, closestHeight int32) *arc.AssetRef {
	var best *arc.AssetRef
	bestDiff := int32(0x7fffffff)

	for _, img := range assets {
		if img.Tags&arc.AssetTags_IsImageMedia == 0 {
			continue
		}

		// If the image is smaller than what we're looking for, make differences matter more
		diff := img.PixelHeight - closestHeight
		if diff < 0 {
			diff *= -2
		}

		if diff < bestDiff {
			best = img
			bestDiff = diff
		}
	}
	return best
}

func chooseBestLink(links map[string]string) *arc.AssetRef {
	if url, ok := links["spotify"]; ok {
		return &arc.AssetRef{
			Scheme: arc.AssetScheme_HttpURL,
			URI:    url,
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
