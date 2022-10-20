package vibe

import (
	"sync/atomic"

	"github.com/genesis3systems/go-planet/planet"
)

type vibeApp struct {
    nextID uint64 
}

func (app *vibeApp) AppURI() string {
    return AppURI
}



func (app *vibeApp) DataModelURIs() []string {
    return []string {
        "pin/app/home",
    }
}

// IssueEphemeralID issued a new ID that will persist
func (app *vibeApp) IssueEphemeralID() planet.CellID {
	return planet.CellID(atomic.AddUint64(&app.nextID, 1) + 100)
}


func (app *vibeApp) ResolveRequest(req *planet.CellReq) error {
    req.Target = app.IssueEphemeralID()
    return nil
}

func (app *vibeApp) ServeCell(sub planet.CellSub) error {

    req := sub.Req()
    
    cellID := req.Target.U64()
    childID := app.IssueEphemeralID().U64()  // TEMP

    batch := planet.NewMsgBatch()
    for i, msg := range batch.AddNew(4) {
        msg.ReqID = req.ReqID
        
        switch i {
        case 0:
            msg.Op = planet.MsgOp_PushAttr
            msg.TargetCellID = cellID
        case 1:
            msg.Op = planet.MsgOp_InsertChildCell
            msg.TargetCellID = childID
            msg.ValType = uint64(planet.ValType_SchemaID)
            msg.ValInt = int64(req.PinChildren[0].SchemaID)
        case 2:
            msg.Op = planet.MsgOp_PushAttr
            msg.TargetCellID = childID
            msg.AttrID = req.PinChildren[0].Attrs[1].AttrID // main.label
            msg.SetVal("Hello World")
        case 3:
            msg.Op = planet.MsgOp_Commit
            msg.TargetCellID = cellID
        }
    }
    
    sub.Cell().PushUpdate(batch)
    batch.Reclaim()
    
    return nil
}