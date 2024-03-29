package oracle

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/settlement/chain"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type ChainOracle struct {
	sync.Mutex
	logger        logging.Logger
	oracle        *Oracle
	chain         *ethclient.Client
	signer        crypto.Signer
	senderAddress common.Address
	chainID       *big.Int
	commonService chain.Common
	subPub        subscribe.SubPub
}

func NewServer(logger logging.Logger, backend *ethclient.Client, chainID *big.Int, address string, signer crypto.Signer, commonService chain.Common, subPub subscribe.SubPub) (chain.Resolver, error) {
	senderAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}
	oracle, err := NewOracle(common.HexToAddress(address), backend)
	if err != nil {
		logger.Errorf("Failed to connect to the Ethereum client: %v", err)
		return &ChainOracle{}, err
	}

	return &ChainOracle{
		logger:        logger,
		oracle:        oracle,
		chain:         backend,
		signer:        signer,
		senderAddress: senderAddress,
		chainID:       chainID,
		commonService: commonService,
		subPub:        subPub,
	}, nil
}

func (ora *ChainOracle) GetCid(_ string) []byte {
	return nil
}

func (ora *ChainOracle) GetNodesFromCid(cid []byte) []boson.Address {

	if ora.chain == nil {
		return make([]boson.Address, 0)
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()
	opts := &bind.CallOpts{Context: ctx}
	overlays, err := ora.oracle.Get(opts, common.BytesToHash(cid))
	overs := make([]boson.Address, 0)
	if err != nil {
		ora.logger.Errorf("Get overlays based on cid : %v", err)
		return overs
	}
	for i := range overlays {
		overs = append(overs, boson.NewAddress(overlays[i][:]))
	}
	return overs
}

func (ora *ChainOracle) GetSourceNodes(_ string) []boson.Address {

	return nil
}

func (ora *ChainOracle) OnStoreMatched(cid boson.Address, dataLen uint64, salt uint64, address boson.Address) {

}

func (ora *ChainOracle) DataStoreFinished(cid boson.Address, dataLen uint64, salt uint64, proof []byte, resCh chan chain.ChainResult) {

}

func (ora *ChainOracle) RegisterCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (hash common.Hash, err error) {
	ora.Lock()
	defer ora.Unlock()
	if ora.chain == nil {
		return common.Hash{}, nil
	}

	defer func() {
		if err == nil {
			ora.commonService.SyncTransaction(chain.ORACLE, rootCid.String(), hash.String())
		}
	}()

	if ora.commonService.IsTransaction() {
		return common.Hash{}, errors.New("existing chain transaction")
	}

	opts, err := ora.getTransactOpts(ctx, gasPrice, minGasPrice)
	if err != nil {
		return common.Hash{}, err
	}
	tract, err := ora.oracle.Set(opts, common.BytesToHash(rootCid.Bytes()), common.BytesToHash(address.Bytes()))
	if err != nil {
		return common.Hash{}, err
	}
	return tract.Hash(), nil
}
func (ora *ChainOracle) RemoveCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (hash common.Hash, err error) {
	ora.Lock()
	defer ora.Unlock()
	if ora.chain == nil {
		return common.Hash{}, nil
	}
	defer func() {
		if err == nil {
			ora.commonService.SyncTransaction(chain.ORACLE, rootCid.String(), hash.String())
		}
	}()
	if ora.commonService.IsTransaction() {
		return common.Hash{}, errors.New("existing chain transaction")
	}
	opts, err := ora.getTransactOpts(ctx, gasPrice, minGasPrice)
	if err != nil {
		return common.Hash{}, err
	}

	tract, err := ora.oracle.Remove(opts, common.BytesToHash(rootCid.Bytes()), common.BytesToHash(address.Bytes()))
	if err != nil {
		return common.Hash{}, err
	}
	return tract.Hash(), nil
}

func (ora *ChainOracle) WaitForReceipt(ctx context.Context, rootCid boson.Address, txHash common.Hash) (receipt *types.Receipt, err error) {
	if ora.chain == nil {
		ora.PublishRegisterStatus(rootCid, 0)
		return nil, nil
	}
	defer func() {
		ora.commonService.UpdateStatus(false)
	}()
	for {
		receipt, err := ora.chain.TransactionReceipt(ctx, txHash)
		if receipt != nil {
			ora.PublishRegisterStatus(rootCid, receipt.Status)
			return receipt, nil
		}
		if err != nil {
			// some node implementations return an error if the transaction is not yet mined
			ora.logger.Tracef("waiting for transaction %x to be mined: %v", txHash, err)
		} else {
			ora.logger.Tracef("waiting for transaction %x to be mined", txHash)
		}

		select {
		case <-ctx.Done():
			ora.PublishRegisterStatus(rootCid, 0)
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func (ora *ChainOracle) GetRegisterState(ctx context.Context, rootCid boson.Address, address boson.Address) (bool, error) {
	if ora.chain == nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.TODO(), 2*time.Second)
	defer cancel()
	opts := &bind.CallOpts{Context: ctx}
	state, err := ora.oracle.OracleIMap(opts, common.BytesToHash(rootCid.Bytes()), common.BytesToHash(address.Bytes()))
	if err != nil {
		return false, err
	}

	return state.Cmp(big.NewInt(0)) != 0, nil
}

func (ora *ChainOracle) getTransactOpts(ctx context.Context, gasPrice, minGasPrice *big.Int) (*bind.TransactOpts, error) {
	if ora.chain == nil {
		return nil, nil
	}
	chainNonce, err := ora.chain.PendingNonceAt(ctx, ora.senderAddress)
	if err != nil {
		return nil, err
	}

	if gasPrice == nil || gasPrice.Cmp(big.NewInt(0)) <= 0 {
		gasPrice, err = ora.chain.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
		if minGasPrice != nil && gasPrice.Cmp(minGasPrice) < 0 {
			gasPrice = minGasPrice
		}
	}
	opts := &bind.TransactOpts{
		From: ora.senderAddress,
		Signer: func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
			if address != ora.senderAddress {
				return nil, bind.ErrNotAuthorized
			}
			return ora.signer.SignTx(tx, ora.chainID)
		},
		GasLimit: 1000000,
		GasPrice: gasPrice,
		Context:  ctx,
		Nonce:    new(big.Int).SetUint64(chainNonce),
	}
	return opts, nil
}

func (ora *ChainOracle) SubscribeRegisterStatus(notifier *rpc.Notifier, sub *rpc.Subscription, rootCids []boson.Address) {
	iNotifier := subscribe.NewNotifier(notifier, sub)
	for i := 0; i < len(rootCids); i++ {
		_ = ora.subPub.Subscribe(iNotifier, "oracle", "registerStatus", rootCids[i].String())
	}
}

type RegisterStatus struct {
	RootCid boson.Address `json:"rootCid"`
	Status  bool          `json:"status"`
}

func (ora *ChainOracle) PublishRegisterStatus(rootCid boson.Address, status uint64) {
	b := false
	if status == 1 {
		b = true
	}
	_ = ora.subPub.Publish("oracle", "registerStatus", rootCid.String(), RegisterStatus{
		RootCid: rootCid,
		Status:  b,
	})
}
