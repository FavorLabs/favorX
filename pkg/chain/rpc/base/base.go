package base

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/config"
	gethrpc "github.com/centrifuge/go-substrate-rpc-client/v4/gethrpc"
	"github.com/centrifuge/go-substrate-rpc-client/v4/rpc"
	"github.com/centrifuge/go-substrate-rpc-client/v4/rpc/author"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
)

type CheckExtrinsicInterface interface {
	CheckExtrinsic(block types.Hash, ext types.Extrinsic) (err error)
}

type Client interface {
	// Call makes the call to RPC method with the provided args,
	// args must be encoded in the format RPC understands
	Call(result interface{}, method string, args ...interface{}) error
	CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error
	Subscribe(ctx context.Context, namespace, subscribeMethodSuffix, unsubscribeMethodSuffix,
		notificationMethodSuffix string, channel interface{}, args ...interface{}) (
		*gethrpc.ClientSubscription, error)

	URL() string
}

type client struct {
	gethrpc.Client

	url string
}

// URL returns the URL the client connects to
func (c client) URL() string {
	return c.url
}

// Connect connects to the provided url
func Connect(url string) (Client, error) {
	logging.Infof("substrate client connecting to %v...", url)

	ctx, cancel := context.WithTimeout(context.Background(), config.Default().DialTimeout)
	defer cancel()

	c, err := gethrpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	cc := client{*c, url}
	return &cc, nil
}

func CallWithBlockHash(c Client, target interface{}, method string, blockHash *types.Hash, args ...interface{}) error {
	if blockHash == nil {
		err := c.Call(target, method, args...)
		if err != nil {
			return err
		}
		return nil
	}
	hexHash, err := codec.Hex(*blockHash)
	if err != nil {
		return err
	}
	args = append(args, hexHash)
	err = c.Call(target, method, args...)
	if err != nil {
		return err
	}
	return nil
}

type SubstrateAPI struct {
	CheckExtrinsicInterface
	RPC    *rpc.RPC
	Client Client
	Signer signature.KeyringPair
}

func NewSubstrateAPI(url string, signer signature.KeyringPair) (*SubstrateAPI, error) {
	cl, err := Connect(url)
	if err != nil {
		return nil, err
	}

	newRPC, err := rpc.NewRPC(cl)
	if err != nil {
		return nil, err
	}

	s := &SubstrateAPI{
		RPC:    newRPC,
		Client: cl,
		Signer: signer,
	}
	return s, nil
}

func (s *SubstrateAPI) GetSignatureOptions() (option types.SignatureOptions, res error) {
	genesisHash, err := s.RPC.Chain.GetBlockHash(0)
	if err != nil {
		return
	}

	rv, err := s.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		return
	}

	meta, err := s.RPC.State.GetMetadataLatest()
	if err != nil {
		return
	}

	// Get the nonce for Alice
	key, err := types.CreateStorageKey(meta, "System", "Account", s.Signer.PublicKey)
	if err != nil {
		return
	}

	var accountInfo types.AccountInfo
	ok, err := s.RPC.State.GetStorageLatest(key, &accountInfo)
	if err != nil {
		return
	}
	if !ok {
		err = errors.New("account not found")
		return
	}
	nonce := uint32(accountInfo.Nonce)
	option = types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	}
	return
}

func (s *SubstrateAPI) GetExtrinsicIndex(blockHash types.Hash, extBytes []byte) (out int, err error) {
	block, err := s.RPC.Chain.GetBlock(blockHash)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	for k, v := range block.Block.Extrinsics {
		b, _ := v.MarshalJSON()
		if bytes.Equal(b, extBytes) {
			return k, nil
		}
	}
	return 0, errors.New("extrinsic not found")
}

func (s *SubstrateAPI) GetEventRecordsRaw(blockHash types.Hash) (res types.EventRecordsRaw, err error) {
	meta, err := s.RPC.State.GetMetadataLatest()
	if err != nil {
		return
	}
	key, err := types.CreateStorageKey(meta, "System", "Events")
	if err != nil {
		return
	}

	ok, err := s.RPC.State.GetStorage(key, &res, blockHash)
	if err != nil {
		logging.Warningf("gsrpc err: %w", err)
		return
	}
	if !ok {
		err = errors.New("System.Events is empty")
		return
	}
	return
}

func (s *SubstrateAPI) SubmitExtrinsicAndWatch(c types.Call, fn CheckExtrinsicInterface) (err error) {
	o, err := s.GetSignatureOptions()
	if err != nil {
		return
	}
	// Create the extrinsic
	ext := types.NewExtrinsic(c)

	var sub *author.ExtrinsicStatusSubscription

	for {
		err = ext.Sign(s.Signer, o)
		if err != nil {
			return
		}

		sub, err = s.RPC.Author.SubmitAndWatchExtrinsic(ext)
		if err != nil {
			o.Nonce = types.NewUCompactFromUInt(uint64(o.Nonce.Int64() + 1))
			logging.Debug("extrinsic submit failed: %v", err)
			continue
		}
		break
	}
	defer sub.Unsubscribe()
	timeout := time.After(ExtrinsicTimeout)

	for {
		select {
		case status := <-sub.Chan():
			if status.IsInBlock || status.IsFinalized {
				return fn.CheckExtrinsic(status.AsInBlock, ext)
			}
		case <-timeout:
			err = ExtrinsicTimeoutError
			return
		}
	}
}
