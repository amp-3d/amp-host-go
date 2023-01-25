package amp

import (
	"sync/atomic"

	"github.com/arcspace/go-arcspace/arc"
)

type ampApp struct {
	nextID uint64
}

func (app *ampApp) AppURI() string {
	return AppURI
}

func (app *ampApp) CellModelURIs() []string {
	return []string{
		CellModel_Dir,
		CellModel_Playlist,
		CellModel_Playable,
	}
}

// IssueEphemeralID issued a new ID that will persist
func (app *ampApp) IssueEphemeralID() arc.CellID {
	return arc.CellID(atomic.AddUint64(&app.nextID, 1) + 100)
}

func (app *ampApp) ResolveRequest(req *arc.CellReq) error {
	req.PinCell = app.IssueEphemeralID()
	return nil
}

func (app *ampApp) PushCellState(sub arc.CellSub) error {
	return nil
}
