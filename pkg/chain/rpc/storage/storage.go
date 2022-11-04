package storage

import (
	"errors"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
)

type Interface interface {
	GetNodesFromCid(cid []byte) ([]boson.Address, error)
	StorageFileWatch(buyer boson.Address, cid []byte) (err error)
	PlaceOrderWatch(cid []byte, fileSize, fileCopy uint64, expire uint32) (err error)
	MerchantRegisterWatch(diskTotal uint64) (err error)
	MerchantUnregisterWatch() (err error)
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

func (s *service) CheckExtrinsic(block types.Hash, ext types.Extrinsic) (err error) {
	b, err := ext.MarshalJSON()
	if err != nil {
		return
	}
	index, err := s.client.GetExtrinsicIndex(block, b)
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
				return NewError(v.DispatchError.ModuleError.Index)
			}
			if v.DispatchError.IsToken {
				return base.TokenError
			}
			if v.DispatchError.IsArithmetic {
				return base.ArithmeticError
			}
			if v.DispatchError.IsTransactional {
				return base.TransactionalError
			}
		}
	}
	return
}

func (s *service) GetNodesFromCid(cid []byte) (overlays []boson.Address, err error) {
	key, err := types.CreateStorageKey(s.meta, "Storage", "Oracles", codec.MustHexDecodeString(boson.NewAddress(cid).String()))
	if err != nil {
		return
	}
	var res []types.AccountID
	ok, err := s.client.RPC.State.GetStorageLatest(key, &res)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = errors.New("Storage.Oracles is empty")
		return
	}
	for _, v := range res {
		overlays = append(overlays, boson.NewAddress(v.ToBytes()))
	}
	return
}

func (s *service) StorageFileWatch(buyer boson.Address, cid []byte) (err error) {
	accountID, err := types.NewAccountID(buyer.Bytes())
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

	return s.client.SubmitExtrinsicAndWatch(c, s)
}

func (s *service) PlaceOrderWatch(cid []byte, fileSize, fileCopy uint64, expire uint32) (err error) {
	hash, err := types.NewAccountID(cid)
	if err != nil {
		return
	}

	c, err := types.NewCall(s.meta, "Storage.place_order", hash, types.NewU64(fileSize), types.NewU64(fileCopy), types.NewU32(expire))
	if err != nil {
		return
	}

	return s.client.SubmitExtrinsicAndWatch(c, s)
}

func (s *service) MerchantRegisterWatch(diskTotal uint64) (err error) {
	c, err := types.NewCall(s.meta, "Storage.merchant_register", types.NewU64(diskTotal))
	if err != nil {
		return
	}

	return s.client.SubmitExtrinsicAndWatch(c, s)
}

func (s *service) MerchantUnregisterWatch() (err error) {
	c, err := types.NewCall(s.meta, "Storage.merchant_unregister")
	if err != nil {
		return
	}

	return s.client.SubmitExtrinsicAndWatch(c, s)
}
