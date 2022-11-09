package chain

import (
	"context"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type ChainResult struct {
	// success bool
	TxHash []byte
	// reason  string
}

// TxRequest describes a request for a transaction that can be executed.
type TxRequest struct {
	To       *common.Address // recipient of the transaction
	Data     []byte          // transaction data
	GasPrice *big.Int        // gas price or nil if suggested gas price should be used
	GasLimit uint64          // gas limit or 0 if it should be estimated
	Value    *big.Int        // amount of wei to send
}

type AllRequest struct {
	Method string
	Params []interface{}
}

type AllResponse struct {
	Result interface{}
}

type TxInfo struct {
	Type   TransactionType `json:"type"`
	Value  string          `json:"value"`
	TxHash string          `json:"txHash"`
}

type Traffic interface {

	// 	TransferredAddress opts todo
	TransferredAddress(address common.Address) ([]common.Address, error)

	RetrievedAddress(address common.Address) ([]common.Address, error)

	BalanceOf(account common.Address) (*big.Int, error)

	RetrievedTotal(address common.Address) (*big.Int, error)

	TransferredTotal(address common.Address) (*big.Int, error)

	TransAmount(beneficiary, recipient common.Address) (*big.Int, error)

	CashChequeBeneficiary(ctx context.Context, peer boson.Address, beneficiary, recipient common.Address, cumulativePayout *big.Int, signature []byte) (*types.Transaction, error)
}

// Service is the service to send transactions. It takes care of gas price, gas
// limit and nonce management.
type Transaction interface {
	// Send creates a transaction based on the request and sends it.
	Send(ctx context.Context, request *TxRequest) (txHash common.Hash, err error)
	// Call simulate a transaction based on the request.
	Call(ctx context.Context, request *TxRequest) (result []byte, err error)
	// WaitForReceipt waits until either the transaction with the given hash has been mined or the context is cancelled.
	WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error)

	NextNonce(ctx context.Context) (uint64, error)
}

type TransactionType string

const (
	ORACLE  TransactionType = "oracle"
	TRAFFIC TransactionType = "traffic"
)

type Common interface {
	All(ctx context.Context, request *AllRequest) (*AllResponse, error)

	SyncTransaction(t TransactionType, value, txHash string)

	IsTransaction() bool

	UpdateStatus(status bool)

	GetTransaction() *TxInfo
}
