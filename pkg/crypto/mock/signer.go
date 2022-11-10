package mock

import (
	"crypto/ecdsa"
	"math/big"

	crypto2 "github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/crypto/eip712"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type signerMock struct {
	signTx          func(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error)
	signTypedData   func(*eip712.TypedData) ([]byte, error)
	ethereumAddress func() (common.Address, error)
}

func (m *signerMock) GetMnemonic() string {
	// TODO implement me
	panic("implement me")
}

func (m *signerMock) GetSeed() []byte {
	// TODO implement me
	panic("implement me")
}

func (m *signerMock) Type() crypto2.KeyType {
	// TODO implement me
	panic("implement me")
}

func (m *signerMock) Public() crypto2.PublicKey {
	// TODO implement me
	panic("implement me")
}

func (m *signerMock) Private() crypto2.PrivateKey {
	// TODO implement me
	panic("implement me")
}

func (m *signerMock) EthereumAddress() (common.Address, error) {
	if m.ethereumAddress != nil {
		return m.ethereumAddress()
	}
	return common.Address{}, nil
}

func (*signerMock) Sign(data []byte) ([]byte, error) {
	return nil, nil
}

func (m *signerMock) SignTx(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	return m.signTx(transaction, chainID)
}

func (*signerMock) PublicKey() (*ecdsa.PublicKey, error) {
	return nil, nil
}

func (m *signerMock) SignTypedData(d *eip712.TypedData) ([]byte, error) {
	return m.signTypedData(d)
}

func New(opts ...Option) crypto.Signer {
	mock := new(signerMock)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// Option is the option passed to the mock Chequebook service
type Option interface {
	apply(*signerMock)
}

type optionFunc func(*signerMock)

func (f optionFunc) apply(r *signerMock) { f(r) }

func WithSignTxFunc(f func(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error)) Option {
	return optionFunc(func(s *signerMock) {
		s.signTx = f
	})
}

func WithSignTypedDataFunc(f func(*eip712.TypedData) ([]byte, error)) Option {
	return optionFunc(func(s *signerMock) {
		s.signTypedData = f
	})
}

func WithEthereumAddressFunc(f func() (common.Address, error)) Option {
	return optionFunc(func(s *signerMock) {
		s.ethereumAddress = f
	})
}
