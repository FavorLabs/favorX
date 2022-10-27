package mock_test

import (
	"testing"

	"github.com/FavorLabs/favorX/pkg/statestore/mock"
	"github.com/FavorLabs/favorX/pkg/statestore/test"
	"github.com/FavorLabs/favorX/pkg/storage"
)

func TestMockStateStore(t *testing.T) {
	test.Run(t, func(t *testing.T) storage.StateStorer {
		return mock.NewStateStore()
	})
}
