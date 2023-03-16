package symbol

import (
	"encoding/binary"
	"errors"
	"sync/atomic"

	"github.com/dgraph-io/badger/v4"
)

var ErrClosed = errors.New("issuer is closed")

// issuer implements symbol.Issuer
type issuer struct {
	db        *badger.DB
	nextIDSeq *badger.Sequence
	nextID    uint64 // Only used if db == nil
}

func openIssuer(db *badger.DB, opts TableOpts) (Issuer, error) {
	iss := &issuer{
		db:     db,
		nextID: MinIssuedID,
	}

	if iss.db != nil {
		seqKey := []byte{opts.DbKeyPrefix, 0xFF, xNextID}
		txn := db.NewTransaction(true)
		defer txn.Discard()

		// Initialize the sequence key if not present
		_, err := txn.Get(seqKey)
		if err == badger.ErrKeyNotFound {
			var buf [8]byte

			// TODO: re-implement with the ID value being IDSz (vs 8)
			binary.BigEndian.PutUint64(buf[:], MinIssuedID)
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

	return iss, nil
}

func (iss *issuer) IssueNextID() (ID, error) {
	var nextID uint64
	var err error

	if iss.db != nil {
		nextID, err = iss.nextIDSeq.Next()
		if err != nil {
			panic(err)
		}
	} else if iss.nextID != 0 {
		nextID = atomic.AddUint64(&iss.nextID, 1)
	}

	return ID(nextID), nil
}

func (iss *issuer) Close() {
	if iss.db != nil {
		if iss.nextIDSeq != nil {
			iss.nextIDSeq.Release()
			iss.nextIDSeq = nil
		}
		iss.db = nil
	}
	iss.nextID = 0
}
