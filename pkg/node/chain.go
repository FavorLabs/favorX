package node

import (
	"context"
	"fmt"

	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/p2p/libp2p"
	"github.com/FavorLabs/favorX/pkg/settlement/pseudosettle"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gauss-project/aurorafs/pkg/logging"
	"github.com/gauss-project/aurorafs/pkg/settlement"
	"github.com/gauss-project/aurorafs/pkg/settlement/chain"
	chainCommon "github.com/gauss-project/aurorafs/pkg/settlement/chain/common"
	"github.com/gauss-project/aurorafs/pkg/settlement/chain/oracle"
	"github.com/gauss-project/aurorafs/pkg/settlement/traffic"
	"github.com/gauss-project/aurorafs/pkg/subscribe"
)

// InitChain will initialize the Ethereum backend at the given endpoint and
// set up the Transaction Service to interact with it using the provided signer.
func InitChain(
	ctx context.Context,
	logger logging.Logger,
	endpoint string,
	oracleContractAddress string,
	stateStore storage.StateStorer,
	signer crypto.Signer,
	trafficEnable bool,
	trafficContractAddr string,
	p2pService *libp2p.Service,
	subPub subscribe.SubPub,
) (chain.Resolver, settlement.Interface, traffic.ApiInterface, chain.Common, error) {
	backend, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("dial eth client: %w", err)
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		logger.Infof("could not connect to backend at %v. In a swap-enabled network a working blockchain node (for goerli network in production) is required. Check your node or specify another node using --traffic-endpoint.", endpoint)
		return nil, nil, nil, nil, fmt.Errorf("get chain id: %w", err)
	}
	cc, err := chainCommon.New(logger, signer, chainID, endpoint)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new common serveice: %v", err)
	}
	if oracleContractAddress == "" {
		return nil, nil, nil, nil, fmt.Errorf("oracle contract address is empty")
	}
	oracleServer, err := oracle.NewServer(logger, backend, oracleContractAddress, signer, cc, subPub)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new oracle service: %w", err)
	}

	address, err := signer.EthereumAddress()
	logger.Infof("address  %s", address.String())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("chain address: %w", err)
	}

	service := pseudosettle.New(p2pService, logger, stateStore, address)
	if err = service.Init(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("InitTraffic:: %w", err)
	}

	return oracleServer, service, service, cc, nil
}
