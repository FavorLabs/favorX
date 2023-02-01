package tokens

import (
	"math/big"

	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
)

type Interface interface {
	BalanceOf(account types.AccountID) (*big.Int, error)
}

// exposes methods for querying Traffic
type service struct {
	client *base.SubstrateAPI
	meta   *types.Metadata
}

type AccountInfo struct {
	Free     types.U128
	Reserved types.U128
	Frozen   types.U128
}

func (s *service) BalanceOf(account types.AccountID) (*big.Int, error) {
	currencyId, _ := codec.Encode(types.U64(1)) // todo id
	key, err := types.CreateStorageKey(s.meta, "Tokens", "Accounts", account.ToBytes(), currencyId)
	if err != nil {
		return nil, err
	}
	var accountInfo AccountInfo
	ok, err := s.client.RPC.State.GetStorageLatest(key, &accountInfo)
	if !ok || err != nil {
		return big.NewInt(0), err
	}
	return accountInfo.Free.Int, nil
}

func New(c *base.SubstrateAPI) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &service{
		client: c,
		meta:   meta,
	}
}
