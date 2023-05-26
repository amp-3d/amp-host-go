package api

import "github.com/arcspace/go-archost/arc"

const (
	AmpAppURI = "arcspace.systems.amp.app"

	// KwArg names
	KwArg_Provider = "amp.provider" // specifies an amp media or system provider - e.g. "amp:", "filesys:", "spotify:"
	KwArg_CellURI  = "amp.cell.uri" // specifies a cell URI that the specified provider interprets and resolves
)

// Provider scheme IDs
const (
	Provider_Amp     = "amp:"
	Provider_FileSys = "filesys:"
	Provider_Spotify = "spotify:"
)

// MimeType (aka MediaType)
const (
	MimeType_Dir      = "application/x-directory"
	MimeType_Album    = "application/x-album"
	MimeType_Playlist = "application/x-playlist"
	// MimeType_Audio    = "audio/x-playable"
	// MimeType_Video    = "video/x-playable"
)

// CellDataModel
const (
	CellDataModel_Playlist = "amp.playlist.v1.model"
	CellDataModel_Playable = "amp.playable.v1.model"
	CellDataModel_Dir      = "amp.directory.v1.model"
)

var DataModels = arc.DataModelMap{
	ModelsByID: map[string]arc.DataModel{
		CellDataModel_Playlist: {},
		CellDataModel_Playable: {},
		CellDataModel_Dir:      {},
	},
}

// AttrURI
const (
	Attr_Title        = "name.string"
	Attr_Subtitle     = "subtitle.string"
	Attr_ArtistDesc   = "artist-desc.string"
	Attr_AlbumDesc    = "album-desc.string"
	Attr_Glyph        = "glyph.AssetRef"    // a 2D or 3D symbolic representation of something, similar in purpose to a thumbnail or icon.
	Attr_World        = "world.AssetRef"    // a 3D visually detailed volume (e.g. a world to step into or even a scene; a screen saver)
	Attr_Playable     = "playable.AssetRef" // a media item experience described as "playable" and typically having a set duration (but could be a live feed).
	Attr_ItemCount    = "item-count.int"
	Attr_LastModified = "modified.DateTime"
	Attr_ByteSz       = "size.bytes.int"
	Attr_Duration     = "duration.seconds.float"
)
