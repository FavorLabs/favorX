package file_test

import (
	"os"
	"testing"

	"github.com/FavorLabs/favorX/pkg/keystore/file"
	"github.com/FavorLabs/favorX/pkg/keystore/test"
)

func TestService(t *testing.T) {
	dir, err := os.MkdirTemp("", "node-keystore-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	test.Service(t, file.New(dir))
}
