package cheque

import (
	"bytes"
	"fmt"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// Cheque represents a cheque for a SimpleSwap chequebook
type Cheque struct {
	Recipient        types.AccountID `json:"recipient"`
	Beneficiary      types.AccountID `json:"beneficiary"`
	CumulativePayout *big.Int        `json:"cumulative_payout"`
}

type EncodeCheque struct {
	Recipient        types.AccountID
	Beneficiary      types.AccountID
	CumulativePayout types.U128
}

// SignedCheque represents a cheque together with its signature
type SignedCheque struct {
	Cheque
	Signature []byte `json:"signature"`
}

type ChainSignedCheque struct {
	Recipient        types.AccountID
	Beneficiary      types.AccountID
	CumulativePayout types.U128
	SignedCheque     types.Signature
}

// ChequeSigner signs cheque
type ChequeSigner interface {
	// Sign signs a cheque
	Sign(cheque *Cheque) ([]byte, error)
}

type chequeSigner struct {
	signer crypto.Signer // the underlying signer used
}

// NewChequeSigner creates a new cheque signer for the given chainID.
func NewChequeSigner(signer crypto.Signer) ChequeSigner {
	return &chequeSigner{
		signer: signer,
	}
}

// Sign signs a cheque.
func (s *chequeSigner) Sign(cheque *Cheque) ([]byte, error) {
	ec := &EncodeCheque{Recipient: cheque.Recipient,
		Beneficiary:      cheque.Beneficiary,
		CumulativePayout: types.NewU128(*cheque.CumulativePayout)}
	sign, err := codec.Encode(ec)
	if err != nil {
		return nil, err
	}
	return s.signer.Sign(sign)
}

func (cheque *Cheque) String() string {
	return fmt.Sprintf(" Beneficiary: %x CumulativePayout: %v", cheque.Beneficiary, cheque.CumulativePayout)
}

func (cheque *Cheque) Equal(other *Cheque) bool {
	if cheque.Beneficiary != other.Beneficiary {
		return false
	}
	if cheque.CumulativePayout.Cmp(other.CumulativePayout) != 0 {
		return false
	}
	return true
}

func (cheque *SignedCheque) Equal(other *SignedCheque) bool {
	if !bytes.Equal(cheque.Signature, other.Signature) {
		return false
	}
	return cheque.Cheque.Equal(&other.Cheque)
}
