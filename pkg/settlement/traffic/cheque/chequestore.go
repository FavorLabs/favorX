package cheque

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"math/big"
	"strings"
	"sync"

	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

var (
	// ErrNoCheque is the error returned if there is no prior cheque for a chainAddress or beneficiary.
	ErrNoCheque = errors.New("no cheque")
	// ErrChequeNotIncreasing is the error returned if the cheque amount is the same or lower.
	ErrChequeNotIncreasing = errors.New("cheque cumulativePayout is not increasing")
	// ErrChequeInvalid is the error returned if the cheque itself is invalid.
	ErrChequeInvalid = errors.New("invalid cheque")
	// ErrWrongBeneficiary is the error returned if the cheque has the wrong beneficiary.
	ErrWrongBeneficiary = errors.New("wrong beneficiary")
	// ErrBouncingCheque is the error returned if the chainAddress is demonstrably illiquid.
	ErrBouncingCheque             = errors.New("bouncing cheque")
	ErrCashOut                    = errors.New("cashout not completing")
	lastReceivedChequePrefix      = "traffic_last_received_cheque_"
	lastSendChequePrefix          = "traffic_last_send_cheque_"
	retrievedTrafficPrefix        = "retrieved_traffic_"
	transferredTrafficPrefix      = "transferred_traffic_"
	chainRetrievedTrafficPrefix   = "chain_retrieved_traffic_"
	chainTransferredTrafficPrefix = "chain_transferred_traffic_"
)

// ChequeStore handles the verification and storage of received cheques
type ChequeStore interface {
	// ReceiveCheque verifies and stores a cheque. It returns the totam amount earned.
	ReceiveCheque(ctx context.Context, cheque *SignedCheque) (*big.Int, error)

	VerifyCheque(cheque *SignedCheque) (types.AccountID, error)

	PutSendCheque(ctx context.Context, cheque *Cheque, chainAddress types.AccountID) error

	PutReceivedCheques(chainAddress types.AccountID, cheque SignedCheque) error

	// LastReceivedCheque returns the last cheque we received from a specific chainAddress.
	LastReceivedCheque(chainAddress types.AccountID) (*SignedCheque, error)
	// LastReceivedCheques returns the last received cheques from every known chainAddress.
	LastReceivedCheques() (map[types.AccountID]*SignedCheque, error)

	LastSendCheque(chainAddress types.AccountID) (*Cheque, error)

	LastSendCheques() (map[types.AccountID]*Cheque, error)

	PutRetrieveTraffic(chainAddress types.AccountID, traffic *big.Int) error

	PutTransferTraffic(chainAddress types.AccountID, traffic *big.Int) error

	GetRetrieveTraffic(chainAddress types.AccountID) (traffic *big.Int, err error)

	GetTransferTraffic(chainAddress types.AccountID) (traffic *big.Int, err error)

	PutChainRetrieveTraffic(chainAddress types.AccountID, traffic *big.Int) error

	PutChainTransferTraffic(chainAddress types.AccountID, traffic *big.Int) error

	GetChainRetrieveTraffic(chainAddress types.AccountID) (traffic *big.Int, err error)

	GetChainTransferTraffic(chainAddress types.AccountID) (traffic *big.Int, err error)

	// GetAllRetrieveTransferAddresses return all addresses that we have transferred or retrieved traffic
	GetAllRetrieveTransferAddresses() (map[types.AccountID]struct{}, error)
}

type chequeStore struct {
	lock              sync.Mutex
	store             storage.StateStorer
	recipient         types.AccountID // the beneficiary we expect in cheques sent to us
	recoverChequeFunc RecoverChequeFunc
}

type RecoverChequeFunc func(cheque *SignedCheque) (types.AccountID, error)

// NewChequeStore creates new ChequeStore
func NewChequeStore(
	store storage.StateStorer,
	recipient types.AccountID,
	recoverChequeFunc RecoverChequeFunc) ChequeStore {
	return &chequeStore{
		store:             store,
		recipient:         recipient,
		recoverChequeFunc: recoverChequeFunc,
	}
}

// lastTransferredTrafficChequeKey computes the key where to store the last cheque received from a chainAddress.
func lastReceivedChequeKey(chainAddress types.AccountID) string {
	return fmt.Sprintf("%s%s", lastReceivedChequePrefix, chainAddress.ToHexString())
}

func lastSendChequeKey(chainAddress types.AccountID) string {
	return fmt.Sprintf("%s%s", lastSendChequePrefix, chainAddress.ToHexString())
}

func retrievedTraffic(chainAddress types.AccountID) string {
	return fmt.Sprintf("%s%s", retrievedTrafficPrefix, chainAddress.ToHexString())
}

func transferredTraffic(chainAddress types.AccountID) string {
	return fmt.Sprintf("%s%s", transferredTrafficPrefix, chainAddress.ToHexString())
}

func chainRetrievedTraffic(chainAddress types.AccountID) string {
	return fmt.Sprintf("%s%s", chainRetrievedTrafficPrefix, chainAddress.ToHexString())
}

func chainTransferredTraffic(chainAddress types.AccountID) string {
	return fmt.Sprintf("%s%s", chainTransferredTrafficPrefix, chainAddress.ToHexString())
}

// LastReceivedCheque returns the last cheque we received from a specific chainAddress.
func (s *chequeStore) LastReceivedCheque(chainAddress types.AccountID) (*SignedCheque, error) {
	var cheque *SignedCheque
	err := s.store.Get(lastReceivedChequeKey(chainAddress), &cheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return &SignedCheque{
			Cheque: Cheque{
				Recipient:        types.AccountID{},
				Beneficiary:      types.AccountID{},
				CumulativePayout: big.NewInt(0),
			},
		}, ErrNoCheque
	}

	return cheque, nil
}

// ReceiveCheque verifies and stores a cheque. It returns the totam amount earned.
func (s *chequeStore) ReceiveCheque(ctx context.Context, cheque *SignedCheque) (*big.Int, error) {
	// verify we are the beneficiary
	if cheque.Recipient != s.recipient {
		return nil, ErrWrongBeneficiary
	}

	// verify the cheque signature
	issuer, err := s.recoverChequeFunc(cheque)
	if err != nil {
		return nil, err
	}

	if issuer != cheque.Beneficiary {
		return nil, ErrChequeInvalid
	}

	// don't allow concurrent processing of cheques
	// this would be sufficient on a per chainAddress basis
	s.lock.Lock()
	defer s.lock.Unlock()

	// load the lastCumulativePayout for the cheques chainAddress
	var lastCumulativePayout *big.Int
	var lastReceivedCheque *SignedCheque
	err = s.store.Get(lastReceivedChequeKey(cheque.Beneficiary), &lastReceivedCheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}

		lastCumulativePayout = big.NewInt(0)
	} else {
		lastCumulativePayout = lastReceivedCheque.CumulativePayout
	}

	// check this cheque is actually increasing in value
	amount := big.NewInt(0).Sub(cheque.CumulativePayout, lastCumulativePayout)

	if amount.Cmp(big.NewInt(0)) <= 0 {
		return nil, ErrChequeNotIncreasing
	}

	// store the accepted cheque
	err = s.store.Put(lastReceivedChequeKey(cheque.Beneficiary), cheque)
	if err != nil {
		return nil, err
	}
	return amount, nil
}

func (s *chequeStore) PutSendCheque(ctx context.Context, cheque *Cheque, chainAddress types.AccountID) error {
	return s.store.Put(lastSendChequeKey(chainAddress), cheque)
}

// RecoverCheque recovers the issuer ethereum address from a signed cheque
func RecoverCheque(signedCheque *SignedCheque) (types.AccountID, error) {
	publicly, err := crypto.NewPublicKey(signedCheque.Beneficiary.ToBytes())
	cheque := Cheque{
		Recipient:        signedCheque.Recipient,
		Beneficiary:      signedCheque.Beneficiary,
		CumulativePayout: signedCheque.CumulativePayout,
	}
	c, err := json.Marshal(cheque)
	if err != nil {
		return types.AccountID{}, err
	}
	_, err = publicly.Verify(c, signedCheque.Signature)
	if err != nil {
		return [32]byte{}, err
	}
	var issuer types.AccountID
	copy(issuer[:], signedCheque.Beneficiary[:])
	return issuer, nil
}

// keyChequebook computes the chainAddress a store entry is for.
func keyChainAddress(key []byte, prefix string) (chainAddress *types.AccountID, err error) {
	k := string(key)

	split := strings.SplitAfter(k, prefix)
	if len(split) != 2 {
		return &types.AccountID{}, errors.New("no peer in key")
	}
	return types.NewAccountIDFromHexString(split[1])
}

// LastReceivedCheques returns the last received cheques from every known chainAddress.
func (s *chequeStore) LastReceivedCheques() (map[types.AccountID]*SignedCheque, error) {
	result := make(map[types.AccountID]*SignedCheque)
	err := s.store.Iterate(lastReceivedChequePrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyChainAddress(key, lastReceivedChequePrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := result[*addr]; !ok {
			lastCheque, err := s.LastReceivedCheque(*addr)
			if err != nil && err != ErrNoCheque {
				return false, err
			} else if err == ErrNoCheque {
				return false, nil
			}

			result[*addr] = lastCheque
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *chequeStore) LastSendCheque(chainAddress types.AccountID) (*Cheque, error) {
	var cheque *Cheque
	err := s.store.Get(lastSendChequeKey(chainAddress), &cheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return &Cheque{
			Recipient:        types.AccountID{},
			Beneficiary:      types.AccountID{},
			CumulativePayout: new(big.Int).SetInt64(0),
		}, ErrNoCheque
	}

	return cheque, nil
}

func (s *chequeStore) LastSendCheques() (map[types.AccountID]*Cheque, error) {
	result := make(map[types.AccountID]*Cheque)
	err := s.store.Iterate(lastSendChequePrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyChainAddress(key, lastSendChequePrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := result[*addr]; !ok {
			lastCheque, err := s.LastSendCheque(*addr)
			if err != nil && err != ErrNoCheque {
				return false, err
			} else if err == ErrNoCheque {
				return false, nil
			}

			result[*addr] = lastCheque
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *chequeStore) PutRetrieveTraffic(chainAddress types.AccountID, traffic *big.Int) error {
	return s.store.Put(retrievedTraffic(chainAddress), traffic)
}

func (s *chequeStore) PutTransferTraffic(chainAddress types.AccountID, traffic *big.Int) error {
	return s.store.Put(transferredTraffic(chainAddress), traffic)
}

func (s *chequeStore) PutReceivedCheques(chainAddress types.AccountID, cheque SignedCheque) error {
	return s.store.Put(lastReceivedChequeKey(chainAddress), cheque)
}

func (s *chequeStore) GetRetrieveTraffic(chainAddress types.AccountID) (traffic *big.Int, err error) {
	err = s.store.Get(retrievedTraffic(chainAddress), &traffic)
	if err != nil {
		if err != storage.ErrNotFound {
			return big.NewInt(0), err
		}
		return big.NewInt(0), nil
	}

	return traffic, nil
}

func (s *chequeStore) GetTransferTraffic(chainAddress types.AccountID) (traffic *big.Int, err error) {
	err = s.store.Get(transferredTraffic(chainAddress), &traffic)
	if err != nil {
		if err != storage.ErrNotFound {
			return big.NewInt(0), err
		}
		return big.NewInt(0), nil
	}

	return traffic, nil
}

// RecoverCheque recovers the issuer ethereum address from a signed cheque
func (s *chequeStore) RecoverCheque(signedCheque *SignedCheque) (types.AccountID, error) {
	return RecoverCheque(signedCheque)
}

func (s *chequeStore) VerifyCheque(cheque *SignedCheque) (types.AccountID, error) {
	return s.recoverChequeFunc(cheque)
}

func (s *chequeStore) PutChainRetrieveTraffic(chainAddress types.AccountID, traffic *big.Int) error {
	return s.store.Put(chainRetrievedTraffic(chainAddress), traffic)
}

func (s *chequeStore) PutChainTransferTraffic(chainAddress types.AccountID, traffic *big.Int) error {
	return s.store.Put(chainTransferredTraffic(chainAddress), traffic)
}

func (s *chequeStore) GetChainRetrieveTraffic(chainAddress types.AccountID) (traffic *big.Int, err error) {
	err = s.store.Get(chainRetrievedTraffic(chainAddress), &traffic)
	if err != nil {
		if err != storage.ErrNotFound {
			return big.NewInt(0), err
		}
		return big.NewInt(0), nil
	}

	return traffic, nil
}

func (s *chequeStore) GetChainTransferTraffic(chainAddress types.AccountID) (traffic *big.Int, err error) {
	err = s.store.Get(chainTransferredTraffic(chainAddress), &traffic)
	if err != nil {
		if err != storage.ErrNotFound {
			return big.NewInt(0), err
		}
		return big.NewInt(0), nil
	}

	return traffic, nil
}

func (s *chequeStore) GetAllRetrieveTransferAddresses() (map[types.AccountID]struct{}, error) {
	result := make(map[types.AccountID]struct{})
	err := s.store.Iterate(retrievedTrafficPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyChainAddress(key, retrievedTrafficPrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}
		result[*addr] = struct{}{}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	err = s.store.Iterate(transferredTrafficPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyChainAddress(key, transferredTrafficPrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}
		result[*addr] = struct{}{}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}
