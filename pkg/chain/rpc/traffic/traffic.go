package traffic

import (
	"context"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

type Interface interface {
	// 	TransferredAddress opts todo
	TransferredAddress(address common.Address) ([]common.Address, error)

	RetrievedAddress(address common.Address) ([]common.Address, error)

	BalanceOf(account common.Address) (*big.Int, error)

	RetrievedTotal(address common.Address) (*big.Int, error)

	TransferredTotal(address common.Address) (*big.Int, error)

	TransAmount(beneficiary, recipient common.Address) (*big.Int, error)

	CashChequeBeneficiary(ctx context.Context, peer boson.Address, beneficiary, recipient common.Address, cumulativePayout *big.Int, signature []byte) (*ethTypes.Transaction, error)
}

// exposes methods for querying Traffic
type service struct {
	client *base.SubstrateAPI
	meta   *types.Metadata
}

func (s service) TransferredAddress(address common.Address) ([]common.Address, error) {
	// TODO implement me
	panic("implement me")
}

func (s service) RetrievedAddress(address common.Address) ([]common.Address, error) {
	// TODO implement me
	panic("implement me")
}

func (s service) BalanceOf(account common.Address) (*big.Int, error) {
	// TODO implement me
	panic("implement me")
}

func (s service) RetrievedTotal(address common.Address) (*big.Int, error) {
	// TODO implement me
	panic("implement me")
}

func (s service) TransferredTotal(address common.Address) (*big.Int, error) {
	// TODO implement me
	panic("implement me")
}

func (s service) TransAmount(beneficiary, recipient common.Address) (*big.Int, error) {
	// TODO implement me
	panic("implement me")
}

func (s service) CashChequeBeneficiary(ctx context.Context, peer boson.Address, beneficiary, recipient common.Address, cumulativePayout *big.Int, signature []byte) (*ethTypes.Transaction, error) {
	// TODO implement me
	panic("implement me")
}

// New creates a new Traffic struct
func New(c *base.SubstrateAPI) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &service{
		client: c,
		meta:   meta,
	}
}
