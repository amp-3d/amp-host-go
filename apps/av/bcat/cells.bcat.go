package bcat

import (
	"github.com/amp-space/amp-host-go/apps/av"
	"github.com/amp-space/amp-sdk-go/amp"
)

type categories struct {
	av.CellBase[*appCtx]
	//items []*
}

func (cats *categories) MarshalAttrs(dst *amp.CellTx, ctx amp.PinContext) error {
	dst.Marshal(ctx.GetAttrID(amp.CellHeaderAttrSpec), 0, &amp.CellHeader{
		Title: "Internet Radio",
		Glyphs: []*amp.AssetTag{
			av.DirGlyph,
		},
	})
	return nil
}

func (cats *categories) GetLogLabel() string {
	return "Internet Radio"
}

type category struct {
	av.CellBase[*appCtx]
	catID uint32 //
}

/*

func (cat *category) PinInto(dst *av.PinnedCell[*appCtx]) error {

	// if cat.itemsByID == nil {
	// 	cat.itemsByID = map[amp.CellID]*station{}
	// }
	app := dst.App
	if err := app.makeReady(); err != nil {
		return err
	}

	params := url.Values{}
	params.Add("subtype", "S")
	params.Add("categories", strconv.Itoa(int(cat.catID)))

	json, err := dst.App.doReq("bookmarks/", params)
	if err != nil {
		return err
	}

	// read '['
	_, err = json.Token()
	if err != nil {
		return err
	}

	children := make([]*av.CellBase[*appCtx], 0, 32)

	// while the array contains values
	for json.More() {
		var entry av.StationInfo
		err := json.Decode(&entry)
		if err != nil {
			return err
		}
		sta := &station{
			links: entry.Url,
		}
		sta.CellBase.ResetState(dst.App.IssueCellID(), sta)
		sta.CellBase.AddAttr(dst.App, "", &amp.CellText{
			Title:    entry.Title,
			Subtitle: entry.Summary,
			About:    entry.Description,
			Glyph: &amp.AssetRef{
				URI:    entry.Image,
				Scheme: amp.URIScheme_File,
			},
		})
		sta.CellBase.AddAttr(dst.App, "", &av.PlayableMedia{
			AuthorDesc: entry.Author,
			Title:      entry.Title,
		})

		children = append(children, &sta.CellBase)

	}

	// read ']'
	_, err = json.Token()
	if err != nil {
		return err
	}

	dst.AddChildren(children)

	return nil
}

type station struct {
	av.CellBase[*appCtx]
	links string
}

func (sta *station) MarshalAttrs(app *appCtx, dst *amp.CellTx) error {

}

func (sta *station) PinInto(dst *av.PinnedCell[*appCtx]) error {

	if len(sta.links) > 0 {

		// [<URL>[:::<kbitrate>[:::<MIME type>]];]*
		// Just choose the first URL for now
		URL := sta.links
		if N := strings.IndexByte(URL, ';'); N > 0 {
			URL = URL[:N]
		}
		parts := strings.Split(URL, ":::")

		if len(parts) > 0 && len(parts[0]) > 0 {
			playable := &amp.AssetRef{
				URI: parts[0],
			}
			if len(parts) >= 3 {
				playable.MediaType = parts[2]
			} else {
				playable.MediaType = "audio/unknown"
			}

			sta.CellBase.SetAttr(dst.App, av.Attr_Playable, playable)
		}
	}

	return nil
}
*/
