package storage

import (
	"context"
	"errors"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type Interface interface {
	base.CheckExtrinsicInterface
	GetNodesFromCid(cid []byte) ([]types.AccountID, error)
	RegisterCidAndNode(rootCid []byte, address []byte) (hash types.Hash, err error)
	RegisterCidAndNodeWatch(ctx context.Context, cid []byte, overlay []byte) (err error)
	RemoveCidAndNode(rootCid []byte, address []byte) (hash types.Hash, err error)
	RemoveCidAndNodeWatch(ctx context.Context, cid []byte, overlay []byte) (err error)
	StorageFileWatch(ctx context.Context, buyer boson.Address, cid []byte, overlay []byte) (err error)
	PlaceOrderWatch(ctx context.Context, cid []byte, fileSize, fileCopy uint64, expire uint32) (users []types.AccountID, err error)
	MerchantRegisterWatch(ctx context.Context, diskTotal uint64) (err error)
	MerchantUnregisterWatch(ctx context.Context) (err error)
}

// storage exposes methods for querying storage
type service struct {
	base.CheckExtrinsicInterface
	client *base.SubstrateAPI
	meta   *types.Metadata
}

// New creates a new oracle struct
func New(c *base.SubstrateAPI) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &service{
		client: c,
		meta:   meta,
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
				err = NewError(v.DispatchError.ModuleError.Index)
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
		err = errors.New("is empty")
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

	return s.client.SubmitExtrinsicAndWatch(ctx, c, s.CheckExtrinsic)
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

	return s.client.SubmitExtrinsicAndWatch(ctx, c, s.CheckExtrinsic)
}

func (s *service) StorageFileWatch(ctx context.Context, buyer boson.Address, cid []byte, overlay []byte) (err error) {
	accountID, err := types.NewAccountID(buyer.Bytes())
	if err != nil {
		return
	}
	hash, err := types.NewAccountID(cid)
	if err != nil {
		return
	}
	ov, err := types.NewAccountID(overlay)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Storage.storage_file", accountID, hash, ov)
	if err != nil {
		return
	}

	return s.client.SubmitExtrinsicAndWatch(ctx, c, s.CheckExtrinsic)
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

	err = s.client.SubmitExtrinsicAndWatch(ctx, c, func(block types.Hash, txn types.Hash) (has bool, err error) {
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
					err = NewError(v.DispatchError.ModuleError.Index)
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
	})
	return
}

func (s *service) MerchantRegisterWatch(ctx context.Context, diskTotal uint64) (err error) {
	c, err := types.NewCall(s.meta, "Storage.merchant_register", types.NewU64(diskTotal))
	if err != nil {
		return
	}

	return s.client.SubmitExtrinsicAndWatch(ctx, c, s.CheckExtrinsic)
}

func (s *service) MerchantUnregisterWatch(ctx context.Context) (err error) {
	c, err := types.NewCall(s.meta, "Storage.merchant_unregister")
	if err != nil {
		return
	}

	return s.client.SubmitExtrinsicAndWatch(ctx, c, s.CheckExtrinsic)
}

func (s *service) RegisterCidAndNode(rootCid []byte, address []byte) (hash types.Hash, err error) {
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

	return s.client.SubmitExtrinsic(c)
}

func (s *service) RemoveCidAndNode(rootCid []byte, address []byte) (hash types.Hash, err error) {
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

	return s.client.SubmitExtrinsic(c)
}
