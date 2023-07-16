package amp_bcat

import (
	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
)

type categories struct {
	amp.CellBase[*appCtx]
	//items []*
}

func (cats *categories) ExportAttrs(app *appCtx, dst *arc.AttrBatch) error {
	dst.Add(app.CellInfoAttr, &arc.CellInfo{
		CellDefID: app.LinkCellSpec,
		Title:     "Internet Radio",
		Glyph:     amp.DirGlyph,
	})
	return nil
}

func (cats *categories) Label() string {
	return "Internet Radio"
}

type category struct {
	amp.CellBase[*appCtx]
	catID uint32 //
}

/*

func (cat *category) PinInto(dst *amp.PinnedCell[*appCtx]) error {

	// if cat.itemsByID == nil {
	// 	cat.itemsByID = map[arc.CellID]*station{}
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

	children := make([]*amp.CellBase[*appCtx], 0, 32)

	// while the array contains values
	for json.More() {
		var entry amp.StationInfo
		err := json.Decode(&entry)
		if err != nil {
			return err
		}
		sta := &station{
			links: entry.Url,
		}
		sta.CellBase.ResetState(dst.App.IssueCellID(), sta)
		sta.CellBase.AddAttr(dst.App, "", &arc.CellInfo{
			Title:    entry.Title,
			Subtitle: entry.Summary,
			About:    entry.Description,
			Glyph: &arc.AssetRef{
				URI:    entry.Image,
				Scheme: arc.URIScheme_File,
			},
		})
		sta.CellBase.AddAttr(dst.App, "", &amp.MediaInfo{
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
	amp.CellBase[*appCtx]
	links string
}

func (sta *station) ExportAttrs(app *appCtx, dst *arc.AttrBatch) error {

}

func (sta *station) PinInto(dst *amp.PinnedCell[*appCtx]) error {

	if len(sta.links) > 0 {

		// [<URL>[:::<kbitrate>[:::<MIME type>]];]*
		// Just choose the first URL for now
		URL := sta.links
		if N := strings.IndexByte(URL, ';'); N > 0 {
			URL = URL[:N]
		}
		parts := strings.Split(URL, ":::")

		if len(parts) > 0 && len(parts[0]) > 0 {
			playable := &arc.AssetRef{
				URI: parts[0],
			}
			if len(parts) >= 3 {
				playable.MediaType = parts[2]
			} else {
				playable.MediaType = "audio/unknown"
			}

			sta.CellBase.SetAttr(dst.App, amp.Attr_Playable, playable)
		}
	}

	return nil
}
*/
