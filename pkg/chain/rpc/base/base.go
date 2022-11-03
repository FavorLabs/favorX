package base

import (
	"context"

	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/centrifuge/go-substrate-rpc-client/v4/config"
	gethrpc "github.com/centrifuge/go-substrate-rpc-client/v4/gethrpc"
	"github.com/centrifuge/go-substrate-rpc-client/v4/rpc"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
)

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
	RPC    *rpc.RPC
	Client Client
}

func NewSubstrateAPI(url string) (*SubstrateAPI, error) {
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
	}
	return s, nil
}
