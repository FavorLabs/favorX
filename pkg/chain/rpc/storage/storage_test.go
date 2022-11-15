package storage_test

import (
	"context"
	"sync"
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/storage"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/stretchr/testify/assert"
)

const url = "ws://127.0.0.1:9944"

var ctx = context.Background()

func TestService_StorageFile(t *testing.T) {
	// t.SkipNow()
	cli, err := chain.NewClient(url, signature.TestKeyringPairAlice)
	assert.NoError(t, err)
	cid := boson.MustParseHexAddress("471d6a05e523183eb9dd57bee7aba29b9f5798e834f60ac69022b41f0ff69948")
	ci2 := boson.MustParseHexAddress("571d6a05e523183eb9dd57bee7aba29b9f5798e834f60ac69022b41f0ff69948")

	err = cli.Storage.MerchantUnregisterWatch(ctx)
	assert.NoError(t, err)

	// register 1T
	err = cli.Storage.MerchantRegisterWatch(ctx, 1024*1024*1024*1024)
	assert.NoError(t, err)

	err = cli.Storage.MerchantRegisterWatch(ctx, 1024*1024*1024*1024)
	assert.Error(t, err, storage.MerchantDuplicate)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		_, err = cli.Storage.PlaceOrderWatch(ctx, cid.Bytes(), 100, 1, 14400)
		assert.NoError(t, err)
		wg.Done()
	}()

	go func() {
		_, err = cli.Storage.PlaceOrderWatch(ctx, ci2.Bytes(), 200, 1, 14400)
		assert.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()

	buyer := signature.TestKeyringPairAlice.PublicKey

	err = cli.Storage.StorageFileWatch(ctx, buyer, cid.Bytes(), cid.Bytes())
	assert.NoError(t, err)

	err = cli.Storage.StorageFileWatch(ctx, buyer, ci2.Bytes(), ci2.Bytes())
	assert.NoError(t, err)

	ov1, _ := types.NewAccountID(cid.Bytes())

	ovs, err := cli.Storage.GetNodesFromCid(cid.Bytes())
	assert.NoError(t, err)
	assert.Equal(t, []types.AccountID{*ov1}, ovs)

	ov2, _ := types.NewAccountID(ci2.Bytes())
	ovs2, err := cli.Storage.GetNodesFromCid(ci2.Bytes())
	assert.NoError(t, err)
	assert.Equal(t, []types.AccountID{*ov2}, ovs2)
}

func Test_CheckOrder(t *testing.T) {
	t.SkipNow()
	cli, err := chain.NewClient(url, signature.TestKeyringPairAlice)
	assert.NoError(t, err)
	buyer, err := crypto.NewPublicKeyFromSs58("5G1FkjcvB1ct2dk7S8k4nUGjRooETh71UFyJiVFeyZk4tHvL")
	assert.NoError(t, err)
	mch, err := crypto.NewPublicKeyFromSs58("5EAPiyvna7EeeFXPEjvcHyKtqb7esErxCypRkcWTraWvm7ho")
	assert.NoError(t, err)
	fileHash := boson.MustParseHexAddress("3db47e7511eb12b3932a18e41fd4f8b7bd2cf2287c26ce328cd8cd1305f98117")
	err = cli.Storage.CheckOrder(buyer.Encode(), fileHash.Bytes(), mch.Encode())
	assert.NoError(t, err)
}
