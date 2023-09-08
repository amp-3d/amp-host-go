package symbol_table

import (
	"sync/atomic"

	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/dgraph-io/badger/v4"
)

const (
	xByID    = byte(0)
	xByValue = byte('$')
	xNextID  = byte('N')
)

func (st *symbolTable) Issuer() symbol.Issuer {
	return st.opts.Issuer
}

func (st *symbolTable) AddRef() {
	st.refCount.Add(1)
}

func (st *symbolTable) Close() error {
	if st.refCount.Add(-1) > 0 {
		return nil
	}
	return st.close()
}

func (st *symbolTable) close() error {
	err := st.opts.Issuer.Close()
	st.opts.Issuer = nil

	if st.dbOwned {
		err = st.opts.Db.Close()
	}
	st.opts.Db = nil
	return err
}

// symbolTable implements symbol.Table using badger (a LSM db) as the backing store.
//
// All methods are concurrency-safe.
type symbolTable struct {
	opts     TableOpts
	refCount atomic.Int32
	dbOwned  bool
}

func (st *symbolTable) GetSymbolID(val []byte, autoIssue bool) symbol.ID {
	symID := st.getsetValueIDPair(val, 0, autoIssue)
	return symID
}

func (st *symbolTable) SetSymbolID(val []byte, symID symbol.ID) symbol.ID {
	// If symID == 0, then behave like GetSymbolID(val, true)
	return st.getsetValueIDPair(val, symID, symID == 0)
}

// getsetValueIDPair loads and returns the ID for the given value, and/or writes the ID and value assignment to the db,
// also updating the cache in the process.
//
//	if symID == 0:
//	  if the given value has an existing value-ID association:
//	      the existing ID is cached and returned (mapID is ignored).
//	  if the given value does NOT have an existing value-ID association:
//	      if mapID == false, the call has no effect and 0 is returned.
//	      if mapID == true, a new ID is issued and new value-to-ID and ID-to-value assignments are written,
//
//	if symID != 0:
//	    if mapID == false, a new value-to-ID assignment is (over)written and any existing ID-to-value assignment remains.
//	    if mapID == true, both value-to-ID and ID-to-value assignments are (over)written.
func (st *symbolTable) getsetValueIDPair(val []byte, symID symbol.ID, mapID bool) symbol.ID {

	// The empty string is always hard-wired to ID 0
	if len(val) == 0 {
		return 0
	}

	{
		txn := st.opts.Db.NewTransaction(true)
		defer txn.Discard()

		// The value index is placed after the ID index
		var (
			keyBuf [128]byte
			idBuf  [8]byte
			err    error
		)

		keyBuf[0] = st.opts.DbKeyPrefix
		keyBuf[1] = xByValue
		valKey := append(keyBuf[:2], val...)

		var existingID symbol.ID
		if symID == 0 || !mapID {

			// Lookup the given value and get its existing ID
			item, err := txn.Get(valKey)
			if err == nil {
				item.Value(func(buf []byte) error {
					if len(buf) == symbol.IDSz {
						existingID.ReadFrom(buf)
					}
					return nil
				})
			}
		}

		reassignID := false
		reassignVal := false

		if symID == 0 {
			if existingID != 0 {
				symID = existingID
			} else if mapID {
				symID, err = st.opts.Issuer.IssueNextID()
				if err == nil {
					reassignID = true
					reassignVal = true
				}
			}
		} else {
			if existingID == 0 {
				reassignVal = true
				reassignID = true
			} else if symID != existingID {
				reassignVal = true
				if mapID {
					symID = existingID
					reassignID = true
				}
			}
		}

		// If applicable, flush the new kv assignment change to the db
		for err == nil && (reassignID || reassignVal) {

			// set (value => ID) entry
			idBuf[0] = st.opts.DbKeyPrefix
			idBuf[1] = xByID
			idKey := symID.AppendTo(idBuf[:2])
			err := txn.Set(valKey, idKey[2:])
			if err == nil {
				if reassignID {
					err = txn.Set(idKey, val)
				}
				if err == nil {
					err = txn.Commit()
				}
			}

			if err != badger.ErrConflict {
				break
			}

			err = nil
			txn.Discard()
			txn = st.opts.Db.NewTransaction(true)
		}

		if err != nil {
			panic(err)
		}
	}

	return symID
}

func (st *symbolTable) GetSymbol(symID symbol.ID, io []byte) []byte {
	db := st.opts.Db
	if symID == 0 || db == nil {
		return nil
	}

	txn := db.NewTransaction(false)
	defer txn.Discard()

	var idBuf [8]byte
	idBuf[0] = st.opts.DbKeyPrefix
	idBuf[1] = xByID
	tokenKey := symID.AppendTo(idBuf[:2])
	item, err := txn.Get(tokenKey)
	if err == nil {
		err = item.Value(func(val []byte) error {
			io = append(io, val...)
			return nil
		})
	}
	if err != nil {
		return nil
	}
	return io
}
