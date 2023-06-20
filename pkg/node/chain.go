package node

import (
	"context"
	"fmt"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p/libp2p"
	"github.com/FavorLabs/favorX/pkg/settlement"
	"github.com/FavorLabs/favorX/pkg/settlement/chain"
	chainCommon "github.com/FavorLabs/favorX/pkg/settlement/chain/common"
	"github.com/FavorLabs/favorX/pkg/settlement/chain/oracle"
	chainTraffic "github.com/FavorLabs/favorX/pkg/settlement/chain/traffic"
	"github.com/FavorLabs/favorX/pkg/settlement/chain/transaction"
	"github.com/FavorLabs/favorX/pkg/settlement/pseudosettle"
	"github.com/FavorLabs/favorX/pkg/settlement/traffic"
	chequePkg "github.com/FavorLabs/favorX/pkg/settlement/traffic/cheque"
	"github.com/FavorLabs/favorX/pkg/settlement/traffic/trafficprotocol"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
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
	var (
		backend = &ethclient.Client{}
		chainID = &big.Int{}
		cc      = &chainCommon.Common{}
	)

	backend, err := ethclient.Dial(endpoint)
	if err != nil && (trafficEnable || oracleContractAddress != "") {
		return nil, nil, nil, nil, fmt.Errorf("dial eth client: %w", err)
	}

	if backend != nil && (trafficEnable || oracleContractAddress != "") {
		chainID, err = backend.ChainID(ctx)
		if err != nil {
			logger.Infof("could not connect to backend at %v. In a swap-enabled network a working blockchain node (for goerli network in production) is required. Check your node or specify another node using --chain-endpoint.", endpoint)
			return nil, nil, nil, nil, fmt.Errorf("get chain id: %w", err)
		}
		cc, err = chainCommon.New(logger, signer, chainID, endpoint)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("new common serveice: %v", err)
		}
		if oracleContractAddress == "" {
			return nil, nil, nil, nil, fmt.Errorf("oracle contract address is empty")
		}
	}
	oracleServer, err := oracle.NewServer(logger, backend, chainID, oracleContractAddress, signer, cc, subPub)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new oracle service: %w", err)
	}
	transactionService, err := transaction.NewService(logger, backend, signer, stateStore, cc, chainID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new transaction service: %w", err)
	}
	address, err := signer.EthereumAddress()
	logger.Infof("address  %s", address.String())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("chain address: %w", err)
	}

	if !trafficEnable {
		service := pseudosettle.New(p2pService, logger, stateStore, address)
		if err = service.Init(); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("InitTraffic:: %w", err)
		}

		return oracleServer, service, service, cc, nil
	}

	trafficChainService, err := chainTraffic.NewServer(logger, chainID, backend, signer, transactionService, trafficContractAddr, cc)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new traffic service: %w", err)
	}

	trafficService, err := InitTraffic(stateStore, address, trafficChainService, transactionService, logger, p2pService, signer, chainID.Int64(), trafficContractAddr, subPub)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	err = trafficService.Init()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("InitChain: %w", err)
	}

	return oracleServer, trafficService, trafficService, cc, nil
}

func InitTraffic(store storage.StateStorer, address common.Address, trafficChainService chain.Traffic,
	transactionService chain.Transaction, logger logging.Logger, p2pService *libp2p.Service, signer crypto.Signer, chainID int64, trafficContractAddr string, subPub subscribe.SubPub) (*traffic.Service, error) {
	chequeStore := chequePkg.NewChequeStore(store, address, chequePkg.RecoverCheque, chainID)
	cashOut := chequePkg.NewCashoutService(store, transactionService, trafficChainService, chequeStore, common.HexToAddress(trafficContractAddr))
	addressBook := traffic.NewAddressBook(store)
	protocol := trafficprotocol.New(p2pService, logger, address)
	if err := p2pService.AddProtocol(protocol.Protocol()); err != nil {
		return nil, fmt.Errorf("traffic server :%v", err)
	}
	chequeSigner := chequePkg.NewChequeSigner(signer, chainID)
	trafficService := traffic.New(logger, address, store, trafficChainService, chequeStore, cashOut, p2pService, addressBook, chequeSigner, protocol, chainID, subPub)
	protocol.SetTraffic(trafficService)
	return trafficService, nil
}
