package traffic

import (
	"context"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type Interface interface {
	TransferredAddress(address types.AccountID) ([]types.AccountID, error)

	BalanceOf(account types.AccountID) (*big.Int, error)

	TransferredTotal(address types.AccountID) (*big.Int, error)

	TransAmount(beneficiary, recipient types.AccountID) (*big.Int, error)

	CashChequeBeneficiary(cid, peer boson.Address) (types.Hash, error)
}

// exposes methods for querying Traffic
type service struct {
	client *base.SubstrateAPI
	meta   *types.Metadata
}

func (s *service) TransferredAddress(address types.AccountID) (accountIds []types.AccountID, err error) {
	key, err := types.CreateStorageKey(s.meta, "Traffic", "ListMap", address.ToBytes())
	if err != nil {
		return
	}
	_, err = s.client.RPC.State.GetStorageLatest(key, &accountIds)
	return
}

func (s *service) BalanceOf(account types.AccountID) (*big.Int, error) {
	// TODO implement me AccountStore
	key, err := types.CreateStorageKey(s.meta, "System", "Account", account.ToBytes())
	if err != nil {
		return nil, err
	}
	var accountInfo types.AccountInfo
	ok, err := s.client.RPC.State.GetStorageLatest(key, &accountInfo)
	if !ok || err != nil {
		return big.NewInt(0), err
	}
	return accountInfo.Data.Free.Int, nil
}

func (s *service) TransferredTotal(address types.AccountID) (*big.Int, error) {
	// TODO implement me
	return big.NewInt(0), nil
}

func (s *service) TransAmount(beneficiary, recipient types.AccountID) (*big.Int, error) {
	key, err := types.CreateStorageKey(s.meta, "Traffic", "Cheques", recipient.ToBytes(), beneficiary.ToBytes())
	if err != nil {
		return nil, err
	}
	var t types.U128
	ok, err := s.client.RPC.State.GetStorageLatest(key, &t)
	if !ok || err != nil {
		return big.NewInt(0), err
	}
	return t.Int, nil
}

func (s *service) CashChequeBeneficiary(cid, peer boson.Address) (hash types.Hash, err error) {

	c, err := types.NewCall(s.meta, "Traffic.cash_out", cid.String(), peer.String())
	if err != nil {
		return
	}
	err = s.client.SubmitExtrinsicAndWatch(context.TODO(), c, func(block types.Hash, txn types.Hash) (has bool, err error) {
		hash = block
		return true, nil
	})
	return
}

// New creates a new Traffic struct
func New(c *base.SubstrateAPI) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &service{
		client: c,
		meta:   meta,
	}
}
