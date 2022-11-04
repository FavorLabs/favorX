package proxy

import (
	"context"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/ethereum/go-ethereum/common"
)

type Interface interface {
	All(ctx context.Context, request *AllRequest) (*AllResponse, error)

	SyncTransaction(t TransactionType, value, txHash string)

	IsTransaction() bool

	UpdateStatus(status bool)

	GetTransaction() *TxInfo
}

// storage exposes methods for querying storage
type service struct {
	client *base.SubstrateAPI
}

func (s *service) All(ctx context.Context, request *AllRequest) (*AllResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (s *service) SyncTransaction(t TransactionType, value, txHash string) {
	// TODO implement me
	panic("implement me")
}

func (s *service) IsTransaction() bool {
	// TODO implement me
	panic("implement me")
}

func (s *service) UpdateStatus(status bool) {
	// TODO implement me
	panic("implement me")
}

func (s *service) GetTransaction() *TxInfo {
	// TODO implement me
	panic("implement me")
}

// New creates a new oracle struct
func New(c *base.SubstrateAPI) Interface {
	return &service{client: c}
}

type TransactionType string

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

type TxCommonRequest struct {
	From     string `json:"from"`
	Nonce    string `json:"nonce"`
	To       string `json:"to"`       // recipient of the transaction
	Data     string `json:"data"`     // transaction data
	GasPrice string `json:"gasPrice"` // gas price or nil if suggested gas price should be used
	GasLimit string `json:"gasLimit"` // gas limit or 0 if it should be estimated
	Value    string `json:"value"`    // amount of wei to send
}
