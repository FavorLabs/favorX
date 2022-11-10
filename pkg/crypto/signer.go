package crypto

import (
	"crypto/sha512"
	"errors"
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
	GetExportPrivateKey() [64]byte
	GetSecretKey64() []byte
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
	mnemonic  string
	seed      []byte
	scryptKey [64]byte
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
		Keypair:   kp,
		seed:      seed[:],
		scryptKey: convertKey(seed),
	}, nil
}

func convertKey(seed [32]byte) (scryptKey [64]byte) {
	h := sha512.Sum512(seed[:])
	var key, nonce [32]byte
	copy(key[:], h[:32])
	copy(nonce[:], h[32:])
	key[0] &= 248
	key[31] &= 63
	key[31] |= 64
	copy(scryptKey[:32], key[:])
	copy(scryptKey[32:], nonce[:])
	return
}

// https://github.com/w3f/schnorrkel/blob/718678e51006d84c7d8e4b6cde758906172e74f8/src/scalars.rs#L18
func divideScalarByCofactor(s []byte) []byte {
	l := len(s) - 1
	low := byte(0)
	for i := range s {
		r := s[l-i] & 0x07 // remainder
		s[l-i] >>= 3
		s[l-i] += low
		low = r << 5
	}

	return s
}

func NewKeypairFromExportPrivateKey(sk []byte) (Signer, error) {
	var scryptKey [64]byte
	copy(scryptKey[:], sk[:])

	if len(sk) != 64 {
		return nil, errors.New("invalid scrypt key")
	}
	var key, nonce [32]byte
	copy(nonce[:], sk[32:])

	tmp := sk[:32]

	copy(key[:], divideScalarByCofactor(tmp[:]))

	scrKey := schnorrkel.NewSecretKey(key, nonce)
	keypair, err := sr25519.NewKeypair(scrKey)
	if err != nil {
		return nil, err
	}

	return &defaultSigner{
		Keypair:   keypair,
		scryptKey: scryptKey,
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
		Keypair:   kp,
		mnemonic:  mnemonic,
		seed:      seed[:],
		scryptKey: convertKey(seed),
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
		Keypair:   kp,
		seed:      seed[:],
		scryptKey: convertKey(seed),
	}
}

func (d *defaultSigner) GetSecretKey64() []byte {
	var tmp [32]byte
	copy(tmp[:], d.scryptKey[:32])
	key := divideScalarByCofactor(tmp[:])

	nonce := d.scryptKey[32:]

	key = append(key, nonce...)
	return key
}

func (d *defaultSigner) GetExportPrivateKey() [64]byte {
	return d.scryptKey
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
