package symbol

import (
	"github.com/dgraph-io/badger/v3"
)

// Creates a new symbol.Table and attaches it to the given db.
// If db == nil, the symbol.Table will operate in memory-only mode.
func OpenTable(db *badger.DB, opts TableOpts) (Table, error) {
	return openTable(db, opts)
}

// ID is a persistent integer value associated with an immutable string or buffer value.
// For convenience, Table's interface methods accept and return uint64 types, but these are understood to be of type ID.
// ID == 0 denotes nil or unassigned.
type ID uint64

// IDSz is the byte size of a Table ID (big endian)
// This means an app issuing a new symbol ID every millisecond would need 35 years to hit the limit.
// Even in this case, a limiting factor is storage (2^40 symbols x 100 bytes => ~100TB)
const IDSz = 5

// MinIssuedID specifies a minimum ID value for newly issued IDs.
//
// ID values less than this value are reserved for clients to represent hard-wired or "out of band" meaning.
// "Hard-wired" meaning that Table.SetSymbolID() can be called with IDs less than MinIssuedID without risk
// of an auto-issued ID contending with it.
const MinIssuedID = 1000

type Issuer interface {

	// Issues the next sequential unique ID, starting at MinIssuedID.
	IssueNextID() (ID, error)

	// Releases internal references to the underlying database.
	Close()
}

// Table stores value-ID pairs, designed for high-performance lookup of an ID or byte string.
// This implementation is indended to handle extreme loads, leveraging:
//      - ID-value pairs are cached once read, offering subsequent O(1) access
//      - Internal value allocations are pooled. The default TableOpts.PoolSz of 16k means
//        thousands of buffers can be issued or read under only a single allocation.
//
// All calls are threadsafe.
type Table interface {

	// Returns the Issuer being used by this Table (passed via TableOpts.Issuer or auto-created if TableOpts.Issuer was not given)
	Issuer() Issuer 
	
	// Returns the symbol ID previously associated with the given string/buffer value.
	// The given value buffer is never retained.
	//
	// If not found and autoIssue == true, a new entry is created and the new ID returned.
	// Newly issued IDs are always > 0 and use the lower bytes of the returned ID (see type ID comments).
	//
	// If not found and autoIssue == false, 0 is returned.
	GetSymbolID(value []byte, autoIssue bool) ID

	// Associates the given buffer value to the given symbol ID, allowing multiple values to be mapped to a single ID.
	// If ID == 0, then this is the equivalent to GetSymbolID(value, true).
	SetSymbolID(value []byte, ID ID) ID

	// Looks up and returns the byte string previously associated with the given symbol ID.
	// The returned buf conveniently retains scope indefinitely (and is read only).
	// If ID is invalid or not found, nil is returned.
	LookupID(ID ID) []byte

	// Releases internal references to the underlying database.
	// Subsequent access to this Table instance is defined but limited to what is already cached.
	Close()
}

type TableOpts struct {
	Issuer          Issuer // How this table will issue new IDs.  If nil, this table's db will be used as the Issuer
	IssuerOwned     bool   // If set, Issuer.Close() will be called when this table is closed
	DbKeyPrefix     byte   // Key prefix for persistent db mode (n/a when in memory-only mode)
	WorkingSizeHint int    // anticipated number of entries in working set
	PoolSz          int32  // Value backing buffer allocation pool sz
}

// DefaultOpts is a suggested set of options.
var DefaultTableOpts = TableOpts{
	WorkingSizeHint: 600,
	PoolSz:          16 * 1024,
	DbKeyPrefix:     0xFC,
}

func AppendID(ID uint64, io []byte) []byte {
	return append(io, // big endian marshal
		byte(ID>>32),
		byte(ID>>24),
		byte(ID>>16),
		byte(ID>>8),
		byte(ID))
}

func ReadID(in []byte) (uint64, []byte) {
	ID := ((uint64(in[0]) << 32) |
		(uint64(in[1]) << 24) |
		(uint64(in[2]) << 16) |
		(uint64(in[3]) << 8) |
		(uint64(in[4])))

	return ID, in[5:]
}
