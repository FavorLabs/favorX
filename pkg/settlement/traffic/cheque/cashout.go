package cheque

import (
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// CashoutService is the service responsible for managing cashout actions
type CashoutService interface {
	// CashCheque
	CashCheque(cid, peer boson.Address) (types.Hash, error)
	// WaitForReceipt
	// WaitForReceipt(ctx context.Context, ctxHash types.Hash) (uint64, error)
}

type cashoutService struct {
	store          storage.StateStorer
	trafficService *chain.SubChainClient
	chequeStore    ChequeStore
}

// NewCashoutService creates a new CashoutService
func NewCashoutService(store storage.StateStorer, trafficService *chain.SubChainClient, chequeStore ChequeStore) CashoutService {

	return &cashoutService{
		store:          store,
		trafficService: trafficService,
		chequeStore:    chequeStore,
	}
}

// CashCheque
func (s *cashoutService) CashCheque(cid, peer boson.Address) (types.Hash, error) {
	tx, err := s.trafficService.Traffic.CashChequeBeneficiary(cid, peer)
	if err != nil {
		return types.Hash{}, err
	}
	return tx, nil
}

// func (s *cashoutService) WaitForReceipt(ctx context.Context, ctxHash types.Hash) (uint64, error) {
//	receipt, err := s.transactionService.WaitForReceipt(ctx, ctxHash)
//
//	return receipt.Status, err
// }
