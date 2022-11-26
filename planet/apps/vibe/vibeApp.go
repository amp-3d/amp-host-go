package vibe

import (
	"sync/atomic"

	"github.com/arcverse/go-planet/planet"
)

type vibeApp struct {
	nextID uint64
}

func (app *vibeApp) AppURI() string {
	return AppURI
}

func (app *vibeApp) DataModelURIs() []string {
	return []string{
		PinAppHome,
		pinAppSettings,
		pinAppFiles,
		pinAppStations,
	}
}

// IssueEphemeralID issued a new ID that will persist
func (app *vibeApp) IssueEphemeralID() planet.CellID {
	return planet.CellID(atomic.AddUint64(&app.nextID, 1) + 100)
}

func (app *vibeApp) ResolveRequest(req *planet.CellReq) error {
	req.PinCell = app.IssueEphemeralID()
	return nil
}

func (app *vibeApp) PushCellState(sub planet.CellSub) error {
	return nil
}
