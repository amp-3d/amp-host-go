package symbol_table_test

import (
	"os"
	"path"
	"testing"

	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/arcspace/go-arc-sdk/stdlib/symbol/tests"
	"github.com/arcspace/go-archost/arc/badger/symbol_table"
	"github.com/dgraph-io/badger/v4"
)

func Test_badger_table(t *testing.T) {
	dir, err := os.MkdirTemp("", "junk*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Test WITH a database
	dbPathname := path.Join(dir, "test1")
	opts := badger.DefaultOptions(dbPathname)
	opts.Logger = nil

	open_Table := func() (symbol.Table, error) {
		db, err := badger.Open(opts)
		if err != nil {
			return nil, err
		}

		opts := symbol_table.DefaultOpts()
		opts.Db = db
		table, err := opts.CreateTable()
		if err != nil {
			return nil, err
		}
		return table, nil
	}

	tests.DoTableTest(t, 0, open_Table)
}
