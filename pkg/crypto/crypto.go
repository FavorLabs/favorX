package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto/eip712"
	"github.com/btcsuite/btcd/btcec"
	"golang.org/x/crypto/sha3"
)

type PublicKey crypto.PublicKey
type PrivateKey crypto.PrivateKey
type Keypair crypto.Keypair

func NewOverlayAddress(pub []byte, networkID uint64) (boson.Address, error) {
	overlay := sha3.Sum256(pub)
	return boson.NewAddress(overlay[:]), nil
}

func NewPublicKey(in []byte) (crypto.PublicKey, error) {
	return sr25519.NewPublicKey(in)
}

func NewPublicKeyFromPubHex(h string) (crypto.PublicKey, error) {
	in, err := common.HexToBytes(h)
	if err != nil {
		return nil, err
	}
	return sr25519.NewPublicKey(in)
}

func NewPublicKeyFromSs58(b58 string) (crypto.PublicKey, error) {
	in := crypto.PublicAddressToByteArray(common.Address(b58))
	return sr25519.NewPublicKey(in)
}

func HexToBytes(in string) ([]byte, error) {
	return common.HexToBytes(in)
}

func BytesToHex(in []byte) string {
	return common.BytesToHex(in)
}

func NewBIP39Mnemonic() (string, error) {
	return crypto.NewBIP39Mnemonic()
}

// RecoverEIP712 recovers the public key for eip712 signed data.
// Deprecated
func RecoverEIP712(signature []byte, data *eip712.TypedData) (*ecdsa.PublicKey, error) {
	if len(signature) != 65 {
		return nil, errors.New("invalid length")
	}
	// Convert to btcec input format with 'recovery id' v at the beginning.
	btcsig := make([]byte, 65)
	btcsig[0] = signature[64]
	copy(btcsig[1:], signature)

	rawData, err := eip712.EncodeForSigning(data)
	if err != nil {
		return nil, err
	}

	sighash, err := LegacyKeccak256(rawData)
	if err != nil {
		return nil, err
	}

	p, _, err := btcec.RecoverCompact(btcec.S256(), btcsig, sighash)
	return (*ecdsa.PublicKey)(p), err
}

// LegacyKeccak256
// Deprecated
func LegacyKeccak256(data []byte) ([]byte, error) {
	var err error
	hasher := sha3.NewLegacyKeccak256()
	_, err = hasher.Write(data)
	if err != nil {
		return nil, err
	}
	return hasher.Sum(nil), err
}

// NewEthereumAddress
// Deprecated
func NewEthereumAddress(p ecdsa.PublicKey) ([]byte, error) {
	if p.X == nil || p.Y == nil {
		return nil, errors.New("invalid public key")
	}
	pubBytes := elliptic.Marshal(btcec.S256(), p.X, p.Y)
	pubHash, err := LegacyKeccak256(pubBytes[1:])
	if err != nil {
		return nil, err
	}
	return pubHash[12:], err
}
