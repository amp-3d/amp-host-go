package arc

import (
	"github.com/arcspace/go-cedar/process"
)

/*
packages

	arc
	    Arcspace interfaces and support utils
	arc/host
	    an implementation of arc.Host & arc.HostSession
	arc/grpc_service
		implements a grpc server that consumes a arc.Host instance
	arc/apps
		implementations of arc.App


	archost process.Context model:
		001 Host
		    002 HostHomePlanet
		        004 HostSession
		        007 cell_101
		    003 grpc.HostService
		        005 grpc <- HostSession(4)
		        006 grpc -> HostSession(4)

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
	GetSchemaByID(schemaID int32) (*AttrSchema, error)
}

// Host is the highest level controller.
// Child processes attach to it and start new host sessions as needed.
type Host interface {
	Context

	HostPlanet() Planet

	// Registers an App for invocation based on its AppURI and SupportedDataModels.
	RegisterApp(app App) error

	// Selects an App, typically based on schema.CellDataModel (or schema.AppURI if given).
	// The given schema is READ ONLY.
	SelectAppForSchema(schema *AttrSchema) (App, error)

	// StartNewSession creates a new HostSession and binds its Msg transport to the given steam.
	StartNewSession(parent HostService, via ServerStream) (HostSession, error)
}

// HostSession in an open session instance with a Host.
// Closing is initiated via Context.Close().
type HostSession interface {
	Context

	// Threadsafe
	TypeRegistry

	// Atomically issues a new and unique ID that will remain unique for the duration of this session.
	// An ID may still expire, go out of scope, or otherwise become meaningless.
	IssueCellID() CellID

	LoggedIn() User
}

// HostService attaches to a arc.Host as a child process, extending host functionality (e.g. Grpc Msg transport).
type HostService interface {
	Context

	// Returns short string identifying this service
	ServiceURI() string

	// Returns the parent Host this extension is attached to.
	Host() Host

	// StartService attaches a child process to the given host and starts this HostService.
	StartService(on Host) error

	// GracefulStop initiates a polite stop of this extension and blocks until it's in a "soft" closed state,
	//    meaning that its service has effectively stopped but its Context is still open.
	// Note this could any amount of time (e.g. until all open requests are closed)
	// Typically, GracefulStop() is called (blocking) and then Context.Close().
	// To stop immediately, Context.Close() is always available.
	GracefulStop()
}

var ErrStreamClosed = ErrCode_Disconnected.Error("stream closed")

// ServerStream wraps a Msg transport abstraction, allowing a Host to connect over any data transport layer.
// This is intended to be implemented by a grpc and other transport layers.
type ServerStream interface {

	// Describes this stream
	Desc() string

	// Called when this stream to be closed because the associated parent host session is closing or has closed.
	Close()

	// SendMsg sends a Msg to the remote client.
	// ErrStreamClosed is used to denote normal stream close.
	// Like grpc.ServerStream.SendMsg(), on exit, the Msg has been copied and so can be reused.
	SendMsg(m *Msg) error

	// RecvMsg blocks until it receives a Msg or the stream is done.
	// ErrStreamClosed is used to denote normal stream close.
	RecvMsg() (*Msg, error)
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

	// WIP
	PushTx(tx *MsgBatch) error
	ReadCell(cellKey []byte, schema *AttrSchema, msgs func(msg *Msg)) error

	//GetCell(ID CellID) (CellInstance, error)

	// BlobStore offers access to this planet's blob store (referenced via ValueType_BlobID).
	//blob.Store

}

type CellID uint64

// U64 is a convenience method that converts a CellID to a uint64.
func (ID CellID) U64() uint64 { return uint64(ID) }

// See api.support.go for CellReq helper methods such as PushMsg.
type CellReq struct {
	CellSub

	ReqID         uint64        // Client-set request ID
	Args          []*KwArg      // Client-set args (typically used when pinning a root where CellID is not known)
	CellID        CellID        // Client-set cell ID to pin (or 0 if Args sufficient).  Use req.Cell.ID() for the resolved CellID.
	ContentSchema *AttrSchema   // Client-set schema specifying the cell attr model for the cell being pinned.
	ChildSchemas  []*AttrSchema // Client-set schema(s) specifying which child cells (and attrs) should be pushed to the client.
	User          User          // Runtime-set; the user that initiated this request
	ParentReq     *CellReq      // Runtime-set; allows arc.App and arc.AppCell to access the parent context
	ParentApp     App           // Runtime-set using SelectAppForSchema()
	Cell          AppCell       // Runtime-set from AppCell.PinCell()
}

// Signals to use the default App for a given AttrSchema CellDataModel.
// See AttrSchema.AppURI in arc.proto
const DefaultAppForDataModel = "."

// App creates a new Channel instance on demand when arc.GetChannel() is called.
// App and AppChannel consume the Planet and Channel interfaces to perform specialized functionality.
// In general, a channel app should be specialized for a specific, taking inspiration from the legacy of unix util way-of-thinking.
type App interface {

	// Identifies this App and usually has the form: "{domain_name}/{app_identifier}/v{MAJOR}.{MINOR}.{REV}"
	AppURI() string

	// SupportedDataModels lists data models that this app supports / handles.
	// When the host session receives a client request for a specific data model URI, it will route it to the app that registered for it here.
	SupportedDataModels() []string

	// Handles a client request to pin a cell, potentially looking at KwArgs and ChildSchemas to make choices.
	// The implementation sets req.Cell so that subsequent calls to PushCellState() are possible.
	PinCell(req *CellReq) error
}

// PushCellOpts specifies how an AppCell should be pushed to the client
type PushCellOpts uint32
const (
	PushAsParent PushCellOpts = 1 << iota
	PushAsChild
)
func (opts PushCellOpts) PushAsParent() bool { return opts&PushAsParent != 0 }
func (opts PushCellOpts) PushAsChild() bool { return opts&PushAsChild != 0 }

// AppCell is how an App offers a cell instance to the planet runtime.
type AppCell interface {

	// Returns the CellID of this cell
	ID() CellID

	// Names the data model that this cell implements.
	CellDataModel() string

	// Called when a cell is pinned and should push its state (in accordance with req.ContentSchema & req.ChildSchemas supplied by the client).
	// The implementation uses req.CellSub.PushMsg(...) to push attributes and child cells to the client.
	// Called on the goroutine owned by the the target CellID.
	PushCellState(req *CellReq, opts PushCellOpts) error

	// PinCell is called when a client requests to either pin the cell itself or one of its children (based on req.CellID and req.ParentReq)
	// This allows Apps to interoperate and query other App cells through the Arc runtime (without direct dependencies).
	PinCell(req *CellReq) error
}

type CellSub interface {

	// Sets msg.ReqID and pushes the given msg to client, blocking until "complete" (queued) or canceled.
	// This msg is reclaimed after it is sent, so it should be accessed following this call.
	PushMsg(msg *Msg) error
}

type User interface {
	Session() HostSession

	HomePlanet() Planet

	// Uses reflection to build an AttrSchema given a ptr to a struct.
	MakeSchemaForStruct(app App, structPtr any) (*AttrSchema, error)

	// High level function that loads a cell with the given URI having the inferred schema (built from its fields using reflection).
	// The URI is scoped from the user's home planet and App URI.
	ReadCell(ctx App, URI string, dst any) error

	// WriteCell is the write analog of ReadCell.
	WriteCell(ctx App, URI string, src any) error
}

// MsgBatch is an ordered list os Msgs
// See NewMsgBatch()
type MsgBatch struct {
	Msgs []*Msg
}

// LoadAttrs gets the most up to date values of the requested attr IDs.
// Returns the number of attrs that were not found or could be exported to the target dst value type.
// If fromID == 0, then all participant members are selected, otherwise only attrs set from the specified participant are selected.
// If nodeID == 0, then all nodes in this channel are selected, otherwise only attrs set for the specified nodeID are selected.
//LoadAttrs(fromID, nodeID symbol.ID, srcAttrs []symbol.ID, dstVals []interface{}) (int, error)
