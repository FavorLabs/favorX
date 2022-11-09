package chain

import (
	"github.com/FavorLabs/favorX/pkg/chain/rpc/acl"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/proxy"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/storage"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/traffic"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
)

type Client struct {
	Default *base.SubstrateAPI
	Acl     acl.Interface
	Traffic traffic.Interface
	Proxy   proxy.Interface
	Storage storage.Interface
}

func NewClient(url string, signer signature.KeyringPair) (*Client, error) {
	api, err := base.NewSubstrateAPI(url, signer)
	if err != nil {
		return nil, err
	}
	return &Client{
		Default: api,
		Acl:     acl.New(api),
		Traffic: traffic.New(api),
		Proxy:   proxy.New(api),
		Storage: storage.New(api),
	}, nil
}
