package spotify

import (
	"fmt"
	"strings"

	"github.com/amp-3d/amp-host-go/amp/apps/amp.app.av/av"
	respot "github.com/amp-3d/amp-librespot-go/librespot/api-respot"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/std"
	"github.com/amp-3d/amp-sdk-go/stdlib/media"
	"github.com/zmb3/spotify/v2"
)

const FactoryPath = "file://icons/ui/providers/"

type spotifyCell struct {
	std.CellNode[*appInst]

	// TODO: get rid of zmb crap and use our own
	// stupid to have types when we could just read json fields and switch off that
	// endpoint  string   just use enpoint instead of reconstructing RPC URL!
	// spotifyID spotify.ID
}

type spotifyHome struct {
	std.CellNode[*appInst]
}

func (cell *spotifyHome) PinInto(pin *std.Pin[*appInst]) error {

	{
		child := add_customCell(pin, "Liked Songs", FactoryPath+"playlists.png")
		child.pinner = func(pin *std.Pin[*appInst]) error {
			resp, err := pin.App.client.CurrentUsersTracks(pin.App)
			if err != nil {
				return err
			}
			for i := range resp.Tracks {
				add_trackCell(pin, i, resp.Tracks[i].FullTrack)
			}
			return nil
		}
	}

	{
		child := add_customCell(pin, "Followed Playlists", FactoryPath+"playlists.png")
		child.pinner = func(pin *std.Pin[*appInst]) error {
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
		child.pinner = func(pin *std.Pin[*appInst]) error {
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
		child.pinner = func(pin *std.Pin[*appInst]) error {
			resp, err := pin.App.client.CurrentUsersTopTracks(pin.App)
			if err != nil {
				return err
			}
			for i := range resp.Tracks {
				add_trackCell(pin, i, resp.Tracks[i])
			}
			return nil
		}
	}

	{
		child := add_customCell(pin, "Recently Played Artists", FactoryPath+"artists.png")
		child.pinner = func(pin *std.Pin[*appInst]) error {
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
		child.pinner = func(pin *std.Pin[*appInst]) error {
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
	return nil
}

func (cell *spotifyHome) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, "Spotify")
}

type customCell struct {
	std.CellNode[*appInst]
	label    string
	caption  string
	imageURL string

	pinner func(pin *std.Pin[*appInst]) error
}

func add_customCell(pin *std.Pin[*appInst], title string, imageURL string) *customCell {
	cell := &customCell{
		label:    title,
		imageURL: imageURL,
	}

	pin.AddChild(cell)
	return cell
}

func (cell *customCell) PinInto(pin *std.Pin[*appInst]) error {
	return cell.pinner(pin)
}

func (cell *customCell) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, cell.label)
	if cell.caption != "" {
		w.PutText(std.CellCaption, cell.caption)
	}
	w.PutItem(std.CellGlyphs, &amp.Tag{
		Use:         amp.TagUse_Glyph,
		URL:         cell.imageURL,
		ContentType: std.GenericImageType,
	})

}

type playlistCell struct {
	spotifyCell
	playlist spotify.SimplePlaylist
}

var allAlbumTypes = []spotify.AlbumType{
	spotify.AlbumTypeAlbum,
	spotify.AlbumTypeSingle,
	spotify.AlbumTypeAppearsOn,
	spotify.AlbumTypeCompilation,
}

func add_playlistCell(pin *std.Pin[*appInst], playlist spotify.SimplePlaylist) {
	cell := &playlistCell{
		playlist: playlist,
	}

	pin.AddChild(cell)
}

func (cell *playlistCell) PinInto(pin *std.Pin[*appInst]) error {
	app := pin.App
	resp, err := app.client.GetPlaylistItems(app, cell.playlist.ID)
	if err != nil {
		return err
	}
	for i, item := range resp.Items {
		if item.Track.Track != nil {
			add_trackCell(pin, i, *item.Track.Track)
		} else if item.Track.Episode != nil {
			// TODO: handle episodes
		}
	}
	return nil
}

func (cell *playlistCell) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, cell.playlist.Name)
	w.PutText(std.CellCaption, cell.playlist.Description)

	putGlyphs(w, cell.playlist.Images)
	putLinks(w, cell.playlist.ExternalURLs)
}

type artistCell struct {
	spotifyCell
	artist spotify.FullArtist
}

func add_artistCell(pin *std.Pin[*appInst], artist spotify.FullArtist) {
	cell := &artistCell{
		artist: artist,
	}

	pin.AddChild(cell)
}

func (cell *artistCell) PinInto(pin *std.Pin[*appInst]) error {
	resp, err := pin.App.client.GetArtistAlbums(pin.App, cell.artist.ID, allAlbumTypes)
	if err != nil {
		return err
	}
	for i := range resp.Albums {
		add_albumCell(pin, resp.Albums[i])
	}
	return nil
}

func (cell *artistCell) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, cell.artist.Name)
	w.PutText(std.CellCaption, fmt.Sprintf("%d followers", cell.artist.Followers.Count))

	putGlyphs(w, cell.artist.Images)
	putLinks(w, cell.artist.ExternalURLs)
}

func add_albumCell(pin *std.Pin[*appInst], album spotify.SimpleAlbum) {
	cell := &albumCell{
		album: album,
	}
	pin.AddChild(cell)
}

func (cell *albumCell) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, cell.album.Name)
	w.PutText(std.CellCaption, formArtistDesc(cell.album.Artists))

	timeTag := std.TimeTag{}
	timeTag.SetFromTime(cell.album.ReleaseDateTime())
	w.PutItem(std.CellTimeCreated, &timeTag)

	putGlyphs(w, cell.album.Images)
	putLinks(w, cell.album.ExternalURLs)
}

type albumCell struct {
	spotifyCell
	album spotify.SimpleAlbum
}

func (cell *albumCell) PinInto(pin *std.Pin[*appInst]) error {
	resp, err := pin.App.client.GetAlbum(pin.App, cell.album.ID)
	if err != nil {
		return err
	}
	for i, track := range resp.Tracks.Tracks {
		add_trackCell(pin, i, spotify.FullTrack{
			SimpleTrack: track,
			Album:       resp.SimpleAlbum,
		})
	}
	return nil
}

type trackCell struct {
	spotifyCell
	trackInfo av.MediaTrackInfo
	track     spotify.FullTrack
	assetLink *amp.Tag
}

func add_trackCell(pin *std.Pin[*appInst], ordering int, track spotify.FullTrack) {
	if track.IsPlayable != nil && !*track.IsPlayable {
		return
	}

	cell := &trackCell{
		track: track,
	}

	cell.trackInfo = av.MediaTrackInfo{
		Flags:       av.MediaFlags_HasAudio | av.MediaFlags_IsSeekable | av.MediaFlags_NeedsNetwork,
		Ordering:    float32(ordering + 1),
		Seconds:     .001 * float64(track.Duration),
		Popularity:  .01 * float32(track.Popularity), // 0..100 => 0..1
		ReleaseDate: track.Album.ReleaseDateTime().Unix(),
	}

	pin.AddChild(cell)
}

func (cell *trackCell) PinInto(pin *std.Pin[*appInst]) error {
	app := pin.App
	asset, err := app.respot.PinTrack(string(cell.track.ID), respot.PinOpts{})
	if err != nil {
		return err
	}
	contentURL, err := app.PublishAsset(asset, media.PublishOpts{
		HostAddr: app.Session().LoginInfo().HostAddr,
		OnExpired: func() {
			cell.assetLink = nil
		},
	})
	if err != nil {
		return err
	}

	cell.assetLink = &amp.Tag{
		Use:         amp.TagUse_Content,
		URL:         contentURL,
		ContentType: asset.ContentType(),
	}
	return nil
}

func (cell *trackCell) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, cell.track.Name)
	w.PutText(std.CellCaption, formArtistDesc(cell.track.Artists))
	w.PutText(std.CellCollection, cell.track.Album.Name)
	w.PutItem(av.MediaTrackInfoID, &cell.trackInfo)

	putGlyphs(w, cell.track.Album.Images)
	putLinks(w, cell.track.ExternalURLs)

	if cell.assetLink != nil {
		w.PutItem(std.CellContentLink, cell.assetLink)
	}
}

/**********************************************************
 *  Helpers
 */

func putGlyphs(w std.CellWriter, images []spotify.Image) {
	var tags *amp.Tag

	for _, img := range images {
		tag := &amp.Tag{
			Use:         amp.TagUse_Glyph,
			URL:         img.URL,
			ContentType: std.GenericImageType,
			Metric:      amp.Metric_OrthoPixel,
			SizeX:       uint64(img.Width),
			SizeY:       uint64(img.Height),
		}
		if tags == nil {
			tags = tag
		} else {
			tags.Tags = append(tags.Tags, tag)
		}
	}

	if tags != nil {
		w.PutItem(std.CellGlyphs, tags)
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

func putLinks(w std.CellWriter, urls map[string]string) {
	var tags *amp.Tag

	for link_key, link_url := range urls {
		tag := &amp.Tag{
			Use:         amp.TagUse_Link,
			URL:         link_url,
			ContentType: link_key,
			Text:        link_key,
		}
		if tags == nil {
			tags = tag
		} else {
			tags.Tags = append(tags.Tags, tag)
		}
	}

	if tags != nil {
		w.PutItem(std.CellExternalLinks, tags)
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
