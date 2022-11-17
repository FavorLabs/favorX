package acl

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type Interface interface {
	base.CheckExtrinsicInterface
	SetNicknameWatch(ctx context.Context, name string) error
	SelfNickname() (name string, err error)
	GetNickName(accountId []byte) (name string, err error)
	GetAccountID(nickname string) (accountID types.AccountID, err error)
	SetResolveWatch(ctx context.Context, uri string, cid []byte) error
	GetResolve(uri string) (cid []byte, err error)
}

type ChainResult struct {
	// success bool
	TxHash []byte
	// reason  string
}

// exposes methods for querying oracle
type service struct {
	base.CheckExtrinsicInterface
	client *base.SubstrateAPI
	meta   *types.Metadata
	submit chan<- *base.SubmitTrans
}

func (s *service) CheckExtrinsic(block types.Hash, txn types.Hash) (has bool, err error) {
	index, err := s.client.GetExtrinsicIndex(block, txn)
	if index > -1 {
		has = true
	}
	if err != nil {
		return
	}
	recordsRaw, err := s.client.GetEventRecordsRaw(block)
	if err != nil {
		return
	}
	var ev EventRecords
	err = recordsRaw.DecodeEventRecords(s.meta, &ev)
	if err != nil {
		return
	}

	for _, v := range ev.System_ExtrinsicFailed {
		if v.Phase.AsApplyExtrinsic == uint32(index) {
			if v.DispatchError.IsModule {
				err = NewError(v.DispatchError.ModuleError)
				return
			}
			if v.DispatchError.IsToken {
				err = base.TokenError
				return
			}
			if v.DispatchError.IsArithmetic {
				err = base.ArithmeticError
				return
			}
			if v.DispatchError.IsTransactional {
				err = base.TransactionalError
				return
			}
		}
	}
	return
}

// New creates a new service struct
func New(c *base.SubstrateAPI, ch chan<- *base.SubmitTrans) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &service{
		client: c,
		meta:   meta,
		submit: ch,
	}
}

func (s *service) SetNicknameWatch(ctx context.Context, name string) error {
	c, err := types.NewCall(s.meta, "Acl.set_nickname", name)
	if err != nil {
		return err
	}

	submit := &base.SubmitTrans{
		Await:          true,
		Call:           c,
		CheckExtrinsic: s.CheckExtrinsic,
		TxResult:       make(chan base.TxResult),
	}
	s.submit <- submit
	select {
	case res := <-submit.TxResult:
		return res.Err
	case <-ctx.Done():
		submit.Cancel = true
		return ctx.Err()
	}
}

func (s *service) GetNickName(accountId []byte) (name string, err error) {
	key, err := types.CreateStorageKey(s.meta, "Acl", "AccountNickname", accountId)
	if err != nil {
		return
	}
	ok, err := s.client.RPC.State.GetStorageLatest(key, &name)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = base.KeyEmptyError
		return
	}
	return
}

func (s *service) GetAccountID(nickname string) (accountID types.AccountID, err error) {
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
	ok, err := s.client.RPC.State.GetStorageLatest(key, &accountID)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = base.KeyEmptyError
		return
	}
	return
}

func (s *service) SelfNickname() (name string, err error) {
	key, err := types.CreateStorageKey(s.meta, "Acl", "AccountNickname", s.client.Signer.PublicKey)
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

func (s *service) SetResolveWatch(ctx context.Context, uri string, cid []byte) (err error) {
	accountID, err := types.NewAccountID(cid)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Acl.set_resolve", uri, accountID)
	if err != nil {
		return
	}

	submit := &base.SubmitTrans{
		Await:          true,
		Call:           c,
		CheckExtrinsic: s.CheckExtrinsic,
		TxResult:       make(chan base.TxResult),
	}
	s.submit <- submit
	select {
	case res := <-submit.TxResult:
		return res.Err
	case <-ctx.Done():
		submit.Cancel = true
		return ctx.Err()
	}
}

func (s *service) GetResolve(uri string) (cid []byte, err error) {
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
		err = base.KeyEmptyError
		return
	}
	return res.ToBytes(), nil
}

func (s *service) parseUri(uri string) (accountId, path []byte, err error) {
	list := strings.Split(uri, "/")
	if len(list) < 2 {
		err = errors.New("uri invalid")
		return
	}
	if list[0] == "" {
		accountId = s.client.Signer.PublicKey
	} else {
		// get accountId
		id, e := s.GetAccountID(list[0])
		if e != nil {
			err = e
			return
		}
		accountId = id.ToBytes()
	}
	path = []byte(strings.TrimPrefix(uri, list[0]))

	buf := bytes.NewBuffer(nil)
	enc := scale.NewEncoder(buf)
	if err = enc.Encode(path); err != nil {
		return
	}

	return accountId, buf.Bytes(), nil
}
