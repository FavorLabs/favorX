// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package traffic

import (
	"fmt"
	"strings"
	"sync"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

var (
	beneficiaryPeerPrefix = "boson_beneficiary_peer_"
	peerBeneficiaryPrefix = "boson_peer_beneficiary_"
)

// Addressbook maps peers to beneficaries, chequebooks and in reverse.
type Addressbook interface {
	// Beneficiary returns the beneficiary for the given peer.
	Beneficiary(peer boson.Address) (beneficiary types.AccountID, known bool)
	// BeneficiaryPeer returns the peer for a beneficiary.
	BeneficiaryPeer(beneficiary types.AccountID) (peer boson.Address, known bool)
	// PutBeneficiary stores the beneficiary for the given peer.
	PutBeneficiary(peer boson.Address, beneficiary types.AccountID) error

	InitAddressBook() error
}

type addressBook struct {
	store storage.StateStorer
	//peer:beneficiary
	peerBeneficiary sync.Map
	//beneficiary:peer
	beneficiaryPeer sync.Map
}

// NewAddressbook creates a new addressbook using the store.
func NewAddressBook(store storage.StateStorer) Addressbook {
	return &addressBook{
		store: store,
	}
}

func (a *addressBook) InitAddressBook() error {
	if err := a.store.Iterate(peerBeneficiaryPrefix, func(k, value []byte) (stop bool, err error) {
		if !strings.HasPrefix(string(k), peerBeneficiaryPrefix) {
			return true, nil
		}

		key := string(k)
		peer, err := peerUnmarshalKey(peerBeneficiaryPrefix, key)
		if err != nil {
			return false, err
		}

		var beneficiary types.AccountID
		if err = a.store.Get(key, &beneficiary); err != nil {
			return true, err
		}

		if err := a.putPeerBeneficiary(peer, beneficiary); err != nil {
			return true, err
		}

		return false, nil
	}); err != nil {
		return err
	}

	if err := a.store.Iterate(beneficiaryPeerPrefix, func(k, value []byte) (stop bool, err error) {
		if !strings.HasPrefix(string(k), beneficiaryPeerPrefix) {
			return true, nil
		}

		key := string(k)
		beneficiary, err := beneficiaryUnmarshalKey(beneficiaryPeerPrefix, key)
		if err != nil {
			return false, err
		}

		var peer boson.Address
		if err = a.store.Get(key, &peer); err != nil {
			return true, err
		}

		if err := a.putBeneficiaryPeer(peer, *beneficiary); err != nil {
			return true, err
		}

		return false, nil
	}); err != nil {
		return err
	}

	return nil
}
func (a *addressBook) putPeerBeneficiary(peer boson.Address, beneficiary types.AccountID) error {
	a.peerBeneficiary.Store(peer.String(), beneficiary)
	return nil
}

func (a *addressBook) putBeneficiaryPeer(peer boson.Address, beneficiary types.AccountID) error {
	a.beneficiaryPeer.Store(beneficiary.ToHexString(), peer)
	return nil
}

// Beneficiary returns the beneficiary for the given peer.
func (a *addressBook) Beneficiary(peer boson.Address) (beneficiary types.AccountID, known bool) {
	if value, ok := a.peerBeneficiary.Load(peer.String()); ok {
		return value.(types.AccountID), true
	} else {
		return types.AccountID{}, false
	}
}

// BeneficiaryPeer returns the peer for a beneficiary.
func (a *addressBook) BeneficiaryPeer(beneficiary types.AccountID) (peer boson.Address, known bool) {
	if value, ok := a.beneficiaryPeer.Load(beneficiary.ToHexString()); ok {
		return value.(boson.Address), true
	} else {
		return boson.Address{}, false
	}
}

// PutBeneficiary stores the beneficiary for the given peer.
func (a *addressBook) PutBeneficiary(peer boson.Address, beneficiary types.AccountID) error {
	err := a.putPeerBeneficiary(peer, beneficiary)
	if err != nil {
		return err
	}
	err = a.putBeneficiaryPeer(peer, beneficiary)
	if err != nil {
		return err
	}

	err = a.store.Put(peerBeneficiaryKey(peer), beneficiary)
	if err != nil {
		return err
	}
	return a.store.Put(beneficiaryPeerKey(beneficiary), peer)
}

// peerBeneficiaryKey computes the key where to store the beneficiary for a peer.
func peerBeneficiaryKey(peer boson.Address) string {
	return fmt.Sprintf("%s-%s", peerBeneficiaryPrefix, peer.String())
}

// beneficiaryPeerKey computes the key where to store the peer for a beneficiary.
func beneficiaryPeerKey(peer types.AccountID) string {
	return fmt.Sprintf("%s-%s", beneficiaryPeerPrefix, peer.ToHexString())
}

func peerUnmarshalKey(keyPrefix, key string) (boson.Address, error) {
	addr := strings.TrimPrefix(key, keyPrefix)
	keys := strings.Split(addr, "-")

	overlay, err := boson.ParseHexAddress(keys[1])
	if err != nil {
		return boson.ZeroAddress, err
	}
	return overlay, nil
}

func beneficiaryUnmarshalKey(keyPrefix, key string) (*types.AccountID, error) {
	addr := strings.TrimPrefix(key, keyPrefix)
	keys := strings.Split(addr, "-")

	if len(keys) > 1 {
		return types.NewAccountIDFromHexString(keys[1])
	} else {
		return types.NewAccountID([]byte(""))
	}
}
