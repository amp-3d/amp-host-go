package spotify

import (
	"fmt"
	"strings"

	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/av"
	respot "github.com/amp-3d/amp-librespot-go/librespot/api-respot"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/basic"
	"github.com/amp-3d/amp-sdk-go/stdlib/media"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
	"github.com/zmb3/spotify/v2"
)

type spotifyCell struct {
	basic.CellInfo[*appInst]

	endpoint  string
	spotifyID spotify.ID
}

type spotifyHome struct {
	basic.CellInfo[*appInst]
}

type customCell struct {
	basic.CellInfo[*appInst]

	pinner func(pin *basic.Pinned[*appInst]) error
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
	av.PlayableMedia
}

func (cell *customCell) PinInto(pin *basic.Pinned[*appInst]) error {
	return cell.pinner(pin)
}

func (cell *playlistCell) MarshalAttrs(pin *basic.Pin[*appInst]) {
	cell.spotifyCell.MarshalAttrs(pin)
	pin.Upsert(cell.ID, av.MediaPlaylistSpec.ID, tag.Nil, &cell.MediaPlaylist)
}

func (cell *trackCell) MarshalAttrs(pin *basic.Pin[*appInst]) {
	cell.spotifyCell.MarshalAttrs(pin)
	pin.Upsert(cell.ID, av.PlayableMediaSpec.ID, tag.Nil, &cell.PlayableMedia)
}

func (cell *trackCell) PinInto(pin *basic.Pinned[*appInst]) error {
	app := pin.App
	asset, err := app.respot.PinTrack(string(cell.spotifyID), respot.PinOpts{})
	if err != nil {
		return err
	}
	contentURL, err := app.PublishAsset(asset, media.PublishOpts{
		HostAddr: app.Session().Auth().HostAddr,
	})
	if err != nil {
		return err
	}
	cell.PlayableMedia.Media.AddTag(&amp.Tag{
		Use:         amp.TagUse_Content,
		URL:         contentURL,
		ContentType: asset.ContentType(),
	})
	return nil
}

const FactoryPath = "file://icons/ui/providers/"

func (cell *spotifyHome) PinInto(pin *basic.Pinned[*appInst]) error {

	{
		child := add_customCell(pin, "Followed Playlists", FactoryPath+"playlists.png")
		child.pinner = func(pin *basic.Pinned[*appInst]) error {
			resp, err := pin.App.client.CurrentUsersPlaylists(pin.App)
			if err != nil {
				return err
			}
			for i := range resp.Playlists {
				add_playlistCell(pin, resp.Playlists[i])
			}
			return nil
		}
	}

	{
		child := add_customCell(pin, "Followed Artists", FactoryPath+"artists.png")
		child.pinner = func(pin *basic.Pinned[*appInst]) error {
			resp, err := pin.App.client.CurrentUsersFollowedArtists(pin.App)
			if err != nil {
				return err
			}
			for i := range resp.Artists {
				add_artistCell(pin, resp.Artists[i])
			}
			return nil
		}
	}

	{
		child := add_customCell(pin, "Recently Played", FactoryPath+"tracks.png")
		child.pinner = func(pin *basic.Pinned[*appInst]) error {
			resp, err := pin.App.client.CurrentUsersTopTracks(pin.App)
			if err != nil {
				return err
			}
			for i := range resp.Tracks {
				add_trackCell(pin, resp.Tracks[i])
			}
			return nil
		}
	}

	{
		child := add_customCell(pin, "Recently Played Artists", FactoryPath+"artists.png")
		child.pinner = func(pin *basic.Pinned[*appInst]) error {
			resp, err := pin.App.client.CurrentUsersTopArtists(pin.App)
			if err != nil {
				return err
			}
			for i := range resp.Artists {
				add_artistCell(pin, resp.Artists[i])
			}
			return nil
		}
	}

	{
		child := add_customCell(pin, "Saved Albums", FactoryPath+"albums.png")
		child.pinner = func(pin *basic.Pinned[*appInst]) error {
			resp, err := pin.App.client.CurrentUsersAlbums(pin.App)
			if err != nil {
				return err
			}
			for i := range resp.Albums {
				add_albumCell(pin, resp.Albums[i].SimpleAlbum)
			}
			return nil
		}

	}

	// CurrentUsersShows

	return nil
}

func add_customCell(pin *basic.Pinned[*appInst], title string, imgURL string) *customCell {
	cell := &customCell{}
	cell.Tab = amp.TagTab{
		Label: title,
		Tags: []*amp.Tag{
			{
				Use:         amp.TagUse_Glyph,
				URL:         imgURL,
				ContentType: amp.GenericImageType,
			},
			amp.PinnableCatalog,
		},
	}
	pin.AddChild(cell)
	return cell
}

var allAlbumTypes = []spotify.AlbumType{
	spotify.AlbumTypeAlbum,
	spotify.AlbumTypeSingle,
	spotify.AlbumTypeAppearsOn,
	spotify.AlbumTypeCompilation,
}

func add_playlistCell(pin *basic.Pinned[*appInst], playlist_zmb spotify.SimplePlaylist) {
	playlist := &playlistCell{}
	playlist.endpoint = playlist_zmb.Endpoint
	playlist.spotifyID = playlist_zmb.ID
	playlist.Tab = amp.TagTab{
		Label:   playlist_zmb.Name,
		Caption: playlist_zmb.Description,
		Tags: []*amp.Tag{
			amp.PinnableCatalog,
		},
	}

	addGlyphs(&playlist.Tab, playlist_zmb.Images, addAll)
	addLinks(&playlist.Tab, playlist_zmb.ExternalURLs)

	playlist.MediaPlaylist = av.MediaPlaylist{
		TotalItems: int32(playlist_zmb.Tracks.Total),
	}

	pin.AddChild(playlist)
}

func (cell *playlistCell) PinInto(pin *basic.Pinned[*appInst]) error {
	app := pin.App
	resp, err := app.client.GetPlaylistItems(app, cell.spotifyID)
	if err != nil {
		return err
	}
	for _, item := range resp.Items {
		if item.Track.Track != nil {
			add_trackCell(pin, *item.Track.Track)
		} else if item.Track.Episode != nil {
			// TODO: handle episodes
		}
	}
	return nil
}

func (cell *artistCell) PinInto(pin *basic.Pinned[*appInst]) error {
	resp, err := pin.App.client.GetArtistAlbums(pin.App, cell.spotifyID, allAlbumTypes)
	if err != nil {
		return err
	}
	for i := range resp.Albums {
		add_albumCell(pin, resp.Albums[i])
	}
	return nil
}

func (cell *albumCell) PinInto(pin *basic.Pinned[*appInst]) error {
	resp, err := pin.App.client.GetAlbum(pin.App, cell.spotifyID)
	if err != nil {
		return err
	}
	for _, track := range resp.Tracks.Tracks {
		add_trackCell(pin, spotify.FullTrack{
			SimpleTrack: track,
			Album:       resp.SimpleAlbum,
		})
	}
	return nil
}

func add_artistCell(pin *basic.Pinned[*appInst], artist spotify.FullArtist) {
	cell := &artistCell{}
	cell.spotifyID = artist.ID
	cell.endpoint = artist.Endpoint
	cell.Tab = amp.TagTab{
		Label:   artist.Name,
		Caption: fmt.Sprintf("%d followers", artist.Followers.Count),
	}

	addGlyphs(&cell.Tab, artist.Images, addAll)
	addLinks(&cell.Tab, artist.ExternalURLs)

	pin.AddChild(cell)
}

func add_albumCell(pin *basic.Pinned[*appInst], album spotify.SimpleAlbum) {
	cell := &albumCell{}
	cell.spotifyID = album.ID
	cell.endpoint = album.Endpoint
	cell.Tab = amp.TagTab{
		Label:   album.Name,
		Caption: formArtistDesc(album.Artists),
	}

	addGlyphs(&cell.Tab, album.Images, addAll)
	addLinks(&cell.Tab, album.ExternalURLs)
	cell.Tab.SetCreatedAt(album.ReleaseDateTime())

	pin.AddChild(cell)
}

func add_trackCell(pin *basic.Pinned[*appInst], track spotify.FullTrack) {
	if track.IsPlayable != nil && !*track.IsPlayable {
		return
	}
	artistDesc := formArtistDesc(track.Artists)

	cell := &trackCell{}
	cell.spotifyID = track.ID
	cell.endpoint = track.Endpoint
	cell.Tab = amp.TagTab{
		Label:   track.Name,
		Caption: artistDesc,
		About:   track.Album.Name,
	}

	addGlyphs(&cell.Tab, track.Album.Images, addAll)
	addLinks(&cell.Tab, track.ExternalURLs)

	cell.PlayableMedia = av.PlayableMedia{
		Flags: av.MediaFlags_HasAudio | av.MediaFlags_IsSeekable | av.MediaFlags_NeedsNetwork,
		Media: &amp.TagTab{
			Label: track.Name,
			Tags:  cell.Tab.Tags,
		},
		Author: &amp.TagTab{
			Label: artistDesc,
		},
		Collection: &amp.TagTab{
			Label: track.Album.Name,
		},
		ItemNumber:  int32(track.TrackNumber),
		Seconds:     .001 * float64(track.Duration),
		Popularity:  .01 * float32(track.Popularity), // 0..100 => 0..1
		ReleaseDate: track.Album.ReleaseDateTime().Unix(),
	}
	pin.AddChild(cell)
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

func addGlyphs(dst *amp.TagTab, images []spotify.Image, selector imageSelector) {

	switch selector {
	case addAll:
		for _, img := range images {
			addImage(dst, img)
		}
		/*
			case bestCoverArt:
				for _, img := range images {
					szTag := sizeTagForImage(img)

					for _, sizePass := range []amp.Tag{
						amp.TagAsset_Res720,
						amp.TagAsset_Res1080,
					} {
						if szTag == sizePass {
							addImage(pin, img, szTag)
							return
						}
					}
				}
			case bestThumbnail:
				for _, img := range images {
					szTag := sizeTagForImage(img)

					for _, sizePass := range []amp.Tag{
						amp.TagAsset_Res240,
						amp.TagAsset_Res720,
						amp.TagAsset_Res1080,
					} {
						if szTag == sizePass {
							addImage(pin, img, szTag)
							return
						}
					}
				}
		*/
	}

}

/*
func sizeTagForImage(img spotify.Image) (szTag amp.TagAssets) {
	switch {
	case img.Height > 0 && img.Height < 400:
		szTag = amp.TagAssets_Res240
	case img.Height < 800:
		szTag = amp.TagAssets_Res720
	default:
		szTag = amp.TagAssets_Res1080
	}
	return
}
*/

func addImage(dst *amp.TagTab, img spotify.Image) {
	dst.Tags = append(dst.Tags, &amp.Tag{
		Use:         amp.TagUse_Glyph,
		ContentType: amp.GenericImageType,
		Metric:      amp.Metric_OrthoPixel,
		URL:         img.URL,
		Size_0:      float32(img.Width),
		Size_1:      float32(img.Height),
	})
}

/*
func chooseBestImageURL(assets []*amp.Tag, closestHeight int32) string {
	img := chooseBestImage(assets, closestHeight)
	if img == nil {
		return ""
	}
	return img.URL
}

func chooseBestImage(assets []*amp.Tag, closestHeight int32) *amp.Tag {
	var best *amp.Tag
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

func chooseBestLink(links map[string]string) *amp.Tag {
	if url, ok := links["spotify"]; ok {
		return &amp.Tag{
			URL: url,
		}
	}
	return nil
}
*/

func addLinks(dst *amp.TagTab, urls map[string]string) {
	for link_key, link_url := range urls {
		dst.Tags = append(dst.Tags, &amp.Tag{
			Use:         amp.TagUse_Link,
			ContentType: link_key,
			URL:         link_url,
		})
	}
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
