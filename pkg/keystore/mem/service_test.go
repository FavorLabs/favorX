package mem_test

import (
	"testing"

	"github.com/FavorLabs/favorX/pkg/keystore/mem"
	"github.com/FavorLabs/favorX/pkg/keystore/test"
)

func TestService(t *testing.T) {
	test.Service(t, mem.New())
}
