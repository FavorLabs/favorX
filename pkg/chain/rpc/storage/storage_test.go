package storage_test

import (
	"sync"
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/boson/test"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/storage"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/stretchr/testify/assert"
)

const url = "ws://127.0.0.1:9944"

func TestService_StorageFile(t *testing.T) {
	cli, err := chain.NewClient(url, signature.TestKeyringPairAlice)
	assert.NoError(t, err)

	err = cli.Storage.MerchantUnregisterWatch()
	assert.NoError(t, err)

	// register 1T
	err = cli.Storage.MerchantRegisterWatch(1024 * 1024 * 1024 * 1024)
	assert.NoError(t, err)

	err = cli.Storage.MerchantRegisterWatch(1024 * 1024 * 1024 * 1024)
	assert.Error(t, err, storage.MerchantDuplicate)

	wg := &sync.WaitGroup{}
	wg.Add(2)
	cid := test.RandomAddress()
	go func() {
		err = cli.Storage.PlaceOrderWatch(cid.Bytes(), 100, 1, 14400)
		assert.NoError(t, err)
		wg.Done()
	}()

	ci2 := test.RandomAddress()
	go func() {
		err = cli.Storage.PlaceOrderWatch(ci2.Bytes(), 200, 1, 14400)
		assert.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()

	buyer := boson.NewAddress(signature.TestKeyringPairAlice.PublicKey)

	err = cli.Storage.StorageFileWatch(buyer, cid.Bytes())
	assert.NoError(t, err)

	err = cli.Storage.StorageFileWatch(buyer, ci2.Bytes())
	assert.NoError(t, err)

	ov1, err := cli.Storage.GetNodesFromCid(cid.Bytes())
	assert.NoError(t, err)
	assert.Equal(t, []boson.Address{boson.NewAddress(signature.TestKeyringPairAlice.PublicKey)}, ov1)

	ov2, err := cli.Storage.GetNodesFromCid(ci2.Bytes())
	assert.NoError(t, err)
	assert.Equal(t, []boson.Address{boson.NewAddress(signature.TestKeyringPairAlice.PublicKey)}, ov2)
}
