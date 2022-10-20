package planet

import "github.com/genesis3systems/go-cedar/process"

/*
packages

planet
    is the set of APIs and API-consuming support utils
host
    implements planet.Host + HostSession
grpc_server
	implements a grpc server that consumes planet.Host + HostSession



May this project be dedicated to God, for all other things are darkness or imperfection.
May these hands and this mind be blessed with Holy Spirit and Holy Purpose.
May I be an instrument for manifesting software that serves the light and used to manifest joy at the largest scale possible.
May the blocks to this mission dissolve into light amidst God's will.
~ Dec 25th, 2021


*/

// TID identifies a specific planet, node, or transaction.
//
// Unless otherwise specified a TID in the wild should always be considered read-only.
type TID []byte

// TIDBuf is the blob version of a TID
type TIDBuf [TIDBinaryLen]byte



type Context interface {
	process.Context


}




type TypeRegistry interface {

    // Resolves and then registers each given def, returning the resolved defs in-place if successful.
    //
    // Resolving a AttrSchema means:
    //    1) all name identifiers have been resolved to their corresponding host-dependent symbol IDs.
    //    2) all "InheritsFrom" types and fields have been "flattened" into the form 
    //
    // See MsgOp_ResolveAndRegister
    ResolveAndRegister(defs *Defs) error
    
    // Returns the resolved AttrSchema for the given cell type ID.
	GetSchemaByID(schemaID uint32) (*AttrSchema, error)
}





// Host is the highest level controller. 
// Child processes attach to it and start new host sessions as needed.
type Host interface {
	Context
		
	HostPlanet() Planet
	
	RegisterApp(app App) error
	
	GetRegisteredApp(appURI string) (App, error)
		
    StartNewSession() (HostSession, error)
    
}


// HostEndpoint offers Msg pipe endpoint access, allowing it to be lifted over any Msg transport layer.
type HostEndpoint interface {
	Context
	
	// This provides Msg pipe endpoint access for lifting over a Msg transport layer.
	// This is intended to be consumed by a grpc (or other io layer).
	Inbox() chan *Msg
	Outbox() chan *Msg
	
}


// Planet is content and governance enclosure.
// A Planet is 1:1 with a KV database model, which works out well for efficiency and performance.
type Planet interface {
	
	// A Planet instance is a child process of a host
	Context
	
	PlanetID() uint64
	
	// A planet offers a persistent symbol table, allowing efficient compression of byte symbols into uint64s
	GetSymbolID(value []byte, autoIssue bool) (ID uint64)
	LookupID(ID uint64) []byte


	//GetCell(ID CellID) (CellInstance, error)

	
	// BlobStore offers access to this planet's blob store (referenced via ValueType_BlobID).
	//blob.Store
	
	
}



/*

Host (1)
    * Home (2)
        * hostSess (8)
	* MountedPlanets
		* Planet1
		* PlanetXYZ
    * GrpcServer (3)
        * grpcSess (9)
            * rpc.SendToClient (10)
            * rpc.RecvFromClient (11)



type MsgPipe interface {
	Context

	// A host servicing this request *sends* to this channel while the client consumes messages
	// When Msg.IsClosed, this will be the last message sent on this channel.
	// Future: this chan is a special overflow chan than won't ever overflow
	ToClient() chan<- *Msg
	
}

*/




// HostSession has two implementations:
//    1) host (processes Msg inbox+outbox, maintains open reqs, etc)
//      PinNode() opens a new req and serves it
//    2)  client (processes Msg inbox+outbox and is a model for C# / Unity)
//      PinNode() generates a request and waits for responses
// HostSession is intended to be consumed by a Msg transport layer that in turn is intended
// to be consumed by an implementation of a client.HostSession.
type HostSession interface {
	HostEndpoint

	// Threadsafe
	TypeRegistry
	
	LoggedIn() User
	
	//UserPlanet() Planet
	
	//GetCell(cellID uint64) (CellInstance, error)
	
	//OnCellOpened(CellInstance) error

	// Pin requests that the Host serve or "pin" a requested Planet Node (or expand the currently mapped or "pinned" attr sub-range).
	// 
	// Client request to pin a range of attr values.
	// For each attr item that is a node ref, the host:
	//    1) generates a new PinID for that instance
	//    2) pushes all attrs for that node (recursively auto-pinning subs based on AttrDef.AutoPin)
	//    3) sends a MsgOp_NodeUpdated when requested range is sent 
	//    4) any updates to the pinned range will be pushed, termianted by MsgOp_NodeUpdated, etc.
	//Pin(req PinReq, pipe MsgPipe) error
	  
    
    // Client request to reconstruct a given node based on its underlying node type (i.e. attr map).
    // The host pushes attr value updates though this request's Msg pipe.
    // For the given node and expected type (i.e. attr map), this loads the requested node's root attr values (values, nodeID, or nodeSetID).
    // Typically called by a client (or invoked via a client-sent MsgOp)
    //PinCell(req CellReq) error
	
	
	// Initiates connection shutdown, cancelling all pending requests and declining any new requests.
	// If/when 
    //PoliteClose() 
    
}


type CellID uint64
func (ID CellID) U64() uint64 { return uint64(ID) }

type CellReq struct {
	ReqID        uint64
	Parent       *CellReq
	PlanetID     uint64
	Target       CellID
	URI          string
	PinSchema    *AttrSchema
	PinChildren  []*AttrSchema
	
}
// App creates a new Channel instance on demand when Planet.GetChannel() is called.
// App and AppChannel consume the Planet and Channel interfaces to perform specialized functionality.
// In general, a channel app should be specialized for a specific, taking inspiration from the legacy of unix util way-of-thinking.
type App interface {

	// Identifies this App's functionality and author.
	AppURI() string
	
	// Maps a set of data model URIs to this App for handling.
	DataModelURIs() []string
		
	// Attaches the given request to the requested cell until it is canceled or completed.
	//PinCell(req *CellReq) (CellSub, error)
	
	// Resolves the given request to final target Planet and CellID that used to   
	ResolveRequest(req *CellReq) error
	
	ServeCell(sub CellSub) error

	// Creates a new App instance that is bound to the given channel and starts it as a "child process" of the host / bound channel
	// Blocks until the new AppChannel is in a valid and ready state.
	// Typically, the returned AppChannel is upcast to the desired/presumed Channel interface.
	//StartAppInstance(sess CellSession) (AppCell, error)
}


// OpenCell?
type CellInstance interface {
	Context

	//CellID() uint64
	
	// AddSub(echo CellSub) CellSub
	// RemoveSub(sub CellSub)
	
	PushUpdate(batch MsgBatch)
	
	
	// Callback when a sub has been added and is awaiting a state push 
	//OnSubPinned(sub CellSub)

	//PushTx(tx) error
}



type CellSub interface {

	Req() *CellReq
	
	Cell() CellInstance
		
	PushUpdate(batch MsgBatch) error
}


type User interface {
	HomePlanet() Planet
}

// MsgBatch is an ordered list os Msgs
// See NewMsgBatch() for construction
type MsgBatch interface {
	
	AddNew(count int) []*Msg
	
	AddMsgs(msgs []*Msg)

	AddMsg(msg *Msg)
	
	Msgs() []*Msg
	
	// Reclaim is called when this MsgBatch is no longer referenced
	Reclaim()
}


// // PinnedCell?
// // CellAccess?
// // CellReq?
// // CellPin?
// // CellSession?
// // CellSub?
// //
// // OpenCell reflects an open cell bound to the governance of its "home" planet.
// type OpenCell interface {

// 	// A channel instance is a child process of its home Planet
// 	Context
	
// 	// Identifies this App's functionality and author.
// 	ParentApp() App

	// // Returns the Planet hosting this Channel
	// HomePlanet() Planet
	
	// // GenesisTID returns the genesis TID of this Channel, in effect identifying this channel.
	// GenesisTID() TID
	
	// // NodeID returns the symbolic representation of this Channel aka "root" NodeID of a channel.
	// RootNode() NodeID
	
	//SnapshotChNode(nodeID symbol.ID, ChNode) ChNodeSnapshot
	// ChNode(nodeID symbol.ID) ChNode
	
	// LoadAttrs gets the most up to date values of the requested attr IDs.
	// Returns the number of attrs that were not found or could be exported to the target dst value type.
	// If fromID == 0, then all participant members are selected, otherwise only attrs set from the specified participant are selected.
	// If nodeID == 0, then all nodes in this channel are selected, otherwise only attrs set for the specified nodeID are selected.
	//LoadAttrs(fromID, nodeID symbol.ID, srcAttrs []symbol.ID, dstVals []interface{}) (int, error)
	
	// Called when the Channel is starting up.
	// The channel is presumed to read from the state and serve channel app clients specialized for that app type.
	//OnStart(state ChState)

	// NewTransaction() (TxWriter, error)
	
// }


