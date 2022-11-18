package storage

import (
	"bytes"
	"context"
	"errors"

	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type Interface interface {
	base.CheckExtrinsicInterface
	GetNodesFromCid(cid []byte) ([]types.AccountID, error)
	RegisterCidAndNode(ctx context.Context, rootCid []byte, address []byte, fn base.Finalized) (hash types.Hash, err error)
	RegisterCidAndNodeWatch(ctx context.Context, cid []byte, overlay []byte) (err error)
	RemoveCidAndNode(ctx context.Context, rootCid []byte, address []byte, fn base.Finalized) (hash types.Hash, err error)
	RemoveCidAndNodeWatch(ctx context.Context, cid []byte, overlay []byte) (err error)
	StorageFileWatch(ctx context.Context, buyer []byte, cid []byte) (err error)
	PlaceOrder(ctx context.Context, cid []byte, fileSize, fileCopy uint64, expire uint32, fn OrderMatchSuccess) (txn types.Hash, err error)
	PlaceOrderWatch(ctx context.Context, cid []byte, fileSize, fileCopy uint64, expire uint32) (users []types.AccountID, err error)
	MerchantRegisterWatch(ctx context.Context, diskTotal uint64) (err error)
	MerchantUnregisterWatch(ctx context.Context) (err error)
	GetMerchantInfo(mch []byte) (*MerchantInfo, error)
	CheckOrder(buyer, rootCid, mch []byte) error
}

// storage exposes methods for querying storage
type service struct {
	base.CheckExtrinsicInterface
	client *base.SubstrateAPI
	meta   *types.Metadata
	submit chan<- *base.SubmitTrans
}

// New creates a new oracle struct
func New(c *base.SubstrateAPI, ch chan<- *base.SubmitTrans) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &service{
		client: c,
		meta:   meta,
		submit: ch,
	}
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
			}
			if v.DispatchError.IsArithmetic {
				err = base.ArithmeticError
				return
			}
			if v.DispatchError.IsTransactional {
				err = base.TransactionalError
				return
			}
			break
		}
	}
	return
}

func (s *service) GetNodesFromCid(cid []byte) (overlays []types.AccountID, err error) {
	key, err := types.CreateStorageKey(s.meta, "Storage", "Oracles", cid)
	if err != nil {
		return
	}
	ok, err := s.client.RPC.State.GetStorageLatest(key, &overlays)
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

func (s *service) RegisterCidAndNodeWatch(ctx context.Context, rootCid []byte, address []byte) (err error) {
	accountID, err := types.NewAccountID(rootCid)
	if err != nil {
		return
	}
	overlay, err := types.NewAccountID(address)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Storage.oracle_register", accountID, overlay)
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

func (s *service) RemoveCidAndNodeWatch(ctx context.Context, rootCid []byte, address []byte) (err error) {
	accountID, err := types.NewAccountID(rootCid)
	if err != nil {
		return
	}
	overlay, err := types.NewAccountID(address)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Storage.oracle_remove", accountID, overlay)
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

func (s *service) StorageFileWatch(ctx context.Context, buyer []byte, cid []byte) (err error) {
	accountID, err := types.NewAccountID(buyer)
	if err != nil {
		return
	}
	hash, err := types.NewAccountID(cid)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Storage.storage_file", accountID, hash)
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

func (s *service) PlaceOrder(ctx context.Context, cid []byte, fileSize, fileCopy uint64, expire uint32, fn OrderMatchSuccess) (txn types.Hash, err error) {
	hash, err := types.NewAccountID(cid)
	if err != nil {
		return
	}

	c, err := types.NewCall(s.meta, "Storage.place_order", hash, types.NewU64(fileSize), types.NewU64(fileCopy), types.NewU32(expire))
	if err != nil {
		return
	}

	check := func(block types.Hash, txn types.Hash) (has bool, err error) {
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
				}
				if v.DispatchError.IsArithmetic {
					err = base.ArithmeticError
					return
				}
				if v.DispatchError.IsTransactional {
					err = base.TransactionalError
					return
				}
				break
			}
		}
		return
	}

	submit := &base.SubmitTrans{
		Await:          false,
		Call:           c,
		CheckExtrinsic: check,
		TxResult:       make(chan base.TxResult),
		Finalized: func(block types.Hash, txn types.Hash) {
			index, err := s.client.GetExtrinsicIndex(block, txn)
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
			for _, v := range ev.Storage_OrderMatchSuccess {
				if v.Phase.AsApplyExtrinsic == uint32(index) {
					fn(v.Merchant)
				}
			}
		},
	}
	s.submit <- submit
	select {
	case res := <-submit.TxResult:
		txn = res.TransHash
		err = res.Err
	case <-ctx.Done():
		submit.Cancel = true
		err = ctx.Err()
	}
	return
}

func (s *service) PlaceOrderWatch(ctx context.Context, cid []byte, fileSize, fileCopy uint64, expire uint32) (users []types.AccountID, err error) {
	hash, err := types.NewAccountID(cid)
	if err != nil {
		return
	}

	c, err := types.NewCall(s.meta, "Storage.place_order", hash, types.NewU64(fileSize), types.NewU64(fileCopy), types.NewU32(expire))
	if err != nil {
		return
	}
	check := func(block types.Hash, txn types.Hash) (has bool, err error) {
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
				}
				if v.DispatchError.IsArithmetic {
					err = base.ArithmeticError
					return
				}
				if v.DispatchError.IsTransactional {
					err = base.TransactionalError
					return
				}
				break
			}
		}

		for _, v := range ev.Storage_OrderMatchSuccess {
			if v.Phase.AsApplyExtrinsic == uint32(index) {
				users = append(users, v.Merchant)
			}
		}
		return
	}

	submit := &base.SubmitTrans{
		Await:          true,
		Call:           c,
		CheckExtrinsic: check,
		TxResult:       make(chan base.TxResult),
	}
	s.submit <- submit
	select {
	case res := <-submit.TxResult:
		err = res.Err
	case <-ctx.Done():
		submit.Cancel = true
		err = ctx.Err()
	}
	return
}

func (s *service) MerchantRegisterWatch(ctx context.Context, diskTotal uint64) (err error) {
	c, err := types.NewCall(s.meta, "Storage.merchant_register", types.NewU64(diskTotal))
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

func (s *service) MerchantUnregisterWatch(ctx context.Context) (err error) {
	c, err := types.NewCall(s.meta, "Storage.merchant_unregister")
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

func (s *service) RegisterCidAndNode(ctx context.Context, rootCid []byte, address []byte, fn base.Finalized) (hash types.Hash, err error) {
	accountID, err := types.NewAccountID(rootCid)
	if err != nil {
		return
	}
	overlay, err := types.NewAccountID(address)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Storage.oracle_register", accountID, overlay)
	if err != nil {
		return
	}

	submit := &base.SubmitTrans{
		Await:          false,
		Call:           c,
		CheckExtrinsic: s.CheckExtrinsic,
		TxResult:       make(chan base.TxResult),
		Finalized:      fn,
	}
	s.submit <- submit
	select {
	case res := <-submit.TxResult:
		hash = res.TransHash
		err = res.Err
	case <-ctx.Done():
		submit.Cancel = true
		err = ctx.Err()
	}
	return
}

func (s *service) RemoveCidAndNode(ctx context.Context, rootCid []byte, address []byte, fn base.Finalized) (hash types.Hash, err error) {
	accountID, err := types.NewAccountID(rootCid)
	if err != nil {
		return
	}
	overlay, err := types.NewAccountID(address)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Storage.oracle_remove", accountID, overlay)
	if err != nil {
		return
	}

	submit := &base.SubmitTrans{
		Await:          false,
		Call:           c,
		CheckExtrinsic: s.CheckExtrinsic,
		TxResult:       make(chan base.TxResult),
		Finalized:      fn,
	}
	s.submit <- submit
	select {
	case res := <-submit.TxResult:
		hash = res.TransHash
		err = res.Err
	case <-ctx.Done():
		submit.Cancel = true
		err = ctx.Err()
	}
	return
}

func (s *service) GetMerchantInfo(mch []byte) (*MerchantInfo, error) {
	key, err := types.CreateStorageKey(s.meta, "Storage", "MerchantServiceList", mch)
	if err != nil {
		return nil, err
	}
	var info MerchantInfo
	ok, err := s.client.RPC.State.GetStorageLatest(key, &info)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return nil, err
	}
	if !ok {
		return nil, base.KeyEmptyError
	}
	return &info, nil
}

func (s *service) CheckOrder(buyer, rootCid, mch []byte) error {
	key, err := types.CreateStorageKey(s.meta, "Storage", "Orders", buyer, rootCid)
	if err != nil {
		return err
	}
	var info OrderInfo
	ok, err := s.client.RPC.State.GetStorageLatest(key, &info)
	if err != nil {
		return err
	}
	if !ok {
		return base.KeyEmptyError
	}
	for _, v := range info.Merchants {
		if bytes.Equal(v.ToBytes(), mch) {
			return nil
		}
	}
	return errors.New("mismatch merchant")
}
