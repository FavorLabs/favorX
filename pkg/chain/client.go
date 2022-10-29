package chain

import (
	"github.com/FavorLabs/favorX/pkg/chain/rpc/acl"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
)

type Client struct {
	Default *base.SubstrateAPI
	Acl     acl.Interface
	signer  signature.KeyringPair
}

func NewClient(url string, signer signature.KeyringPair) (*Client, error) {
	api, err := base.NewSubstrateAPI(url)
	if err != nil {
		return nil, err
	}
	return &Client{
		Default: api,
		Acl:     acl.New(api, signer),
	}, nil
}
