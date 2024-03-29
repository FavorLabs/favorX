package ens

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/resolver/client"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/multiformats/go-multihash"
	goens "github.com/wealdtech/go-ens/v3"
)

const (
	defaultENSContractAddress = "A959836a03abbc7bF7EFAD8F9422456150bFE567"
	contentHashPrefix         = "favor://"
)

// Address is the boson address.
type Address = boson.Address

// Make sure Client implements the resolver.Client interface.
var _ client.Interface = (*Client)(nil)

var (
	// ErrFailedToConnect denotes that the resolver failed to connect to the
	// provided endpoint.
	ErrFailedToConnect = errors.New("failed to connect")
	// ErrResolveFailed denotes that a name could not be resolved.
	ErrResolveFailed = errors.New("resolve failed")
	// ErrInvalidContentHash denotes that the value of the content hash record is
	// not valid.
	ErrInvalidContentHash = errors.New("invalid boson content hash")
	// errNotImplemented denotes that the function has not been implemented.
	errNotImplemented = errors.New("function not implemented")
	// errNameNotRegistered denotes that the name is not registered.
	errNameNotRegistered = errors.New("name is not registered")
)

// Client is a name resolution client that can connect to ENS via an
// Ethereum endpoint.
type Client struct {
	endpoint     string
	contractAddr string
	ethCl        *ethclient.Client
	connectFn    func(string, string) (*ethclient.Client, *goens.Registry, error)
	resolveFn    func(*goens.Registry, common.Address, string) (string, error)
	registry     *goens.Registry
}

// Option is a function that applies an option to a Client.
type Option func(*Client)

// NewClient will return a new Client.
func NewClient(endpoint string, opts ...Option) (client.Interface, error) {
	c := &Client{
		endpoint:  endpoint,
		connectFn: wrapDial,
		resolveFn: wrapResolve,
	}

	// Apply all options to the Client.
	for _, o := range opts {
		o(c)
	}

	// Set the default ENS contract address.
	if c.contractAddr == "" {
		c.contractAddr = defaultENSContractAddress
	}

	// Establish a connection to the ENS.
	if c.connectFn == nil {
		return nil, fmt.Errorf("connectFn: %w", errNotImplemented)
	}
	ethCl, registry, err := c.connectFn(c.endpoint, c.contractAddr)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, ErrFailedToConnect)
	}
	c.ethCl = ethCl
	c.registry = registry

	return c, nil
}

// WithContractAddress will set the ENS contract address.
func WithContractAddress(addr string) Option {
	return func(c *Client) {
		c.contractAddr = addr
	}
}

// IsConnected returns true if there is an active RPC connection with an
// Ethereum node at the configured endpoint.
func (c *Client) IsConnected() bool {
	return c.ethCl != nil
}

// Endpoint returns the endpoint the client was connected to.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// Resolve implements the resolver.Client interface.
func (c *Client) Resolve(name string) (Address, error) {
	if c.resolveFn == nil {
		return boson.ZeroAddress, fmt.Errorf("resolveFn: %w", errNotImplemented)
	}

	hash, err := c.resolveFn(c.registry, common.HexToAddress(c.contractAddr), name)
	if err != nil {
		return boson.ZeroAddress, fmt.Errorf("%v: %w", err, ErrResolveFailed)
	}

	// Ensure that the content hash string is in a valid format, eg.
	// "favor://<address>".
	if !strings.HasPrefix(hash, contentHashPrefix) {
		return boson.ZeroAddress, fmt.Errorf("contenthash %s: %w", hash, ErrInvalidContentHash)
	}

	// Trim the prefix and try to parse the result as an address.
	return boson.ParseHexAddress(strings.TrimPrefix(hash, contentHashPrefix))
}

// Close closes the RPC connection with the client, terminating all unfinished
// requests. If the connection is already closed, this call is a noop.
func (c *Client) Close() error {
	if c.ethCl != nil {
		c.ethCl.Close()

	}
	c.ethCl = nil

	return nil
}

func wrapDial(endpoint string, contractAddr string) (*ethclient.Client, *goens.Registry, error) {
	// Dial the eth client.
	ethCl, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("dial: %w", err)
	}

	// Obtain the ENS registry.
	registry, err := goens.NewRegistryAt(ethCl, common.HexToAddress(contractAddr))
	if err != nil {
		return nil, nil, fmt.Errorf("new registry: %w", err)
	}

	// Ensure that the ENS registry client is deployed to the given contract address.
	_, err = registry.Owner("")
	if err != nil {
		return nil, nil, fmt.Errorf("owner: %w", err)
	}

	return ethCl, registry, nil
}

func wrapResolve(registry *goens.Registry, addr common.Address, name string) (string, error) {
	// Ensure the name is registered.
	ownerAddress, err := registry.Owner(name)
	if err != nil {
		return "", fmt.Errorf("owner: %w", err)
	}

	// If the name is not registered, return an error.
	if bytes.Equal(ownerAddress.Bytes(), goens.UnknownAddress.Bytes()) {
		return "", errNameNotRegistered
	}

	// Obtain the resolver for this domain name.
	ensR, err := registry.Resolver(name)
	if err != nil {
		return "", fmt.Errorf("resolver: %w", err)
	}

	// Try and read out the content hash record.
	ch, err := ensR.Contenthash()
	if err != nil {
		return "", fmt.Errorf("contenthash: %w", err)
	}

	decodedMHash, err := multihash.Decode(ch)
	if err != nil {
		return "", err
	}

	subPrefix := []byte(contentHashPrefix)

	prefixIndex := bytes.Index(decodedMHash.Digest, subPrefix)
	if prefixIndex != 0 {
		return "", fmt.Errorf("only support aurora-format contenthash")
	}

	contentHash := hex.EncodeToString(decodedMHash.Digest[len(subPrefix):])
	prefix := string(decodedMHash.Digest[:len(subPrefix)])

	return prefix + contentHash, nil
}
