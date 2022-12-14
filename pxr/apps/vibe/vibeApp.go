package vibe

import (
	"sync/atomic"

	"github.com/arcspace/go-arcspace/pxr"
)

type vibeApp struct {
	nextID uint64
}

func (app *vibeApp) AppURI() string {
	return AppURI
}

func (app *vibeApp) AttrModelURIs() []string {
	return []string{
		PinAppHome,
		pinAppSettings,
		pinAppFiles,
		pinAppStations,
	}
}

// IssueEphemeralID issued a new ID that will persist
func (app *vibeApp) IssueEphemeralID() pxr.CellID {
	return pxr.CellID(atomic.AddUint64(&app.nextID, 1) + 100)
}

func (app *vibeApp) ResolveRequest(req *pxr.CellReq) error {
	req.PinCell = app.IssueEphemeralID()
	return nil
}

func (app *vibeApp) PushCellState(sub pxr.CellSub) error {
	return nil
}
