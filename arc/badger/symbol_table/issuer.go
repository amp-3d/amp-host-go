package symbol_table

import (
	"encoding/binary"
	"errors"
	"sync/atomic"

	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/dgraph-io/badger/v4"
)

var ErrIssuerClosed = errors.New("issuer is closed")

func NewVolatileIssuer() symbol.Issuer {
	iss := &atomicIssuer{}
	iss.nextID.Store(symbol.MinIssuedID)
	iss.refCount.Store(1)
	return iss
}

// atomicIssuer implements symbol.Issuer using an atomic int
type atomicIssuer struct {
	nextID   atomic.Uint64
	refCount atomic.Int32
	closed   bool
}

func (iss *atomicIssuer) IssueNextID() (symbol.ID, error) {
	if iss.closed {
		return 0, ErrIssuerClosed
	}
	nextID := iss.nextID.Add(1)
	return symbol.ID(nextID), nil
}

func (iss *atomicIssuer) AddRef() {
	iss.refCount.Add(1)
}

func (iss *atomicIssuer) Close() error {
	if iss.refCount.Add(-1) > 0 {
		return nil
	}
	if iss.closed {
		return ErrIssuerClosed
	}
	iss.closed = true
	return nil
}

func NewBadgerIssuer(db *badger.DB, dbKeyPrefix byte) (symbol.Issuer, error) {
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
			binary.BigEndian.PutUint64(buf[:], symbol.MinIssuedID)
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
		return 0, ErrIssuerClosed
	}

	nextID, err := iss.nextIDSeq.Next()
	return symbol.ID(nextID), err
}

func (iss *badgerIssuer) AddRef() {
	iss.refCount.Add(1)
}

func (iss *badgerIssuer) Close() error {
	if iss.refCount.Add(-1) > 0 {
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
