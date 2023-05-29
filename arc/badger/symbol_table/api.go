package symbol_table

import (
	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/dgraph-io/badger/v4"
)

type TableOpts struct {
	symbol.Issuer            // How this table will issue new IDs.  If nil, this table's db will be used as the Issuer
	Db            *badger.DB // Db is the badger.DB storing the table's key and value data. .
	DbKeyPrefix   byte       // Key prefix for persistent db mode (n/a for memory-only mode)
}

// DefaultOpts is a suggested set of options.
func DefaultOpts() TableOpts {
	return TableOpts{
		DbKeyPrefix: 0xFC,
	}
}

// Creates a new symbol.Table and attaches it to the given db.
// If db == nil, the symbol.Table will operate in memory-only mode.
func (opts TableOpts) CreateTable() (symbol.Table, error) {
	st := &symbolTable{
		opts: opts,
	}

	var err error
	if st.opts.Issuer == nil {
		if opts.Db != nil {
			st.opts.Issuer, err = NewBadgerIssuer(opts.Db, opts.DbKeyPrefix)
		} else {
			st.opts.Issuer = NewVolatileIssuer()	
		}
		if err != nil {
			return nil, err
		}
	} else {
		opts.Issuer.AddRef()
		st.dbOwned = true
	}

	st.refCount.Store(1)
	return st, nil
}
