package leveldb_test

import (
	"os"
	"testing"

	"github.com/FavorLabs/favorX/pkg/statestore/leveldb"
	"github.com/FavorLabs/favorX/pkg/statestore/test"
	"github.com/FavorLabs/favorX/pkg/storage"
)

func TestPersistentStateStore(t *testing.T) {
	test.Run(t, func(t *testing.T) storage.StateStorer {
		dir, err := os.MkdirTemp("", "statestore_test")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
		})

		store, err := leveldb.NewStateStore(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
		})

		return store
	})

	test.RunPersist(t, func(t *testing.T, dir string) storage.StateStorer {
		store, err := leveldb.NewStateStore(dir, nil)
		if err != nil {
			t.Fatal(err)
		}

		return store
	})
}

func TestGetSchemaName(t *testing.T) {
	dir, err := os.MkdirTemp("", "statestore_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	})

	store, err := leveldb.NewStateStore(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	})
	sn := store.(interface {
		GetSchemaName() (string, error)
	})
	n, err := sn.GetSchemaName() // expect current
	if err != nil {
		t.Fatal(err)
	}
	if n != leveldb.DbSchemaCurrent {
		t.Fatalf("wanted current db schema but got '%s'", n)
	}
}
