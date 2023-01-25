package amp

import "github.com/arcspace/go-arcspace/arc"

const (
	AppURI = "arcspace.systems/amp.app/v1.2023.1"

	// CellModelURIs
	CellModel_Dir      = "amp.dir.v1.model"
	CellModel_Playlist = "amp.playlist.v1.model"
	CellModel_Playable = "amp.playable.v1.model"
)

func NewApp() arc.App {
	return &ampApp{}
}
