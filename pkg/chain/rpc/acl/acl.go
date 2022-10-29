package acl

import (
	"bytes"
	"errors"
	"strings"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/rpc/author"
	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
)

type Interface interface {
	SetNicknameWatch(name string) error
	SelfNickname() (name string, err error)
	GetNickName(accountId string) (name string, err error)
	GetAccountID(nickname string) (accountID string, err error)
	SetResolveWatch(uri string, cid []byte) error
	GetResolve(uri string) (cid []byte, err error)
	GetNodesFromCid(cid []byte) (overlays []boson.Address, err error)
}

// oracle exposes methods for querying oracle
type acl struct {
	client *base.SubstrateAPI
	meta   *types.Metadata
	signer signature.KeyringPair
}

// New creates a new oracle struct
func New(c *base.SubstrateAPI, signer signature.KeyringPair) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &acl{
		client: c,
		meta:   meta,
		signer: signer,
	}
}

func (s *acl) GetNodesFromCid(cid []byte) (overlays []boson.Address, err error) {
	key, err := types.CreateStorageKey(s.meta, "Acl", "Oracles", s.signer.PublicKey, cid)
	if err != nil {
		return
	}
	ok, err := s.client.RPC.State.GetStorageLatest(key, &overlays)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = errors.New("Acl.Oracles is empty")
		return
	}
	return
}

func (s *acl) SetNicknameWatch(name string) error {
	c, err := types.NewCall(s.meta, "Acl.set_nickname", name)
	if err != nil {
		return err
	}

	var sub *author.ExtrinsicStatusSubscription
	for {
		// Create the extrinsic
		ext := types.NewExtrinsic(c)
		genesisHash, err := s.client.RPC.Chain.GetBlockHash(0)
		if err != nil {
			return err
		}

		rv, err := s.client.RPC.State.GetRuntimeVersionLatest()
		if err != nil {
			return err
		}

		// Get the nonce for Alice
		key, err := types.CreateStorageKey(s.meta, "System", "Account", s.signer.PublicKey)
		if err != nil {
			return err
		}

		var accountInfo types.AccountInfo
		ok, err := s.client.RPC.State.GetStorageLatest(key, &accountInfo)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("account not found")
		}
		nonce := uint32(accountInfo.Nonce)
		o := types.SignatureOptions{
			BlockHash:          genesisHash,
			Era:                types.ExtrinsicEra{IsMortalEra: false},
			GenesisHash:        genesisHash,
			Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
			SpecVersion:        rv.SpecVersion,
			Tip:                types.NewUCompactFromUInt(0),
			TransactionVersion: rv.TransactionVersion,
		}

		logging.Infof("set nickname %s\n", name)

		// Sign the transaction using Alice's default account
		err = ext.Sign(s.signer, o)
		if err != nil {
			return err
		}

		// Do the transfer and track the actual status
		sub, err = s.client.RPC.Author.SubmitAndWatchExtrinsic(ext)
		if err != nil {
			logging.Warningf("set_nickname submit failed: %v", err)
			continue
		}
		break
	}
	defer sub.Unsubscribe()
	for {
		status := <-sub.Chan()

		// wait until finalisation
		if status.IsInBlock || status.IsFinalized {
			break
		}

		logging.Infof("waiting for the set_nickname to be included/finalized")
	}
	return nil
}

func (s *acl) GetNickName(accountId string) (name string, err error) {
	key, err := types.CreateStorageKey(s.meta, "Acl", "AccountNickname", codec.MustHexDecodeString(accountId))
	if err != nil {
		return
	}
	ok, err := s.client.RPC.State.GetStorageLatest(key, &name)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = errors.New("Acl.AccountNickname is empty")
		return
	}
	return
}

func (s *acl) GetAccountID(nickname string) (accountID string, err error) {
	// SCALE encode nickname bytes
	buf := bytes.NewBuffer(nil)
	enc := scale.NewEncoder(buf)
	if err = enc.Encode(nickname); err != nil {
		return
	}

	key, err := types.CreateStorageKey(s.meta, "Acl", "NicknameAccount", buf.Bytes())
	if err != nil {
		return
	}
	res := &types.AccountID{}
	ok, err := s.client.RPC.State.GetStorageLatest(key, &res)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = errors.New("Acl.AccountNickname is empty")
		return
	}
	return res.ToHexString(), nil
}

func (s *acl) SelfNickname() (name string, err error) {
	key, err := types.CreateStorageKey(s.meta, "Acl", "AccountNickname", s.signer.PublicKey)
	if err != nil {
		return
	}
	ok, err := s.client.RPC.State.GetStorageLatest(key, &name)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = errors.New("Acl.AccountNickname is empty")
		return
	}
	return
}

func (s *acl) SetResolveWatch(uri string, cid []byte) error {
	accountID, err := types.NewAccountID(cid)
	if err != nil {
		return err
	}
	c, err := types.NewCall(s.meta, "Acl.set_resolve", uri, accountID)
	if err != nil {
		return err
	}
	var sub *author.ExtrinsicStatusSubscription
	for {
		// Create the extrinsic
		ext := types.NewExtrinsic(c)
		genesisHash, err := s.client.RPC.Chain.GetBlockHash(0)
		if err != nil {
			return err
		}

		rv, err := s.client.RPC.State.GetRuntimeVersionLatest()
		if err != nil {
			return err
		}

		// Get the nonce for Alice
		key, err := types.CreateStorageKey(s.meta, "System", "Account", s.signer.PublicKey)
		if err != nil {
			return err
		}

		var accountInfo types.AccountInfo
		ok, err := s.client.RPC.State.GetStorageLatest(key, &accountInfo)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("account not found")
		}
		nonce := uint32(accountInfo.Nonce)
		o := types.SignatureOptions{
			BlockHash:          genesisHash,
			Era:                types.ExtrinsicEra{IsMortalEra: false},
			GenesisHash:        genesisHash,
			Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
			SpecVersion:        rv.SpecVersion,
			Tip:                types.NewUCompactFromUInt(0),
			TransactionVersion: rv.TransactionVersion,
		}

		logging.Infof("set resolve %s\n", uri)

		// Sign the transaction using Alice's default account
		err = ext.Sign(s.signer, o)
		if err != nil {
			return err
		}

		// Do the transfer and track the actual status
		sub, err = s.client.RPC.Author.SubmitAndWatchExtrinsic(ext)
		if err != nil {
			logging.Warningf("set_resolve submit failed: %v", err)
			continue
		}
		break
	}
	defer sub.Unsubscribe()
	for {
		status := <-sub.Chan()

		// wait until finalisation
		if status.IsInBlock || status.IsFinalized {
			break
		}

		logging.Infof("waiting for the set_resolve to be included/finalized")
	}
	return nil
}

func (s *acl) GetResolve(uri string) (cid []byte, err error) {
	aid, path, err := s.parseUri(uri)
	if err != nil {
		return nil, err
	}
	key, err := types.CreateStorageKey(s.meta, "Acl", "Resolves", aid, path)
	if err != nil {
		return
	}
	res := &types.AccountID{}
	ok, err := s.client.RPC.State.GetStorageLatest(key, &res)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = errors.New("Acl.Resolves is empty")
		return
	}
	return res.ToBytes(), nil
}

func (s *acl) parseUri(uri string) (accountId, path []byte, err error) {
	list := strings.Split(uri, "/")
	if len(list) < 2 {
		err = errors.New("uri invalid")
		return
	}
	if list[0] == "" {
		accountId = s.signer.PublicKey
	} else {
		// get accountId
		id, e := s.GetAccountID(list[0])
		if e != nil {
			err = e
			return
		}
		accountId = codec.MustHexDecodeString(id)
	}
	path = []byte(strings.TrimPrefix(uri, list[0]))

	buf := bytes.NewBuffer(nil)
	enc := scale.NewEncoder(buf)
	if err = enc.Encode(path); err != nil {
		return
	}

	return accountId, buf.Bytes(), nil
}
