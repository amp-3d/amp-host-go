package symbol_table

import (
	"github.com/git-amp/amp-sdk-go/stdlib/symbol"
	"github.com/dgraph-io/badger/v4"
)

type TableOpts struct {
	Db          *badger.DB // Db is the badger.DB storing the table's key and value data. .
	DbKeyPrefix byte       // Key prefix for persistent db mode (n/a for memory-only mode)

	symbol.Issuer           // How this table will issue new IDs.  If nil, this table's db will be used as the Issuer
	IssuerInitsAt symbol.ID // The floor ID to start issuing from if initializing a new Issuer. 
}

// DefaultOpts is a suggested set of options.
func DefaultOpts() TableOpts {
	return TableOpts{
		DbKeyPrefix:   0xFC,
		IssuerInitsAt: symbol.DefaultIssuerMin,
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
			st.opts.Issuer, err = NewBadgerIssuer(opts.Db, opts.DbKeyPrefix, opts.IssuerInitsAt)
		} else {
			st.opts.Issuer = symbol.NewVolatileIssuer(opts.IssuerInitsAt)
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
