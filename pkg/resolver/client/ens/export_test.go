package ens

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	goens "github.com/wealdtech/go-ens/v3"
)

const ContentHashPrefix = contentHashPrefix

var ErrNotImplemented = errNotImplemented

// WithConnectFunc will set the Dial function implementaton.
func WithConnectFunc(fn func(endpoint string, contractAddr string) (*ethclient.Client, *goens.Registry, error)) Option {
	return func(c *Client) {
		c.connectFn = fn
	}
}

// WithResolveFunc will set the Resolve function implementation.
func WithResolveFunc(fn func(registry *goens.Registry, addr common.Address, input string) (string, error)) Option {
	return func(c *Client) {
		c.resolveFn = fn
	}
}
