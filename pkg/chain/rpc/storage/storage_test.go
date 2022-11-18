package storage_test

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/storage"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
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

	err = cli.Storage.StorageFileWatch(ctx, buyer, cid.Bytes())
	assert.NoError(t, err)

	err = cli.Storage.StorageFileWatch(ctx, buyer, ci2.Bytes())
	assert.NoError(t, err)

	ov1, _ := types.NewAccountID(signature.TestKeyringPairAlice.PublicKey)

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
	fileHash := boson.MustParseHexAddress("2b08759c571c60e372070b2651e03ae11937a2edce2cecdbb3afbf3633953f88")
	err = cli.Storage.CheckOrder(buyer.Encode(), fileHash.Bytes(), mch.Encode())
	assert.NoError(t, err)
}

func Test_BlockNumber(t *testing.T) {
	str := "0x9a0700002b08759c571c60e372070b2651e03ae11937a2edce2cecdbb3afbf3633953f8890e70100000000000100000000000000da3f0000045ccee65a262c90b760b38ee7286eb38b51a8ba3af217c1f622fc787c0375924d0000000000000000"
	bz, err := codec.HexDecodeString(str)
	if err != nil {
		t.Fatal(err)
	}
	// ------
	file, err := types.NewAccountIDFromHexString("0x2b08759c571c60e372070b2651e03ae11937a2edce2cecdbb3afbf3633953f88")
	if err != nil {
		t.Fatal(err)
	}
	mch, err := types.NewAccountIDFromHexString("0x5ccee65a262c90b760b38ee7286eb38b51a8ba3af217c1f622fc787c0375924d")
	if err != nil {
		t.Fatal(err)
	}
	en := storage.OrderInfo{
		CreateAt:  1946,
		FileHash:  *file,
		FileSize:  124816,
		FileCopy:  1,
		ExpiredAt: 16346,
		Merchants: []types.AccountID{*mch},
		Price:     0,
	}
	bts, err := codec.Encode(en)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bts, bz) {
		t.Fatalf("mismatch encoded")
	}
	// -------
	var res1 storage.OrderInfo

	err = codec.Decode(bz, &res1)
	if err != nil {
		t.Fatal(err)
	}
	if res1.CreateAt != en.CreateAt {
		t.Fatalf("mismatch createAt")
	}
	if res1.ExpiredAt != en.ExpiredAt {
		t.Fatalf("mismatch ExpiredAt")
	}
}
