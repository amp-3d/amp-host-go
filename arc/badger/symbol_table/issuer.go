package symbol_table

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/dgraph-io/badger/v4"
)

func NewBadgerIssuer(db *badger.DB, dbKeyPrefix byte, initsAt symbol.ID) (symbol.Issuer, error) {
	iss := &badgerIssuer{
		db: db,
	}

	if iss.db != nil {
		seqKey := append([]byte{}, dbKeyPrefix, 0xFF, xNextID)
		txn := db.NewTransaction(true)
		defer txn.Discard()

		// Initialize the sequence key if not present
		_, err := txn.Get(seqKey)
		if err == badger.ErrKeyNotFound {
			var buf [8]byte

			// TODO: re-implement with the ID value being IDSz (vs 8)
			binary.BigEndian.PutUint64(buf[:], uint64(initsAt))
			err = txn.Set(seqKey, buf[:])
			if err == nil {
				err = txn.Commit()
			}
		}

		if err == nil {
			iss.nextIDSeq, err = iss.db.GetSequence(seqKey, 300)
		}
		if err != nil {
			return nil, err
		}
	}

	iss.refCount.Store(1)
	return iss, nil
}

// badgerIssuer implements symbol.Issuer using badger.DB
type badgerIssuer struct {
	db        *badger.DB
	nextIDSeq *badger.Sequence
	refCount  atomic.Int32
}

func (iss *badgerIssuer) IssueNextID() (symbol.ID, error) {
	if iss.db == nil {
		return 0, symbol.ErrIssuerNotOpen
	}

	nextID, err := iss.nextIDSeq.Next()
	return symbol.ID(nextID), err
}

func (iss *badgerIssuer) AddRef() {
	if iss.refCount.Add(1) <= 1 {
		panic("AddRef() called on closed issuer")
	}
}

func (iss *badgerIssuer) Close() error {
	newRefCount := iss.refCount.Add(-1)
	if newRefCount < 0 {
		return symbol.ErrIssuerNotOpen
	}
	if newRefCount > 0 {
		return nil
	}
	return iss.close()
}

func (iss *badgerIssuer) close() error {
	if iss.db == nil {
		return nil
	}
	if iss.nextIDSeq != nil {
		iss.nextIDSeq.Release()
		iss.nextIDSeq = nil
	}
	err := iss.db.Close()
	iss.db = nil
	return err
}
