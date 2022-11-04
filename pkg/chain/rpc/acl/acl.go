package acl

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

type Interface interface {
	SetNicknameWatch(name string) error
	SelfNickname() (name string, err error)
	GetNickName(accountId string) (name string, err error)
	GetAccountID(nickname string) (accountID string, err error)
	SetResolveWatch(uri string, cid []byte) error
	GetResolve(uri string) (cid []byte, err error)

	// GetNodesFromCid  Get source nodes of specified cid
	GetNodesFromCid(cid []byte) (overlays []boson.Address)

	// GetCid Resolve cid from  uri
	GetCid(uri string) []byte

	GetSourceNodes(uri string) []boson.Address

	// OnStoreMatched Notification when new data req matched
	OnStoreMatched(cid boson.Address, dataLen uint64, salt uint64, address boson.Address)

	// DataStoreFinished when data retrieved and saved, use this function to report onchain
	DataStoreFinished(cid boson.Address, dataLen uint64, salt uint64, proof []byte, resCh chan ChainResult)
	RegisterCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (hash common.Hash, err error)
	RemoveCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (common.Hash, error)
	GetRegisterState(ctx context.Context, rootCid boson.Address, address boson.Address) (bool, error)
	WaitForReceipt(ctx context.Context, rootCid boson.Address, txHash common.Hash) (receipt *ethTypes.Receipt, err error)
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

func (s *service) GetCid(uri string) []byte {
	// TODO implement me
	panic("implement me")
}

func (s *service) GetSourceNodes(uri string) []boson.Address {
	// TODO implement me
	panic("implement me")
}

func (s *service) OnStoreMatched(cid boson.Address, dataLen uint64, salt uint64, address boson.Address) {
	// TODO implement me
	panic("implement me")
}

func (s *service) DataStoreFinished(cid boson.Address, dataLen uint64, salt uint64, proof []byte, resCh chan ChainResult) {
	// TODO implement me
	panic("implement me")
}

func (s *service) RegisterCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (hash common.Hash, err error) {
	// TODO implement me
	panic("implement me")
}

func (s *service) RemoveCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (common.Hash, error) {
	// TODO implement me
	panic("implement me")
}

func (s *service) GetRegisterState(ctx context.Context, rootCid boson.Address, address boson.Address) (bool, error) {
	// TODO implement me
	panic("implement me")
}

func (s *service) WaitForReceipt(ctx context.Context, rootCid boson.Address, txHash common.Hash) (receipt *ethTypes.Receipt, err error) {
	// TODO implement me
	panic("implement me")
}

// New creates a new service struct
func New(c *base.SubstrateAPI) Interface {
	meta, _ := c.RPC.State.GetMetadataLatest()
	return &service{
		client: c,
		meta:   meta,
	}
}

func (s *service) GetNodesFromCid(cid []byte) (overlays []boson.Address) {
	key, err := types.CreateStorageKey(s.meta, "Acl", "Oracles", s.client.Signer.PublicKey, cid)
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

func (s *service) SetNicknameWatch(name string) error {
	c, err := types.NewCall(s.meta, "Acl.set_nickname", name)
	if err != nil {
		return err
	}

	return s.client.SubmitExtrinsicAndWatch(c, s)
}

func (s *service) GetNickName(accountId string) (name string, err error) {
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

func (s *service) GetAccountID(nickname string) (accountID string, err error) {
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

func (s *service) SetResolveWatch(uri string, cid []byte) (err error) {
	accountID, err := types.NewAccountID(cid)
	if err != nil {
		return
	}
	c, err := types.NewCall(s.meta, "Acl.set_resolve", uri, accountID)
	if err != nil {
		return
	}
	return s.client.SubmitExtrinsicAndWatch(c, s)
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
		err = errors.New("Acl.Resolves is empty")
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
