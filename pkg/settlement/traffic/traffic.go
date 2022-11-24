package traffic

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/cac"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/traffic"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/settlement"
	chequePkg "github.com/FavorLabs/favorX/pkg/settlement/traffic/cheque"
	"github.com/FavorLabs/favorX/pkg/settlement/traffic/trafficprotocol"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
)

var (
	ErrUnknownBeneficary = errors.New("unknown beneficiary for peer")
	ErrInsufficientFunds = errors.New("insufficient token balance")
)

type SendChequeFunc func(cheque *chequePkg.SignedCheque) error

type Traffic struct {
	sync.Mutex
	trafficPeerBalance    *big.Int
	retrieveChainTraffic  *big.Int
	transferChainTraffic  *big.Int
	retrieveChequeTraffic *big.Int
	transferChequeTraffic *big.Int
	retrieveTraffic       *big.Int
	transferTraffic       *big.Int
	status                CashStatus
}

type TrafficPeer struct {
	trafficLock  sync.Mutex
	trafficPeers map[string]*Traffic
	balance      *big.Int
	totalPaidOut *big.Int
}

type TrafficCheque struct {
	Peer                boson.Address `json:"peer"`
	OutstandingTraffic  *big.Int      `json:"outstandingTraffic"`
	SentSettlements     *big.Int      `json:"sentSettlements"`
	ReceivedSettlements *big.Int      `json:"receivedSettlements"`
	Total               *big.Int      `json:"total"`
	Uncashed            *big.Int      `json:"unCashed"`
	Status              CashStatus    `json:"status"`
}

type CashStatus = int

const (
	UnOperation CashStatus = iota
	Operation
)

type cashCheque struct {
	cid boson.Address
}

type CashOutStatus struct {
	Status bool `json:"status"`
}

type TrafficInfo struct {
	Balance          *big.Int `json:"balance"`
	AvailableBalance *big.Int `json:"availableBalance"`
	TotalSendTraffic *big.Int `json:"totalSendTraffic"`
	ReceivedTraffic  *big.Int `json:"receivedTraffic"`
}

type ApiInterface interface {
	LastSentCheque(peer boson.Address) (*chequePkg.Cheque, error)

	LastReceivedCheque(peer boson.Address) (*chequePkg.SignedCheque, error)

	CashCheque(peer boson.Address) (types.Hash, error)

	TrafficCheques() ([]*TrafficCheque, error)

	Address() types.AccountID

	TrafficInfo() (*TrafficInfo, error)

	TrafficInit() error

	API() rpc.API
}

const (
	trafficChainRefreshDuration = 24 * time.Hour
	chequesCount                = 100
)

type Service struct {
	logger              logging.Logger
	chainAddress        types.AccountID
	stateStore          storage.StateStorer
	localStore          storage.Storer
	metrics             metrics
	chequeStore         chequePkg.ChequeStore
	cashout             chequePkg.CashoutService
	trafficChainService traffic.Interface
	p2pService          p2p.Service
	peersLock           sync.Mutex
	trafficPeers        TrafficPeer
	addressBook         Addressbook
	chequeSigner        chequePkg.ChequeSigner
	protocol            trafficprotocol.Interface
	notifyPaymentFunc   settlement.NotifyPaymentFunc
	subPub              subscribe.SubPub
	//txHash:beneficiary
	cashChequeChan chan cashCheque
}

func New(logger logging.Logger, chainAddress types.AccountID, store storage.StateStorer, localStore storage.Storer,
	trafficChainService traffic.Interface, chequeStore chequePkg.ChequeStore, cashout chequePkg.CashoutService,
	p2pService p2p.Service, addressBook Addressbook, chequeSigner chequePkg.ChequeSigner,
	protocol trafficprotocol.Interface, subPub subscribe.SubPub) *Service {

	service := &Service{
		logger:              logger,
		stateStore:          store,
		localStore:          localStore,
		chainAddress:        chainAddress,
		trafficChainService: trafficChainService,
		metrics:             newMetrics(),
		chequeStore:         chequeStore,
		cashout:             cashout,
		p2pService:          p2pService,
		addressBook:         addressBook,
		chequeSigner:        chequeSigner,
		protocol:            protocol,
		trafficPeers: TrafficPeer{
			trafficPeers: make(map[string]*Traffic),
			balance:      big.NewInt(0),
			totalPaidOut: big.NewInt(0),
		},
		subPub:         subPub,
		cashChequeChan: make(chan cashCheque, 5),
	}
	service.triggerRefreshInit()
	service.cashChequeReceiptUpdate()
	return service
}

func (s *Service) Init() error {
	err := s.trafficInit()
	if err != nil {
		return err
	}

	return s.addressBook.InitAddressBook()
}

func (s *Service) triggerRefreshInit() {
	ticker := time.NewTicker(trafficChainRefreshDuration)
	go func(t *time.Ticker) {
		for {
			<-t.C
			err := s.trafficInit()
			if err != nil {
				s.logger.Errorf("traffic-InitChain: %w", err)
				//os.Exit(1)
			}
		}
	}(ticker)
}

func newTraffic() *Traffic {
	return &Traffic{
		trafficPeerBalance:    big.NewInt(0),
		retrieveChainTraffic:  big.NewInt(0),
		transferChainTraffic:  big.NewInt(0),
		retrieveChequeTraffic: big.NewInt(0),
		transferChequeTraffic: big.NewInt(0),
		retrieveTraffic:       big.NewInt(0),
		transferTraffic:       big.NewInt(0),
		status:                UnOperation,
	}
}
func NewTrafficInfo() *TrafficInfo {
	return &TrafficInfo{
		Balance:          big.NewInt(0),
		AvailableBalance: big.NewInt(0),
		TotalSendTraffic: big.NewInt(0),
		ReceivedTraffic:  big.NewInt(0),
	}
}

func (s *Service) getTraffic(peer types.AccountID) (traffic *Traffic) {
	s.trafficPeers.trafficLock.Lock()
	defer s.trafficPeers.trafficLock.Unlock()
	key := peer.ToHexString()
	traffic = s.trafficPeers.trafficPeers[key]
	if traffic == nil {
		traffic = newTraffic()
		s.trafficPeers.trafficPeers[key] = traffic
	}
	return
}

func (s *Service) getAllAddress(addresses map[types.AccountID]struct{}) (map[types.AccountID]Traffic, error) {
	chanResp := make(map[types.AccountID]Traffic)

	for k := range addresses {
		if _, ok := chanResp[k]; !ok {
			chanResp[k] = Traffic{}
		}
	}

	//retrieveList, err := s.trafficChainService.RetrievedAddress(s.chainAddress)
	//if err != nil {
	//	return chanResp, err
	//}
	//for _, v := range retrieveList {
	//	if _, ok := chanResp[v]; !ok {
	//		chanResp[v] = Traffic{}
	//	}
	//}
	transferList, err := s.trafficChainService.TransferredAddress(s.chainAddress)
	if err != nil {
		return chanResp, err
	}
	for _, v := range transferList {
		if _, ok := chanResp[v]; !ok {
			chanResp[v] = Traffic{}
		}
	}

	return chanResp, err
}

func (s *Service) trafficInit() error {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()
	lastCheques, err := s.chequeStore.LastSendCheques()
	if err != nil {
		s.logger.Errorf("Traffic failed to obtain local check information.")
		return err
	}

	lastTransCheques, err := s.chequeStore.LastReceivedCheques()
	if err != nil {
		s.logger.Errorf("Traffic failed to obtain local check information.")
		return err
	}

	allRetrieveTransfer, err := s.chequeStore.GetAllRetrieveTransferAddresses()
	if err != nil {
		s.logger.Errorf("Traffic failed to obtain local check information.")
		return err
	}

	addressList, err := s.getAllAddress(allRetrieveTransfer)
	if err != nil {
		return fmt.Errorf("traffic: Failed to get chain node information:%v ", err)
	}

	err = s.replaceTraffic(addressList, lastCheques, lastTransCheques)
	if err != nil {
		return fmt.Errorf("traffic: Update of local traffic data failed. ")
	}

	//transferTotal, err := s.trafficChainService.TransferredTotal(k)
	balance, err := s.trafficChainService.BalanceOf(s.chainAddress)
	if err != nil {
		return fmt.Errorf("failed to get the chain balance")
	}
	s.trafficPeers.balance = balance

	paiOut, err := s.trafficChainService.TransferredTotal(s.chainAddress)
	if err != nil {
		return fmt.Errorf("failed to get the chain totalPaidOut")
	}
	s.trafficPeers.totalPaidOut = paiOut
	return nil
}
func (s *Service) replaceTraffic(addressList map[types.AccountID]Traffic, lastCheques map[types.AccountID]*chequePkg.Cheque, lastTransCheques map[types.AccountID]*chequePkg.SignedCheque) error {

	s.trafficPeers.totalPaidOut = new(big.Int).SetInt64(0)
	workload := make(chan struct{}, 50) // limit goroutine number
	waitGroup := new(sync.WaitGroup)
	for key := range addressList {
		workload <- struct{}{}
		waitGroup.Add(1)
		go func(address types.AccountID, workload chan struct{}, waitGroup *sync.WaitGroup) {
			defer func() {
				<-workload
				waitGroup.Done()
			}()
			err := s.trafficPeerChainUpdate(address, s.chainAddress)
			if err != nil {
				s.logger.Errorf("traffic: getChainTraffic %v", err.Error())
			}
			err = s.trafficPeerChequeUpdate(address, lastCheques, lastTransCheques)
			if err != nil {
				s.logger.Errorf("traffic: replaceTraffic %v", err.Error())
			}
		}(key, workload, waitGroup)
	}
	waitGroup.Wait()
	return nil
}

func (s *Service) trafficPeerChainUpdate(peerAddress, chainAddress types.AccountID) error {
	traffic := s.getTraffic(peerAddress)
	traffic.Lock()
	defer traffic.Unlock()
	transferTotal, err := s.trafficChainService.TransAmount(peerAddress, chainAddress)
	if err != nil {
		transferTotal, err = s.chequeStore.GetChainTransferTraffic(peerAddress)
		if err != nil {
			return err
		}
	} else {
		err = s.chequeStore.PutChainTransferTraffic(peerAddress, transferTotal)
		if err != nil {
			return err
		}
	}
	retrievedTotal, err := s.trafficChainService.TransAmount(chainAddress, peerAddress)
	if err != nil {
		retrievedTotal, err = s.chequeStore.GetChainRetrieveTraffic(peerAddress)
		if err != nil {
			return err
		}
	} else {
		err = s.chequeStore.PutChainRetrieveTraffic(peerAddress, retrievedTotal)
		if err != nil {
			return err
		}
	}
	traffic.retrieveChainTraffic = retrievedTotal
	traffic.transferChainTraffic = transferTotal
	return nil
}

func (s *Service) trafficPeerChequeUpdate(peerAddress types.AccountID, lastCheques map[types.AccountID]*chequePkg.Cheque, lastTransCheques map[types.AccountID]*chequePkg.SignedCheque) error {

	traffic := s.getTraffic(peerAddress)
	traffic.Lock()
	defer traffic.Unlock()
	traffic.retrieveChequeTraffic = traffic.retrieveChainTraffic
	traffic.retrieveTraffic = traffic.retrieveChainTraffic
	traffic.transferChequeTraffic = traffic.transferChainTraffic
	traffic.transferTraffic = traffic.transferChainTraffic
	if cq, ok := lastCheques[peerAddress]; ok {
		traffic.retrieveTraffic = s.maxBigint(traffic.retrieveTraffic, cq.CumulativePayout)
		traffic.retrieveChequeTraffic = s.maxBigint(traffic.retrieveChequeTraffic, cq.CumulativePayout)
	}

	if cq, ok := lastTransCheques[peerAddress]; ok {
		traffic.transferTraffic = s.maxBigint(traffic.transferTraffic, cq.CumulativePayout)
		traffic.transferChequeTraffic = s.maxBigint(traffic.transferChequeTraffic, cq.CumulativePayout)
	}

	retrieve, err := s.chequeStore.GetRetrieveTraffic(peerAddress)
	if err != nil {
		return err
	}
	traffic.retrieveTraffic = s.maxBigint(traffic.retrieveTraffic, retrieve)

	transfer, err := s.chequeStore.GetTransferTraffic(peerAddress)
	if err != nil {
		return err
	}
	traffic.transferTraffic = s.maxBigint(traffic.transferTraffic, transfer)
	return nil
}

// Returns the maximum value
func (s *Service) maxBigint(a *big.Int, b *big.Int) *big.Int {
	if a.Cmp(b) < 0 {
		return b
	} else {
		return a
	}
}

// LastSentCheque returns the last sent cheque for the peer
func (s *Service) LastSentCheque(peer boson.Address) (*chequePkg.Cheque, error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)

	if !known {
		return nil, chequePkg.ErrNoCheque
	}
	return s.chequeStore.LastSendCheque(chainAddress)
}

// LastReceivedCheque returns the list of last received cheques for all peers
func (s *Service) LastReceivedCheque(peer boson.Address) (*chequePkg.SignedCheque, error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)

	if !known {
		return &chequePkg.SignedCheque{}, nil
	}
	return s.chequeStore.LastReceivedCheque(chainAddress)
}

// CashCheque sends a cashing transaction for the last cheque of the peer
func (s *Service) CashCheque(peer boson.Address) (types.Hash, error) {

	cid, err := s.countCheque()
	if err != nil {
		return types.Hash{}, err
	}
	tx, err := s.cashout.CashCheque(cid, peer)
	if err != nil {
		return tx, err
	}
	s.cashChequeChan <- cashCheque{
		cid: cid,
	}
	return tx, err
}

func (s *Service) countCheque() (boson.Address, error) {
	tp := s.trafficPeers.trafficPeers
	cheques := make([]chequePkg.ChainSignedCheque, 0, len(tp))
	for chainAddress, t := range tp {
		if t.transferChequeTraffic.Cmp(t.transferChainTraffic) == 1 {
			accountId, _ := types.NewAccountIDFromHexString(chainAddress)
			cheque, err := s.chequeStore.LastReceivedCheque(*accountId)
			if err != nil {
				continue
			}
			if len(cheques) > chequesCount {
				break
			}
			chainCheque := chequePkg.ChainSignedCheque{
				SignedCheque:     types.NewSignature(cheque.Signature),
				Recipient:        cheque.Recipient,
				Beneficiary:      cheque.Beneficiary,
				CumulativePayout: types.NewU128(*cheque.CumulativePayout),
			}
			cheques = append(cheques, chainCheque)
		}
	}
	chunk, err := codec.Encode(cheques)
	if err != nil {
		s.logger.Errorf("encode err:%w", err)
		return boson.ZeroAddress, err
	}
	c, err := cac.New(chunk)
	if err != nil {
		s.logger.Errorf("new chunk err:%w", err)
		return boson.ZeroAddress, err
	}
	_, err = s.localStore.Put(context.TODO(), storage.ModePutChain, c)
	if err != nil {
		s.logger.Errorf("store chunk err%w", err)
		return boson.ZeroAddress, err
	}
	return c.Address(), nil
}

func (s *Service) Address() types.AccountID {
	return s.chainAddress
}

func (s *Service) TrafficInfo() (*TrafficInfo, error) {
	respTraffic := NewTrafficInfo()
	s.trafficPeers.trafficLock.Lock()
	defer s.trafficPeers.trafficLock.Unlock()
	cashed := big.NewInt(0)
	transfer := big.NewInt(0)
	for _, traffic := range s.trafficPeers.trafficPeers {
		cashed = new(big.Int).Add(cashed, traffic.retrieveChainTraffic)
		transfer = new(big.Int).Add(transfer, traffic.retrieveChequeTraffic)
		respTraffic.TotalSendTraffic = new(big.Int).Add(respTraffic.TotalSendTraffic, traffic.retrieveChequeTraffic)
		respTraffic.ReceivedTraffic = new(big.Int).Add(respTraffic.ReceivedTraffic, traffic.transferChequeTraffic)
	}

	respTraffic.Balance = s.trafficPeers.balance
	respTraffic.AvailableBalance = new(big.Int).Add(respTraffic.Balance, new(big.Int).Sub(cashed, transfer))

	return respTraffic, nil
}

func (s *Service) TrafficCheques() ([]*TrafficCheque, error) {
	s.trafficPeers.trafficLock.Lock()
	defer s.trafficPeers.trafficLock.Unlock()
	var trafficCheques []*TrafficCheque
	for chainAddress, traffic := range s.trafficPeers.trafficPeers {
		accountId, _ := types.NewAccountIDFromHexString(chainAddress)
		peer, known := s.addressBook.BeneficiaryPeer(*accountId)
		if known {
			trans := new(big.Int).Sub(traffic.transferTraffic, traffic.transferChequeTraffic)
			retrieve := new(big.Int).Sub(traffic.retrieveTraffic, traffic.retrieveChequeTraffic)
			trafficCheque := &TrafficCheque{
				Peer:                peer,
				OutstandingTraffic:  new(big.Int).Sub(trans, retrieve),
				SentSettlements:     traffic.retrieveChequeTraffic,
				ReceivedSettlements: traffic.transferChequeTraffic,
				Total:               new(big.Int).Sub(traffic.transferTraffic, traffic.retrieveTraffic),
				Uncashed:            new(big.Int).Sub(traffic.transferChequeTraffic, traffic.transferChainTraffic),
				Status:              traffic.status,
			}
			if trafficCheque.OutstandingTraffic.Cmp(big.NewInt(0)) == 0 && trafficCheque.SentSettlements.Cmp(big.NewInt(0)) == 0 && trafficCheque.ReceivedSettlements.Cmp(big.NewInt(0)) == 0 {
				continue
			}
			trafficCheques = append(trafficCheques, trafficCheque)
		} else {
			s.logger.Errorf("traffic: The method TrafficCheques failed to ChainAddress has no corresponding peer address.")
		}
	}

	return trafficCheques, nil
}

func (s *Service) Pay(ctx context.Context, peer boson.Address, paymentThreshold *big.Int) error {

	recipient, known := s.addressBook.Beneficiary(peer)
	if !known {
		s.logger.Warningf("disconnecting non-traffic peer %v", peer)
		err := s.p2pService.Disconnect(peer, "no recipient found")
		if err != nil {
			return err
		}
		return ErrUnknownBeneficary
	}
	balance := s.retrieveTraffic(recipient)
	traffic := s.getTraffic(recipient)
	traffic.Lock()
	defer traffic.Unlock()
	if balance.Cmp(paymentThreshold) >= 0 {
		if err := s.issue(ctx, peer, recipient, s.chainAddress, balance, traffic); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) issue(ctx context.Context, peer boson.Address, recipient, beneficiary types.AccountID, balance *big.Int, traffic *Traffic) error {

	defer func() {
		_ = s.notifyPaymentFunc(peer, balance)
	}()

	available, err := s.AvailableBalance()
	if err != nil {
		return err
	}

	if available.Cmp(balance) < 0 {
		return ErrInsufficientFunds
	}

	cumulativePayout := traffic.retrieveChequeTraffic
	// increase cumulativePayout by amount
	cumulativePayout = cumulativePayout.Add(cumulativePayout, balance)
	// create and sign the new cheque
	c := chequePkg.Cheque{
		Recipient:        recipient,
		Beneficiary:      beneficiary,
		CumulativePayout: cumulativePayout,
	}
	sin, err := s.chequeSigner.Sign(&c)
	if err != nil {
		return err
	}
	signedCheque := &chequePkg.SignedCheque{
		Cheque:    c,
		Signature: sin,
	}
	if err := s.protocol.EmitCheque(ctx, peer, signedCheque); err != nil {
		return err
	}
	return s.putSendCheque(ctx, &c, recipient, traffic)
}

func (s *Service) putSendCheque(ctx context.Context, cheque *chequePkg.Cheque, recipient types.AccountID, traffic *Traffic) error {
	traffic.retrieveChequeTraffic = cheque.CumulativePayout
	traffic.retrieveTraffic = s.maxBigint(traffic.retrieveTraffic, cheque.CumulativePayout)
	go s.PublishTrafficCheque(recipient)
	return s.chequeStore.PutSendCheque(ctx, cheque, recipient)
}

// TotalSent returns the total amount sent to a peer
func (s *Service) TotalSent(peer boson.Address) (totalSent *big.Int, err error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)
	totalSent = big.NewInt(0)
	if !known {
		return totalSent, chequePkg.ErrNoCheque
	}

	traffic := s.getTraffic(chainAddress)
	traffic.Lock()
	defer traffic.Unlock()
	totalSent = traffic.transferTraffic
	return
}

// TotalReceived returns the total amount received from a peer
func (s *Service) TotalReceived(peer boson.Address) (totalReceived *big.Int, err error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)
	totalReceived = big.NewInt(0)

	if !known {
		return totalReceived, chequePkg.ErrNoCheque
	}

	traffic := s.getTraffic(chainAddress)
	traffic.Lock()
	defer traffic.Unlock()
	totalReceived = traffic.retrieveTraffic
	return
}

func (s *Service) TransferTraffic(peer boson.Address) (*big.Int, error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)

	if !known {
		return big.NewInt(0), chequePkg.ErrNoCheque
	}

	traffic := s.getTraffic(chainAddress)
	traffic.Lock()
	defer traffic.Unlock()
	return new(big.Int).Sub(traffic.transferTraffic, traffic.transferChequeTraffic), nil
}

func (s *Service) RetrieveTraffic(peer boson.Address) (traffic *big.Int, err error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)

	if !known {
		return big.NewInt(0), chequePkg.ErrNoCheque
	}
	return s.retrieveTraffic(chainAddress), nil
}

func (s *Service) retrieveTraffic(peer types.AccountID) *big.Int {
	traffic := s.getTraffic(peer)
	traffic.Lock()
	defer traffic.Unlock()
	return new(big.Int).Sub(traffic.retrieveTraffic, traffic.retrieveChequeTraffic)
}

func (s *Service) PutRetrieveTraffic(peer boson.Address, traffic *big.Int) error {
	chainAddress, known := s.addressBook.Beneficiary(peer)
	if !known {
		return chequePkg.ErrNoCheque
	}

	chainTraffic := s.getTraffic(chainAddress)
	chainTraffic.Lock()
	chainTraffic.retrieveTraffic = new(big.Int).Add(chainTraffic.retrieveTraffic, traffic)
	chainTraffic.Unlock()
	go s.PublishHeader()
	go s.PublishTrafficCheque(chainAddress)
	return s.chequeStore.PutRetrieveTraffic(chainAddress, chainTraffic.retrieveTraffic)
}

func (s *Service) PutTransferTraffic(peer boson.Address, traffic *big.Int) error {
	chainAddress, known := s.addressBook.Beneficiary(peer)

	if !known {
		return chequePkg.ErrNoCheque
	}
	localTraffic := s.getTraffic(chainAddress)
	localTraffic.Lock()
	localTraffic.transferTraffic = new(big.Int).Add(localTraffic.transferTraffic, traffic)
	localTraffic.Unlock()
	go s.PublishHeader()
	go s.PublishTrafficCheque(chainAddress)
	return s.chequeStore.PutTransferTraffic(chainAddress, localTraffic.transferTraffic)
}

// AvailableBalance Get actual available balance
func (s *Service) AvailableBalance() (*big.Int, error) {
	s.trafficPeers.trafficLock.Lock()
	defer s.trafficPeers.trafficLock.Unlock()
	cashed := big.NewInt(0)
	transfer := big.NewInt(0)
	for _, traffic := range s.trafficPeers.trafficPeers {
		cashed = new(big.Int).Add(cashed, traffic.retrieveChainTraffic)
		transfer = new(big.Int).Add(transfer, traffic.retrieveTraffic)
	}

	return new(big.Int).Add(s.trafficPeers.balance, new(big.Int).Sub(cashed, transfer)), nil
}

func (s *Service) Handshake(peer boson.Address, recipient types.AccountID, signedCheque chequePkg.SignedCheque) error {
	recipientLocal, known := s.addressBook.Beneficiary(peer)

	if !known {
		s.logger.Tracef("initial swap handshake peer: %v recipient: %x", peer, recipient)
		_, known := s.addressBook.BeneficiaryPeer(recipient)
		if known {
			return fmt.Errorf("overlay is exists")
		}
		if err := s.addressBook.PutBeneficiary(peer, recipient); err != nil {
			return err
		}
	} else {
		if signedCheque.Signature != nil && signedCheque.Recipient != recipientLocal {
			return fmt.Errorf("error in verifying check receiver ")
		}
	}

	err := s.UpdatePeerBalance(recipientLocal)
	if err != nil {
		return err
	}

	if signedCheque.Signature == nil {
		return nil
	}

	isUser, err := s.chequeStore.VerifyCheque(&signedCheque)
	if err != nil {
		return err
	}
	if isUser != s.chainAddress {
		return chequePkg.ErrChequeInvalid
	}

	cheque, err := s.chequeStore.LastSendCheque(recipient)
	if err != nil && err != chequePkg.ErrNoCheque {
		return err
	}
	if err == chequePkg.ErrNoCheque {
		cheque = &chequePkg.Cheque{
			CumulativePayout: new(big.Int).SetInt64(0),
		}

	}

	if signedCheque.CumulativePayout.Cmp(cheque.CumulativePayout) > 0 {
		traffic := s.getTraffic(recipient)
		return s.putSendCheque(context.Background(), &signedCheque.Cheque, recipient, traffic)
	}

	return nil
}

func (s *Service) UpdatePeerBalance(addr types.AccountID) error {
	balance, err := s.trafficChainService.BalanceOf(addr)
	if err != nil {
		return err
	}
	traffic := s.getTraffic(addr)
	traffic.Lock()
	defer traffic.Unlock()
	traffic.trafficPeerBalance = balance
	return nil
}

func (s *Service) ReceiveCheque(ctx context.Context, peer boson.Address, cheque *chequePkg.SignedCheque) error {

	chainAddress, known := s.addressBook.Beneficiary(peer)

	if !known {
		return errors.New("account information error")
	}

	if cheque.Beneficiary != chainAddress && cheque.Recipient != s.Address() {
		return errors.New("account information error ")
	}

	t := s.getTraffic(chainAddress)
	t.Lock()
	defer t.Unlock()
	_, err := s.chequeStore.ReceiveCheque(ctx, cheque)
	if err != nil {
		return err
	}
	t.transferChequeTraffic = cheque.CumulativePayout
	go s.PublishTrafficCheque(chainAddress)
	return nil
}

func (s *Service) SetNotifyPaymentFunc(notifyPaymentFunc settlement.NotifyPaymentFunc) {
	s.notifyPaymentFunc = notifyPaymentFunc
}

func (s *Service) GetPeerBalance(peer boson.Address) (*big.Int, error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)
	if !known {
		return big.NewInt(0), chequePkg.ErrNoCheque
	}
	traffic := s.getTraffic(chainAddress)
	traffic.Lock()
	defer traffic.Unlock()
	return traffic.trafficPeerBalance, nil
}

func (s *Service) GetUnPaidBalance(peer boson.Address) (*big.Int, error) {
	chainAddress, known := s.addressBook.Beneficiary(peer)
	if !known {
		return big.NewInt(0), chequePkg.ErrNoCheque
	}

	traffic := s.getTraffic(chainAddress)
	traffic.Lock()
	defer traffic.Unlock()
	transferTraffic := new(big.Int).Sub(traffic.transferTraffic, traffic.transferChainTraffic)
	return transferTraffic, nil
}

func (s *Service) TrafficInit() error {
	return s.Init()
}

func (s *Service) cashChequeReceiptUpdate() {

	go func() {
		cashUpdate := func(beneficiary types.AccountID) error {
			balance, err := s.trafficChainService.BalanceOf(s.chainAddress)
			if err != nil {
				return fmt.Errorf("failed to get the chain balance")
			}
			s.peersLock.Lock()
			s.trafficPeers.balance = balance
			s.peersLock.Unlock()
			err = s.trafficPeerChainUpdate(beneficiary, s.chainAddress)
			if err != nil {
				return err
			}
			return s.UpdatePeerBalance(beneficiary)
		}

		for cashInfo := range s.cashChequeChan {
			cheques, err := s.localStore.Get(context.TODO(), storage.ModeGetChain, cashInfo.cid, 0)
			if err != nil {
				go s.PublishCashOut(CashOutStatus{Status: false})
				continue
			}
			chainCheques := make([]chequePkg.ChainSignedCheque, 0, chequesCount)
			chunk := cheques.Data()[boson.SpanSize:]
			err = codec.Decode(chunk, &chainCheques)
			if err != nil {
				go s.PublishCashOut(CashOutStatus{Status: false})
				continue
			}
			for _, cheque := range chainCheques {
				err = s.chequeStore.PutChainTransferTraffic(cheque.Beneficiary, cheque.CumulativePayout.Int)
				if err != nil {
					s.logger.Errorf("traffic:chainTransferTrafficUpdate - %v ", err.Error())
				}
				err = cashUpdate(cheque.Beneficiary)
				if err != nil {
					s.logger.Errorf("traffic:cashChequeReceiptUpdate - %v ", err.Error())
				}
				go s.PublishHeader()
				go s.PublishTrafficCheque(cheque.Beneficiary)
			}
			go s.PublishCashOut(CashOutStatus{Status: true})
		}
	}()

}

func (s *Service) SubscribeHeader(notifier *rpc.Notifier, sub *rpc.Subscription) {
	iNotifier := subscribe.NewNotifierWithDelay(notifier, sub, 1, false)
	_ = s.subPub.Subscribe(iNotifier, "traffic", "header", "")
	return
}

func (s *Service) SubscribeTrafficCheque(notifier *rpc.Notifier, sub *rpc.Subscription, addresses []types.AccountID) {
	iNotifier := subscribe.NewNotifierWithDelay(notifier, sub, 1, true)
	if len(addresses) == 0 {
		_ = s.subPub.Subscribe(iNotifier, "traffic", "trafficCheque", "")
	} else {
		for _, address := range addresses {
			_ = s.subPub.Subscribe(iNotifier, "traffic", "trafficCheque", address.ToHexString())
		}
	}
}

func (s *Service) SubscribeCashOut(notifier *rpc.Notifier, sub *rpc.Subscription) {
	iNotifier := subscribe.NewNotifierWithDelay(notifier, sub, 1, true)
	_ = s.subPub.Subscribe(iNotifier, "traffic", "cashOut", "*")
}

func (s *Service) PublishHeader() {
	trafficInfo, _ := s.TrafficInfo()
	_ = s.subPub.Publish("traffic", "header", "", trafficInfo)
}

func (s *Service) PublishTrafficCheque(overlay types.AccountID) {
	traffic := s.getTraffic(overlay)
	traffic.Lock()
	trans := new(big.Int).Sub(traffic.transferTraffic, traffic.transferChequeTraffic)
	retrieve := new(big.Int).Sub(traffic.retrieveTraffic, traffic.retrieveChequeTraffic)
	peer, _ := s.addressBook.BeneficiaryPeer(overlay)
	trafficCheque := TrafficCheque{
		Peer:                peer,
		OutstandingTraffic:  new(big.Int).Sub(trans, retrieve),
		SentSettlements:     traffic.retrieveChequeTraffic,
		ReceivedSettlements: traffic.transferChequeTraffic,
		Total:               new(big.Int).Sub(traffic.transferTraffic, traffic.retrieveTraffic),
		Uncashed:            new(big.Int).Sub(traffic.transferChequeTraffic, traffic.transferChainTraffic),
		Status:              traffic.status,
	}
	traffic.Unlock()
	_ = s.subPub.Publish("traffic", "trafficCheque", overlay.ToHexString(), trafficCheque)
}

func (s *Service) PublishCashOut(msg CashOutStatus) {
	_ = s.subPub.Publish("traffic", "cashOut", "*", msg)
}

func (t *Traffic) updateStatus(status CashStatus) {
	t.Lock()
	defer t.Unlock()
	t.status = status
}
