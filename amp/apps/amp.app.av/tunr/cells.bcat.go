package tunr

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/amp-3d/amp-host-go/amp/apps/amp.app.av/av"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/std"
)

type categories struct {
	std.CellNode[*appInst]
	catalog []*category
}

func (cats *categories) PinInto(pin *std.Pin[*appInst]) error {
	if len(cats.catalog) > 0 {
		return nil
	}

	if err := cats.reload(pin.App); err != nil {
		return err
	}
	for _, cat := range cats.catalog {
		pin.AddChild(cat)
	}

	return nil
}

func (cats *categories) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, "Internet Radio")
}

func (cats *categories) reload(app *appInst) error {
	params := url.Values{}
	params.Add("subtype", "S")

	cats.catalog = nil
	json, err := app.doReq("categories/", params)
	if err != nil {
		return err
	}

	// read '['
	_, err = json.Token()
	if err != nil {
		return err
	}

	// while the array contains values
	for json.More() {
		cat := &category{}
		err := json.Decode(&cat.info)
		if err != nil {
			return err
		}
		if cat.info.Title == "Unlisted" {
			continue
		}

		cats.catalog = append(cats.catalog, cat)
	}

	// read ']'
	_, err = json.Token()
	if err != nil {
		return err
	}

	return nil
}

type category struct {
	std.CellNode[*appInst]
	info     av.CategoryInfo
	stations []*station
}

func (cat *category) PinInto(pin *std.Pin[*appInst]) error {
	if err := cat.reloadAsNeeded(pin.App); err != nil {
		return err
	}
	for _, sta := range cat.stations {
		pin.AddChild(sta)
	}
	return nil

}

func (cat *category) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, cat.info.Title)
	w.PutText(std.CellCaption, cat.info.Description)
	w.PutItem(std.CellGlyphs, &amp.Tag{
		Use:         amp.TagUse_Glyph,
		URL:         "amp:asset/av/" + cat.info.Image,
		ContentType: std.GenericImageType,
	})
}

func (cat *category) reloadAsNeeded(app *appInst) error {
	if len(cat.stations) > 0 {
		return nil
	}

	params := url.Values{}
	params.Add("subtype", "S")
	params.Add("categories", strconv.Itoa(int(cat.info.Id)))

	json, err := app.doReq("bookmarks/", params)
	if err != nil {
		return err
	}

	// read '['
	_, err = json.Token()
	if err != nil {
		return err
	}

	// while the array contains values
	for json.More() {

		sta := &station{}

		err := json.Decode(&sta.info)
		if err != nil {
			return err
		}

		// if created, err := time.Parse(time.RFC3339, cat.info.TimestampCreated); err == nil {
		// 	sta.Tab.SetCreatedAt(created)
		// }
		// if modified, err := time.Parse(time.RFC3339, cat.info.TimestampModified); err == nil {
		// 	sta.Tab.SetModifiedAt(modified)
		// }

		// [<URL>[:::<kbitrate>[:::<MIME type>]];]*
		// Just choose the first URL for now
		url := sta.info.Url
		if N := strings.IndexByte(url, ';'); N > 0 {
			url = url[:N]
		}
		parts := strings.Split(url, ":::")
		if len(parts) > 0 && len(parts[0]) > 0 {
			playable := amp.Tag{
				URL: parts[0],
			}
			if len(parts) >= 3 {
				playable.ContentType = parts[2]
			} else {
				playable.ContentType = "audio/*"
			}

			sta.playable = playable
		}

		cat.stations = append(cat.stations, sta)
	}

	// read ']'
	_, err = json.Token()
	if err != nil {
		return err
	}

	return nil
}

type station struct {
	std.CellNode[*appInst]
	info     av.StationInfo
	playable amp.Tag
}

func (sta *station) PinInto(pin *std.Pin[*appInst]) error {
	return nil
}

func (sta *station) MarshalAttrs(w std.CellWriter) {
	w.PutText(std.CellLabel, sta.info.Title)
	w.PutText(std.CellAuthor, sta.info.Author)
	w.PutText(std.CellCaption, sta.info.Summary)
	w.PutText(std.CellSynopsis, sta.info.Description)
	w.PutItem(std.CellGlyphs, &amp.Tag{
		Use:         amp.TagUse_Glyph,
		URL:         "file://tunr/" + sta.info.Image,
		ContentType: std.GenericImageType,
	})
	w.PutItem(av.MediaTrackInfoID, &av.MediaTrackInfo{
		Flags: av.MediaFlags_HasAudio,
	})
	w.PutItem(std.ContentLink, &sta.playable)
}
