package spotify

import (
	"fmt"
	"strings"

	"github.com/amp-space/amp-host-go/apps/av"
	respot "github.com/amp-space/amp-librespot-go/librespot/api-respot"
	"github.com/amp-space/amp-sdk-go/amp"
	"github.com/zmb3/spotify/v2"
)

type Pinner func(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error

type spotifyCell struct {
	av.CellBase[*appCtx]

	spotifyID spotify.ID
	pinner    Pinner
	hdr       amp.CellHeader
}

type playlistCell struct {
	spotifyCell
	av.MediaPlaylist
}

type artistCell struct {
	spotifyCell
}

type albumCell struct {
	spotifyCell
}

type trackCell struct {
	spotifyCell
	av.PlayableMediaItem
	assets av.PlayableMediaAssets
}

func (cell *spotifyCell) GetLogLabel() string {
	return cell.hdr.Title
}

func (cell *spotifyCell) PinInto(dst *av.PinnedCell[*appCtx]) error {
	return cell.pinner(dst, cell)
}

func (cell *spotifyCell) MarshalAttrs(dst *amp.CellTx, ctx amp.PinContext) error {
	dst.Marshal(ctx.GetAttrID(amp.CellHeaderAttrSpec), 0, &cell.hdr)
	return nil
}

func (cell *playlistCell) MarshalAttrs(dst *amp.CellTx, ctx amp.PinContext) error {
	cell.spotifyCell.MarshalAttrs(dst, ctx)
	dst.Marshal(ctx.GetAttrID(av.MediaPlaylistAttrSpec), 0, &cell.MediaPlaylist)
	return nil
}

func (cell *trackCell) MarshalAttrs(dst *amp.CellTx, ctx amp.PinContext) error {
	cell.spotifyCell.MarshalAttrs(dst, ctx)
	dst.Marshal(ctx.GetAttrID(av.PlayableMediaItemAttrSpec), 0, &cell.PlayableMediaItem)
	if cell.assets.MainTrack != nil {
		dst.Marshal(ctx.GetAttrID(av.PlayableMediaAssetsAttrSpec), 0, &cell.assets)
	}
	return nil
}

func (cell *trackCell) PinInto(dst *av.PinnedCell[*appCtx]) error {
	app := dst.App
	asset, err := app.respot.PinTrack(string(cell.spotifyID), respot.PinOpts{})
	if err != nil {
		return err
	}
	url, err := app.PublishAsset(asset, amp.PublishOpts{
		HostAddr: app.Session().LoginInfo().HostAddr,
	})
	if err != nil {
		return err
	}

	cell.assets.MainTrack = &amp.AssetTag{
		ContentType: asset.MediaType(),
		URI:         url,
	}
	return nil
}

const FactoryPath = "file://icons/ui/providers/"

func pin_appHome(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error {

	{
		child := addChild_dir(dst, "Followed Playlists", FactoryPath+"playlists.png")
		child.pinner = func(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error {
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
		child := addChild_dir(dst, "Followed Artists", FactoryPath+"artists.png")
		child.pinner = func(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error {
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
		child := addChild_dir(dst, "Recently Played", FactoryPath+"tracks.png")
		child.pinner = func(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error {
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
		child := addChild_dir(dst, "Recently Played Artists", FactoryPath+"artists.png")
		child.pinner = func(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error {
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
		child := addChild_dir(dst, "Saved Albums", FactoryPath+"albums.png")
		child.pinner = func(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error {
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

func addChild_dir(dst *av.PinnedCell[*appCtx], title string, imgURI string) *spotifyCell {
	cell := &spotifyCell{}
	cell.hdr = amp.CellHeader{
		Title: title,
		Glyphs: []*amp.AssetTag{
			{
				ContentType: amp.GenericImageType,
				URI:         imgURI,
			},
		},
	}
	cell.AddTo(dst, cell)
	return cell
}

// func pin_Track(dst *av.PinnedCell[*appCtx], cell *spotifyCell) error {
// 	app := dst.App
// 	asset, err := app.respot.PinTrack(string(cell.spotifyID), respot.PinOpts{})
// 	if err != nil {
// 		return err
// 	}
// 	assetRef := &amp.AssetRef{
// 		MediaType: asset.MediaType(),
// 	}
// 	assetRef.URI, err = app.PublishAsset(asset, amp.PublishOpts{
// 		HostAddr: app.Session().LoginInfo().HostAddr,
// 	})
// 	if err != nil {
// 		return err
// 	}
// 	cell.(*trackCell).PlayableMedia.URI = assetRef.URI
// 	cell.SetAttr(dst.App, av.Attr_Playable, assetRef)
// 	return nil
// }

var allAlbumTypes = []spotify.AlbumType{
	spotify.AlbumTypeAlbum,
	spotify.AlbumTypeSingle,
	spotify.AlbumTypeAppearsOn,
	spotify.AlbumTypeCompilation,
}

func addChild_Playlist(dst *av.PinnedCell[*appCtx], playlist spotify.SimplePlaylist) {
	cell := &playlistCell{}
	cell.spotifyID = playlist.ID
	cell.hdr = amp.CellHeader{
		Title:        playlist.Name,
		Subtitle:     playlist.Description,
		ExternalLink: chooseBestLink(playlist.ExternalURLs),
	}
	addGlyphs(&cell.hdr, playlist.Images, addAll)

	cell.MediaPlaylist = av.MediaPlaylist{
		TotalItems: int32(playlist.Tracks.Total),
	}

	cell.AddTo(dst, cell)
}

func (cell *playlistCell) PinInto(dst *av.PinnedCell[*appCtx]) error {
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

func (cell *artistCell) PinInto(dst *av.PinnedCell[*appCtx]) error {
	resp, err := dst.App.client.GetArtistAlbums(dst.App, cell.spotifyID, allAlbumTypes)
	if err != nil {
		return err
	}
	for i := range resp.Albums {
		addChild_Album(dst, resp.Albums[i])
	}
	return nil
}

func (cell *albumCell) PinInto(dst *av.PinnedCell[*appCtx]) error {
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

func addChild_Artist(dst *av.PinnedCell[*appCtx], artist spotify.FullArtist) {
	cell := &artistCell{}
	cell.spotifyID = artist.ID
	cell.hdr = amp.CellHeader{
		Title:        artist.Name,
		Subtitle:     fmt.Sprintf("%d followers", artist.Followers.Count),
		ExternalLink: chooseBestLink(artist.ExternalURLs),
	}
	addGlyphs(&cell.hdr, artist.Images, addAll)

	cell.AddTo(dst, cell)
}

func addChild_Album(dst *av.PinnedCell[*appCtx], album spotify.SimpleAlbum) {
	cell := &albumCell{}
	cell.spotifyID = album.ID
	cell.hdr = amp.CellHeader{
		Title:        album.Name,
		Subtitle:     formArtistDesc(album.Artists),
		ExternalLink: chooseBestLink(album.ExternalURLs),
		Created:      album.ReleaseDateTime().Unix() << 16,
	}
	addGlyphs(&cell.hdr, album.Images, addAll)

	cell.AddTo(dst, cell)
}

func addChild_Track(dst *av.PinnedCell[*appCtx], track spotify.FullTrack) {
	if track.IsPlayable != nil && !*track.IsPlayable {
		return
	}
	artistDesc := formArtistDesc(track.Artists)
	releaseDate := track.Album.ReleaseDateTime().Unix()

	cell := &trackCell{}
	cell.spotifyID = track.ID
	cell.hdr = amp.CellHeader{
		Title:        track.Name,
		Subtitle:     artistDesc,
		About:        track.Album.Name,
		ExternalLink: chooseBestLink(track.ExternalURLs),
		Created:      releaseDate << 16,
	}
	addGlyphs(&cell.hdr, track.Album.Images, addAll)

	cell.PlayableMediaItem = av.PlayableMediaItem{
		Flags:       av.HasAudio | av.IsSeekable | av.NeedsNetwork,
		Title:       track.Name,
		AuthorDesc:  artistDesc,
		Collection:  track.Album.Name,
		ItemNumber:  int32(track.TrackNumber),
		Duration16:  int64(amp.ConvertMsToUTC(int64(track.Duration))),
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

func addGlyphs(dst *amp.CellHeader, images []spotify.Image, selector imageSelector) {

	switch selector {
	case addAll:
		for _, img := range images {
			addImage(dst, img)
		}
		/*
			case bestCoverArt:
				for _, img := range images {
					szTag := sizeTagForImage(img)

					for _, sizePass := range []amp.AssetTag{
						amp.AssetTag_Res720,
						amp.AssetTag_Res1080,
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

					for _, sizePass := range []amp.AssetTag{
						amp.AssetTag_Res240,
						amp.AssetTag_Res720,
						amp.AssetTag_Res1080,
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
func sizeTagForImage(img spotify.Image) (szTag amp.AssetTags) {
	switch {
	case img.Height > 0 && img.Height < 400:
		szTag = amp.AssetTags_Res240
	case img.Height < 800:
		szTag = amp.AssetTags_Res720
	default:
		szTag = amp.AssetTags_Res1080
	}
	return
}
*/

func addImage(dst *amp.CellHeader, img spotify.Image) {
	dst.Glyphs = append(dst.Glyphs, &amp.AssetTag{
		ContentType: amp.GenericImageType,
		URI:         img.URL,
		PixelHeight: int32(img.Height),
		PixelWidth:  int32(img.Width),
	})
}

func chooseBestImageURL(assets []*amp.AssetTag, closestHeight int32) string {
	img := chooseBestImage(assets, closestHeight)
	if img == nil {
		return ""
	}
	return img.URI
}

func chooseBestImage(assets []*amp.AssetTag, closestHeight int32) *amp.AssetTag {
	var best *amp.AssetTag
	bestDiff := int32(0x7fffffff)

	for _, img := range assets {

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

func chooseBestLink(links map[string]string) *amp.AssetTag {
	if url, ok := links["spotify"]; ok {
		return &amp.AssetTag{
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
				str.WriteString(av.ListItemSeparator)
			}
			str.WriteString(artist.Name)
		}
		return str.String()
	}
}
