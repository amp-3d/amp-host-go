package bcat

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/av"
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/basic"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
)

type categories struct {
	basic.CellInfo[*appInst]
	catalog []*category
}

func (cats *categories) PinInto(pin *basic.Pinned[*appInst]) error {
	if len(cats.catalog) > 0 {
		return nil
	}

	cats.Tab = amp.TagTab{
		Label: "Internet Radio",
		Tags: []*amp.Tag{
			amp.GenericFolderGlyph,
			amp.PinnableCatalog,
		},
	}
	if err := cats.reload(pin.App); err != nil {
		return err
	}
	for _, cat := range cats.catalog {
		pin.AddChild(cat)
	}

	// pin.Declare(amp.AppChannel[*appInst]{
	// 	Spec: amp.TabCatalogSpec,
	// 	Marshaller: func() {
	// 		pin.Upsert(cats.ID, amp.TagTabSpec.ID, tag.Nil, &cats.Tab)
	// 		for _, cat := range cats.catalog {
	// 			pin.Upsert(cat.ID, amp.TagTabSpec.ID, tag.Nil, &cat.Tab)
	// 		}
	// 	},
	// })

	return nil
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

		cat.Tab = amp.TagTab{
			Label:   cat.info.Title,
			Caption: cat.info.Description,
			Tags: []*amp.Tag{
				{
					Use:         amp.TagUse_Glyph,
					URL:         "amp:asset/av/" + cat.info.Image,
					ContentType: amp.GenericImageType,
				},
				amp.PinnableCatalog,
			},
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

/*

func (cats *categories) MarshalAttr(pin *app.Pin[*appInst]) {
	if op.Contains(amp.TagTabSpec.ID) {

			pin.Upsert(cats.ID, amp.TagTabSpec.ID, tag.Nil, &cats.Tab)
			for _, cat := range cats.catalog {
				pin.Upsert(cat.ID, amp.TagTabSpec.ID, tag.Nil, &cat.Tab)
			}

		hdr := amp.TagTab{}
		hdr = amp.TagTab{
			Label:  "Internet Radio",
			Glyphs: amp.GenericFolderGlyph,
		}

		for _, cat := range cats.catalog {
			tab := amp.TagTab{
				Label:   cat.info.Title,
				Caption: cat.info.Description,
				Glyphs: &amp.Tag{
					URL:         "amp:asset/av/" + cat.info.Image,
					ContentType: amp.GenericImageType,
				},
			}
			pin.Upsert(cat.ID, amp.TagTabSpec.ID, tag.Nil, &tab)
		}
	}

}
*/

type category struct {
	basic.CellInfo[*appInst]
	info     av.CategoryInfo
	stations []*station
}

func (cat *category) PinInto(pin *basic.Pinned[*appInst]) error {
	return cat.reloadAsNeeded(pin.App)

	// pin.Declare(amp.AppChannel[*appInst]{
	// 	Spec: amp.TabCatalogSpec,
	// 	Marshaller: func() {
	// 		pin.Upsert(cat.ID, amp.TagTabSpec.ID, tag.Nil, &cat.Tab)
	// 		for _, sta := range cat.stations {
	// 			pin.Upsert(sta.ID, amp.TagTabSpec.ID, tag.Nil, &sta.Tab)
	// 			pin.Upsert(sta.ID, av.PlayableMediaSpec.ID, tag.Nil, &sta.playable)
	// 		}
	// 	},
	// })

	// pin.Declare(amp.AppChannel[*appInst]{
	// 	Spec: av.MediaPlaylistSpec,
	// 	Marshaller: func(){
	// 		for _, sta := range cat.stations {
	// 			pin.Upsert(cat.ID, av.PlayableMediaSpec.ID, tag.Nil, &sta.playable)
	// 		}
	// 	},
	// })
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

		glyphTag := &amp.Tag{
			Use:         amp.TagUse_Glyph,
			URL:         "file://tunr/" + sta.info.Image,
			ContentType: amp.GenericImageType,
		}

		sta.Tab = amp.TagTab{
			Label:   sta.info.Title,
			Caption: sta.info.Summary,
			About:   sta.info.Description,
			Tags: []*amp.Tag{
				glyphTag,
			},
		}

		sta.playable = av.PlayableMedia{
			Flags: av.MediaFlags_HasAudio,
			Media: &amp.TagTab{
				Label: sta.info.Title,
				Tags: []*amp.Tag{
					glyphTag,
				},
			},
			Author: &amp.TagTab{
				Label: sta.info.Author,
			},
			Collection: &amp.TagTab{
				Label: sta.info.Category,
			},
		}

		if created, err := time.Parse(time.RFC3339, cat.info.TimestampCreated); err == nil {
			sta.Tab.SetCreatedAt(created)
		}
		if modified, err := time.Parse(time.RFC3339, cat.info.TimestampModified); err == nil {
			sta.Tab.SetModifiedAt(modified)
		}

		// [<URL>[:::<kbitrate>[:::<MIME type>]];]*
		// Just choose the first URL for now
		url := sta.info.Url
		if N := strings.IndexByte(url, ';'); N > 0 {
			url = url[:N]
		}
		parts := strings.Split(url, ":::")

		// FIX ME
		if len(parts) > 0 && len(parts[0]) > 0 {
			playable := &amp.Tag{
				URL: parts[0],
			}
			if len(parts) >= 3 {
				playable.ContentType = parts[2]
			} else {
				playable.ContentType = "audio/*"
			}

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
	basic.CellInfo[*appInst]
	info     av.StationInfo
	playable av.PlayableMedia
}

func (sta *station) MarshalAttrs(pin *basic.Pin[*appInst]) {
	sta.CellInfo.MarshalAttrs(pin)
	pin.Upsert(sta.ID, av.PlayableMediaSpec.ID, tag.Nil, &sta.playable)
}
