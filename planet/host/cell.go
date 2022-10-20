package host

import (
	"sync"

	"github.com/genesis3systems/go-cedar/process"
	"github.com/genesis3systems/go-planet/planet"
)

// cellInst is a "mounted" cell servicing requests for a specific cell (typically one).
// This can be thought of as the controller for one or more active cell pins.
type cellInst struct {
	process.Context // TODO: make custom lightweight later

	//cid       planet.CellID
	pl       *planetSess
	subsHead *openReq // single linked list of open reqs on this cell
	subsMu   sync.Mutex
}

func (cell *cellInst) addSub(sub *openReq) {
	if sub.cell != cell {
		panic("not this sub")
	}
	sub.cell = cell

	cell.subsMu.Lock()
	{
		prev := &cell.subsHead
		for *prev != nil {
			prev = &((*prev).next)
		}
		*prev = sub
		sub.next = nil
	}
	cell.subsMu.Unlock()
}

func (cell *cellInst) removeSub(sub *openReq) {
	if sub.cell != cell {
		panic("not this sub")
	}
	sub.cell = nil

	cell.subsMu.Lock()
	{
		prev := &cell.subsHead
		for *prev != sub && *prev != nil {
			prev = &((*prev).next)
		}
		if *prev == sub {
			*prev = sub.next
			sub.next = nil
		} else {
			panic("failed to find sub")
		}
	}
	cell.subsMu.Unlock()

	// N := len(csess.subs)
	// for i := 0; i < N; i++ {
	// 	if csess.subs[i] == remove {
	// 		N--
	// 		csess.subs[i] = csess.subs[N]
	// 		csess.subs[N] = nil
	// 		csess.subs = csess.subs[:N]
	// 		break
	// 	}
	// }
}

func (cell *cellInst) PushUpdate(batch planet.MsgBatch) {

	cell.subsMu.Lock()
	defer cell.subsMu.Unlock()

	
	for sub := cell.subsHead; sub != nil; sub = sub.next{
		err := sub.PushUpdate(batch)
		if err != nil {
			panic(err)
			// sub.Error("dropping client due to error", err)
			// sub.Close()  // TODO: prevent deadlock since chSess.subsMu is locked
			// chSess.subs[i] = nil
		}
	}

}
