package amp

import "github.com/arcspace/go-arcspace/arc"

const (
	AppURI = "arcspace.systems/amp.app/v1.2023.1"

	// KwArg names
	KwArg_PinRoot = "pin-root" // pins a new root -- "home", "settings", "filesys"
	KwArg_PinPath = "pin-path" // used with PinRoot="filesys" to specify a file system path to pin
)

// MimeType
const (
	MimeType_Dir      = "application/x-directory"
	MimeType_Playlist = "application/x-playlist"
	MimeType_Audio    = "audio/x-playable"
	MimeType_Video    = "video/x-playable"
)

// CellDataModel
const (
	CellDataModel_Playlist = "amp.playlist.v1.model"
	CellDataModel_Playable = "amp.playable.v1.model"
)

// AttrURI
const (
	Attr_ItemName  = "name.string"
	Attr_Subtitle  = "subtitle.string"
	Attr_Glyph     = "glyph.AssetRef"    // a 2D or 3D symbolic representation of something, similar in purpose to a thumbnail or icon.
	Attr_World     = "world.AssetRef"    // a 3D visually detailed volume (e.g. a world to step into or even a scene; a screen saver)
	Attr_Playable  = "playable.AssetRef" // a media item experience described as "playable" and typically having a set duration (but could be a live feed).
	Attr_Artist    = "artist.string"
	Attr_ItemCount = "item-count.int"
)

func NewApp() arc.App {
	return &ampApp{}
}
