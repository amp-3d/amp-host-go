package arc

import (
	"reflect"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/process"
	"github.com/arcspace/go-archost/arc/assets"
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

// Registers the given app module for invocation -- threadsafe
func RegisterApp(app *AppModule) error {
	return gAppRegistry.RegisterApp(app)
}

// Gets an AppModule by URI -- threadsafe
func GetRegisteredApp(appID string) (*AppModule, error) {
	return gAppRegistry.GetAppByID(appID)
}

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

	// StartNewSession creates a new HostSession and binds its Msg transport to the given steam.
	StartNewSession(parent HostService, via ServerStream) (HostSession, error)
}

// HostSession in an open session instance with a Host.
// Closing is initiated via Context.Close().
type HostSession interface {
	Context

	// Thread-safe
	TypeRegistry

	// Returns the running AssetServer instance for this session.
	AssetServer() assets.AssetServer

	LoggedIn() User
}

// HostService attaches to a arc.Host as a child process, extending host functionality.
// For example. it wraps a Grpc-based Msg transport as well as a dll-based Msg transport implementation.
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

var (
	ErrStreamClosed = ErrCode_Disconnected.Error("stream closed")
	ErrCellNotFound = ErrCode_CellNotFound.Error("cell not found")
	ErrShuttingDown = ErrCode_ShuttingDown.Error("shutting down")
)

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
	GetSymbol(ID uint64, io []byte) []byte

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

	Args          []*KwArg      // Client-set args (typically used when pinning a root where CellID is not known)
	PinCell       CellID        // Client-set cell ID to pin (or 0 if Args sufficient).  Use req.Cell.ID() for the resolved CellID.
	ContentSchema *AttrSchema   // Client-set schema specifying the cell attr model for the cell being pinned.
	ChildSchemas  []*AttrSchema // Client-set schema(s) specifying which child cells (and attrs) should be pushed to the client.
}

// See AttrSchema.ScopeID in arc.proto
const ImpliedScopeForDataModel = "."

// type DataModel map[string]*Attr
type DataModel struct {
}

type DataModelMap struct {
	ModelsByID map[string]DataModel // Maps a data model ID to a data model definition
}

// AppModule declares a 3rd-party module this is registered with an archost.
// An app is invoked by its AppID directly or a client requesting a data model this app declares to support.
type AppModule struct {
	AppID      string       // "{domain_name}/{module_identifier}"
	Version    string       // "v{MAJOR}.{MINOR}.{REV}"
	DataModels DataModelMap // Data models that this app defines and handles.

	// Called when an App is invoked on an active User session and is not yet running.
	// Msg processing is blocked until this returns -- only AppRuntime calls should block.
	NewAppInstance func(ctx AppContext) (AppRuntime, error)
}

type CellContext interface {
	//Label() string

	// PinCell pins a requested cell, typically specified by req.PinCell.
	// req.KwArgs and ChildSchemas can also be used to specify the cell to pin.
	PinCell(req *CellReq) (AppCell, error)
}

type AppRuntime interface {
	CellContext

	HandleAppMsg(msg *AppMsg) (handled bool, err error)

	OnClosing()
}

type AppContext interface {
	process.Context    // Container for AppRuntime
	arc.AssetPublisher // Allows an app to publish assets for client consumption
	User() User        // Access to user operations and io
	CellContext        // How to pin cells

	// Atomically issues a new and unique ID that will remain globally unique for the duration of this session.
	// An ID may still expire, go out of scope, or otherwise become meaningless.
	IssueCellID() CellID

	// Unique state scope ID for this app instance -- defaults to AppID
	StateScope() string

	// Uses reflection to build and register (as necessary) an AttrSchema for a given a ptr to a struct.
	GetSchemaForType(typ reflect.Type) (*AttrSchema, error)

	// Loads the data stored at the given key, appends it to the given buffer, and returns the result (or an error).
	// The given subKey is scoped by both the app and the user so key collision with other users or apps is not possible.
	// Typically used by apps for holding high-level state or settings.
	GetAppValue(subKey string) (val []byte, err error)

	// Write analog for GetAppValue()
	PutAppValue(subKey string, val []byte) error
}

// PushCellOpts specifies how an AppCell should be pushed to the client
type PushCellOpts uint32

const (
	PushAsParent PushCellOpts = 1 << iota
	PushAsChild
)

func (opts PushCellOpts) PushAsParent() bool { return opts&PushAsParent != 0 }
func (opts PushCellOpts) PushAsChild() bool  { return opts&PushAsChild != 0 }

// type CellInfo struct {
// 	CellID
// 	CellDataModel string
// 	AppContext
// }

// PinnedCell?
// AppCell is how an App offers a cell instance to the planet runtime.
type AppCell interface {
	CellContext

	//AppContext() AppContext

	// Returns the CellID of this cell
	ID() CellID

	// Names the data model that this cell implements.
	CellDataModel() string

	// Called when a cell is pinned and should push its state (in accordance with req.ContentSchema & req.ChildSchemas supplied by the client).
	// The implementation uses req.CellSub.PushMsg(...) to push attributes and child cells to the client.
	// Called on the goroutine owned by the the target CellID.
	PushCellState(req *CellReq, opts PushCellOpts) error
}

type CellSub interface {

	// Sets msg.ReqID and pushes the given msg to client, blocking until "complete" (queued) or canceled.
	// This msg is reclaimed after it is sent, so it should be accessed following this call.
	PushMsg(msg *Msg) error
}

// User + HostSession --> UserSession?
type User interface {
	Session() HostSession

	HomePlanet() Planet

	LoginInfo() LoginReq

	// Gets the currently active AppContext for an AppID.
	// If does not exist and autoCreate is set, a new AppContext is created, started, and returned.
	GetAppContext(appID string, autoCreate bool) (AppContext, error)
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
