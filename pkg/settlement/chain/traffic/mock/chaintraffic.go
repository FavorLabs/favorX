package mock

import (
	"context"
	"errors"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/settlement/chain"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type ChainTrafficMock struct {
	transferredAddress func(address types.AccountID) ([]types.AccountID, error)

	retrievedAddress func(address types.AccountID) ([]types.AccountID, error)

	balanceOf func(account types.AccountID) (*big.Int, error)

	retrievedTotal func(address types.AccountID) (*big.Int, error)

	transferredTotal func(address types.AccountID) (*big.Int, error)

	transAmount func(beneficiary, recipient types.AccountID) (*big.Int, error)

	cashChequeBeneficiary func(cid, addr boson.Address) (*types.Hash, error)
}

func (m *ChainTrafficMock) TransferredAddress(address types.AccountID) ([]types.AccountID, error) {
	if m.transferredAddress != nil {
		return m.transferredAddress(address)
	}
	return []types.AccountID{}, errors.New("not implemented")
}

func (m *ChainTrafficMock) RetrievedAddress(address types.AccountID) ([]types.AccountID, error) {
	if m.retrievedAddress != nil {
		return m.retrievedAddress(address)
	}
	return []types.AccountID{}, errors.New("not implemented")
}

func (m *ChainTrafficMock) BalanceOf(address types.AccountID) (*big.Int, error) {
	if m.balanceOf != nil {
		return m.balanceOf(address)
	}
	return big.NewInt(0), errors.New("not implemented")
}

func (m *ChainTrafficMock) RetrievedTotal(address types.AccountID) (*big.Int, error) {
	if m.retrievedTotal != nil {
		return m.retrievedTotal(address)
	}
	return big.NewInt(0), errors.New("not implemented")
}

func (m *ChainTrafficMock) TransferredTotal(address types.AccountID) (*big.Int, error) {
	if m.transferredTotal != nil {
		return m.transferredTotal(address)
	}
	return big.NewInt(0), errors.New("not implemented")
}

func (m *ChainTrafficMock) TransAmount(beneficiary, recipient types.AccountID) (*big.Int, error) {
	if m.transAmount != nil {
		return m.transAmount(beneficiary, recipient)
	}
	return big.NewInt(0), errors.New("not implemented")
}

func (m *ChainTrafficMock) CashChequeBeneficiary(ctx context.Context, peer boson.Address, beneficiary, recipient types.AccountID, cumulativePayout *big.Int, signature []byte) (*types.Transaction, error) {
	if m.cashChequeBeneficiary != nil {
		return m.cashChequeBeneficiary(ctx, peer, beneficiary, recipient, cumulativePayout, signature)
	}
	return nil, errors.New("not implemented")
}

func New(opts ...Option) chain.Traffic {
	mock := new(ChainTrafficMock)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// Option is the option passed to the mock Chequebook service
type Option interface {
	apply(*ChainTrafficMock)
}

func WithTransferredAddress(f func(address types.AccountID) ([]types.AccountID, error)) Option {
	return optionFunc(func(s *ChainTrafficMock) {
		s.transferredAddress = f
	})
}

func WithRetrievedAddress(f func(address types.AccountID) ([]types.AccountID, error)) Option {
	return optionFunc(func(s *ChainTrafficMock) {
		s.retrievedAddress = f
	})
}

func WithBalanceOf(f func(account types.AccountID) (*big.Int, error)) Option {
	return optionFunc(func(s *ChainTrafficMock) {
		s.balanceOf = f
	})
}

func WithRetrievedTotal(f func(address types.AccountID) (*big.Int, error)) Option {
	return optionFunc(func(s *ChainTrafficMock) {
		s.retrievedTotal = f
	})
}

func WithTransferredTotal(f func(address types.AccountID) (*big.Int, error)) Option {
	return optionFunc(func(s *ChainTrafficMock) {
		s.transferredTotal = f
	})
}

func WithTransAmount(f func(beneficiary, recipient types.AccountID) (*big.Int, error)) Option {
	return optionFunc(func(s *ChainTrafficMock) {
		s.transAmount = f
	})
}

func WithCashChequeBeneficiary(f func(ctx context.Context, peer boson.Address, beneficiary types.AccountID, recipient types.AccountID, cumulativePayout *big.Int, signature []byte) (*types.Transaction, error)) Option {
	return optionFunc(func(s *ChainTrafficMock) {
		s.cashChequeBeneficiary = f
	})
}

type optionFunc func(*ChainTrafficMock)

func (f optionFunc) apply(r *ChainTrafficMock) { f(r) }
