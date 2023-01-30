package api

import (
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"math/big"
	"net/http"
	"sort"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
)

type trafficInfo struct {
	Balance          *big.Int `json:"balance"`
	AvailableBalance *big.Int `json:"availableBalance"`
	TotalSendTraffic *big.Int `json:"totalSendTraffic"`
	ReceivedTraffic  *big.Int `json:"receivedTraffic"`
}

type trafficCheque struct {
	Peer                boson.Address `json:"peer"`
	OutstandingTraffic  *big.Int      `json:"outstandingTraffic"`
	SentSettlements     *big.Int      `json:"sentSettlements"`
	ReceivedSettlements *big.Int      `json:"receivedSettlements"`
	Total               *big.Int      `json:"total"`
	UnCashed            *big.Int      `json:"unCashed"`
	Status              int           `json:"status"`
}

func (s *server) trafficInfo(w http.ResponseWriter, r *http.Request) {
	tra, err := s.traffic.TrafficInfo()
	if err != nil {
		s.logger.Errorf("Api-trafficInfo Failed to get traffic information: %v", err)
		jsonhttp.InternalServerError(w, err)
		return
	}

	var traffic trafficInfo
	traffic.Balance = tra.Balance
	traffic.AvailableBalance = tra.AvailableBalance
	traffic.TotalSendTraffic = tra.TotalSendTraffic
	traffic.ReceivedTraffic = tra.ReceivedTraffic
	jsonhttp.OK(w, traffic)
}

func (s *server) address(w http.ResponseWriter, r *http.Request) {
	address := s.traffic.Address()
	jsonhttp.OK(w, struct {
		References types.AccountID `json:"references"`
	}{
		References: address,
	})
}

func (s *server) trafficCheques(w http.ResponseWriter, r *http.Request) {
	var chequeList []*trafficCheque
	list, err := s.traffic.TrafficCheques()
	if err != nil {
		s.logger.Error("Api trafficInfo: Failed to get traffic information: %v", err)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	for _, v := range list {
		cheque := &trafficCheque{
			Peer:                v.Peer,
			OutstandingTraffic:  v.OutstandingTraffic,
			SentSettlements:     v.SentSettlements,
			ReceivedSettlements: v.ReceivedSettlements,
			Total:               v.Total,
			UnCashed:            v.Uncashed,
			Status:              v.Status,
		}
		chequeList = append(chequeList, cheque)
	}

	sort.Slice(chequeList, func(i, j int) bool {
		return chequeList[i].UnCashed.Cmp(chequeList[j].UnCashed) > 0
	})
	jsonhttp.OK(w, chequeList)
}

func (s *server) cashCheque(w http.ResponseWriter, r *http.Request) {
	hash, err := s.traffic.CashCheque(s.overlay)
	if err != nil {
		s.logger.Errorf("api cashCheque: query failed %v", err)
		jsonhttp.NotFound(w, err)
		return
	}

	type out struct {
		Hash types.Hash `json:"hash"`
	}
	jsonhttp.OK(w, out{Hash: hash})
}
