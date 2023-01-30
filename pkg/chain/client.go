package chain

import (
	"github.com/FavorLabs/favorX/pkg/chain/rpc/acl"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/proxy"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/storage"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/tokens"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/traffic"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/rpc/author"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
	"github.com/decred/dcrd/crypto/blake256"
)

type MainClient struct {
	Default         *base.SubstrateAPI
	Proxy           proxy.Interface
	SubmitTransChan chan *base.SubmitTrans
	Acl             acl.Interface
	Tokens          tokens.Interface
}

type SubChainClient struct {
	Default         *base.SubstrateAPI
	Proxy           proxy.Interface
	SubmitTransChan chan *base.SubmitTrans
	Traffic         traffic.Interface
	Storage         storage.Interface
}

func (s *SubChainClient) CloneTo(p *SubChainClient) {
	p.Traffic = s.Traffic
	p.Proxy = s.Proxy
	p.SubmitTransChan = s.SubmitTransChan
	p.Default = s.Default
	p.Storage = s.Storage
}

func (s *MainClient) CloneTo(p *MainClient) {
	p.Proxy = s.Proxy
	p.SubmitTransChan = s.SubmitTransChan
	p.Default = s.Default
	p.Acl = s.Acl
	p.Tokens = s.Tokens
}

func NewSubChainClient(url string, signer signature.KeyringPair) (*SubChainClient, error) {
	api, err := base.NewSubstrateAPI(url, signer)
	if err != nil {
		return nil, err
	}
	ch := make(chan *base.SubmitTrans, 10)
	go start(ch, api, signer)

	return &SubChainClient{
		SubmitTransChan: ch,
		Default:         api,
		Proxy:           proxy.New(api),
		Traffic:         traffic.New(api),
		Storage:         storage.New(api, ch),
	}, nil
}

func NewClient(url string, signer signature.KeyringPair) (*MainClient, error) {
	api, err := base.NewSubstrateAPI(url, signer)
	if err != nil {
		return nil, err
	}
	ch := make(chan *base.SubmitTrans, 10)
	go start(ch, api, signer)

	return &MainClient{
		SubmitTransChan: ch,
		Default:         api,
		Proxy:           proxy.New(api),
		Acl:             acl.New(api, ch),
		Tokens:          tokens.New(api),
	}, nil
}

func start(ch <-chan *base.SubmitTrans, api *base.SubstrateAPI, signer signature.KeyringPair) {
	for {
		select {
		case tx := <-ch:
			if tx.Cancel {
				break
			}
			txHash, sub, err := submit(signer, api, tx.Call)
			if err != nil {
				tx.TxResult <- base.TxResult{Err: err}
				break
			}
			if tx.Await {
				err = wait(sub, txHash, tx.CheckExtrinsic, tx.Finalized)
				tx.TxResult <- base.TxResult{TransHash: txHash, Err: err}
			} else {
				go wait(sub, txHash, tx.CheckExtrinsic, tx.Finalized)
				tx.TxResult <- base.TxResult{TransHash: txHash}
			}
		}
	}
}

func submit(signer signature.KeyringPair, api *base.SubstrateAPI, c types.Call) (txHash types.Hash, sub *author.ExtrinsicStatusSubscription, err error) {
	o, err := api.GetSignatureOptions()
	if err != nil {
		return
	}
	// Create the extrinsic
	ext := types.NewExtrinsic(c)

	for {
		err = ext.Sign(signer, o)
		if err != nil {
			return
		}

		sub, err = api.RPC.Author.SubmitAndWatchExtrinsic(ext)
		if err != nil {
			o.Nonce = types.NewUCompactFromUInt(uint64(o.Nonce.Int64() + 1))
			logging.Debug("extrinsic submit watch failed: %v", err)
			continue
		}
		break
	}

	extBytes, _ := codec.Encode(ext)
	txHash = blake256.Sum256(extBytes)
	return
}

func wait(sub *author.ExtrinsicStatusSubscription, txn types.Hash, check base.CheckExtrinsic, final base.Finalized) (err error) {
	defer sub.Unsubscribe()

	for {
		select {
		case status := <-sub.Chan():
			if status.IsInBlock {
				if check != nil {
					_, err = check(status.AsInBlock, txn)
					if err != nil {
						return
					}
				}
			}
			if status.IsFinalized {
				if final != nil {
					final(status.AsFinalized, txn)
				}
				return
			}
		}
	}
}
