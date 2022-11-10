package crypto

import (
	"math/big"

	"github.com/ChainSafe/go-schnorrkel"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto/eip712"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	crypto2 "github.com/libp2p/go-libp2p-core/crypto"
)

type SignerConfig struct {
	Signer           Signer
	Overlay          boson.Address
	Libp2pPrivateKey crypto2.PrivKey
	SubKey           signature.KeyringPair
}

type Signer interface {
	GetMnemonic() string
	GetSeed() []byte
	crypto.Keypair
	// EthereumAddress TODO remove old
	// Deprecated
	EthereumAddress() (common.Address, error)
	// SignTx
	// Deprecated
	SignTx(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error)
	// SignTypedData
	// Deprecated
	SignTypedData(typedData *eip712.TypedData) ([]byte, error)
}

type defaultSigner struct {
	crypto.Keypair
	mnemonic string
	seed     []byte
}

func NewKeypairFromSeedHex(in string) (Signer, error) {
	key, err := schnorrkel.NewMiniSecretKeyFromHex(in)
	if err != nil {
		return nil, err
	}
	seed := key.Encode()
	kp, err := sr25519.NewKeypairFromSeed(seed[:])
	if err != nil {
		return nil, err
	}
	return &defaultSigner{
		Keypair: kp,
		seed:    seed[:],
	}, nil
}

func NewKeypairFromMnemonic(mnemonic string) (Signer, error) {
	key, err := schnorrkel.MiniSecretKeyFromMnemonic(mnemonic, "")
	if err != nil {
		return nil, err
	}

	seed := key.Encode()
	kp, err := sr25519.NewKeypairFromSeed(seed[:])
	if err != nil {
		return nil, err
	}
	return &defaultSigner{
		Keypair:  kp,
		mnemonic: mnemonic,
		seed:     seed[:],
	}, nil
}

func NewDefaultSigner(k ...*schnorrkel.MiniSecretKey) Signer {
	var key *schnorrkel.MiniSecretKey
	if len(k) > 0 {
		key = k[0]
	} else {
		key, _ = schnorrkel.GenerateMiniSecretKey()
	}

	seed := key.Encode()
	kp, _ := sr25519.NewKeypairFromSeed(seed[:])
	return &defaultSigner{
		Keypair: kp,
		seed:    seed[:],
	}
}

func (d *defaultSigner) GetMnemonic() string {
	return d.mnemonic
}

func (d *defaultSigner) GetSeed() []byte {
	return d.seed
}

// EthereumAddress
// Deprecated:
func (d *defaultSigner) EthereumAddress() (addr common.Address, err error) {
	return
}

// SignTx
// Deprecated
func (d *defaultSigner) SignTx(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	// TODO implement me
	panic("implement me")
}

// SignTypedData
// Deprecated
func (d *defaultSigner) SignTypedData(typedData *eip712.TypedData) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}
