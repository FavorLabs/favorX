package mock

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/routetab/pb"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Service is the mock of a P2P Service
type Service struct {
	addProtocolFunc       func(p2p.ProtocolSpec) error
	connectFunc           func(ctx context.Context, addr ma.Multiaddr) (peer *p2p.Peer, err error)
	disconnectFunc        func(overlay boson.Address, reason string) error
	peersFunc             func() []p2p.Peer
	blocklistedPeersFunc  func() ([]p2p.BlockPeers, error)
	addressesFunc         func() ([]ma.Multiaddr, error)
	notifierFunc          p2p.PickyNotifier
	setWelcomeMessageFunc func(string) error
	getWelcomeMessageFunc func() string
	blocklistFunc         func(boson.Address, time.Duration, string) error
	welcomeMessage        string
}

func (s *Service) CallHandlerWithConnChain(ctx context.Context, last, src p2p.Peer, stream p2p.Stream, protocolName, protocolVersion, streamName string) error {
	// TODO implement me
	panic("implement me")
}

// WithAddProtocolFunc sets the mock implementation of the AddProtocol function
func WithAddProtocolFunc(f func(p2p.ProtocolSpec) error) Option {
	return optionFunc(func(s *Service) {
		s.addProtocolFunc = f
	})
}

// WithConnectFunc sets the mock implementation of the Connect function
func WithConnectFunc(f func(ctx context.Context, addr ma.Multiaddr) (peer *p2p.Peer, err error)) Option {
	return optionFunc(func(s *Service) {
		s.connectFunc = f
	})
}

// WithDisconnectFunc sets the mock implementation of the Disconnect function
func WithDisconnectFunc(f func(overlay boson.Address, reason string) error) Option {
	return optionFunc(func(s *Service) {
		s.disconnectFunc = f
	})
}

// WithPeersFunc sets the mock implementation of the Peers function
func WithPeersFunc(f func() []p2p.Peer) Option {
	return optionFunc(func(s *Service) {
		s.peersFunc = f
	})
}

// WithBlocklistedPeersFunc sets the mock implementation of the BlocklistedPeers function
func WithBlocklistedPeersFunc(f func() ([]p2p.BlockPeers, error)) Option {
	return optionFunc(func(s *Service) {
		s.blocklistedPeersFunc = f
	})
}

// WithAddressesFunc sets the mock implementation of the Adresses function
func WithAddressesFunc(f func() ([]ma.Multiaddr, error)) Option {
	return optionFunc(func(s *Service) {
		s.addressesFunc = f
	})
}

// WithGetWelcomeMessageFunc sets the mock implementation of the GetWelcomeMessage function
func WithGetWelcomeMessageFunc(f func() string) Option {
	return optionFunc(func(s *Service) {
		s.getWelcomeMessageFunc = f
	})
}

// WithSetWelcomeMessageFunc sets the mock implementation of the SetWelcomeMessage function
func WithSetWelcomeMessageFunc(f func(string) error) Option {
	return optionFunc(func(s *Service) {
		s.setWelcomeMessageFunc = f
	})
}

func WithBlocklistFunc(f func(boson.Address, time.Duration, string) error) Option {
	return optionFunc(func(s *Service) {
		s.blocklistFunc = f
	})
}

// New will create a new mock P2P Service with the given options
func New(opts ...Option) *Service {
	s := new(Service)
	for _, o := range opts {
		o.apply(s)
	}
	return s
}

func (s *Service) AddProtocol(spec p2p.ProtocolSpec) error {
	if s.addProtocolFunc == nil {
		return errors.New("function AddProtocol not configured")
	}
	return s.addProtocolFunc(spec)
}

func (s *Service) NATAddresses() ([]net.Addr, error) {
	a, err := net.ResolveIPAddr("ip", "1.1.1.1")
	return []net.Addr{a}, err
}

func (s *Service) Connect(ctx context.Context, addr ma.Multiaddr) (peer *p2p.Peer, err error) {
	if s.connectFunc == nil {
		return nil, errors.New("function Connect not configured")
	}
	return s.connectFunc(ctx, addr)
}

func (s *Service) Disconnect(overlay boson.Address, reason string) error {
	if s.disconnectFunc == nil {
		return errors.New("function Disconnect not configured")
	}

	if s.notifierFunc != nil {
		s.notifierFunc.Disconnected(p2p.Peer{Address: overlay, Mode: address.NewModel()}, reason)
	}

	return s.disconnectFunc(overlay, reason)
}

func (s *Service) Addresses() ([]ma.Multiaddr, error) {
	if s.addressesFunc == nil {
		return nil, errors.New("function Addresses not configured")
	}
	return s.addressesFunc()
}

func (s *Service) Peers() []p2p.Peer {
	if s.peersFunc == nil {
		return nil
	}
	return s.peersFunc()
}

func (s *Service) PeerID(overlay boson.Address) (id libp2ppeer.ID, found bool) {
	return
}

func (s *Service) ResourceManager() network.ResourceManager {
	// TODO implement me
	panic("implement me")
}

func (s *Service) BlocklistedPeers() ([]p2p.BlockPeers, error) {
	if s.blocklistedPeersFunc == nil {
		return nil, nil
	}

	return s.blocklistedPeersFunc()
}

func (s *Service) SetWelcomeMessage(val string) error {
	if s.setWelcomeMessageFunc != nil {
		return s.setWelcomeMessageFunc(val)
	}
	s.welcomeMessage = val
	return nil
}

func (s *Service) GetWelcomeMessage() string {
	if s.getWelcomeMessageFunc != nil {
		return s.getWelcomeMessageFunc()
	}
	return s.welcomeMessage
}

func (s *Service) Halt() {}

func (s *Service) Blocklist(overlay boson.Address, duration time.Duration, reason string) error {
	if s.blocklistFunc == nil {
		return errors.New("function blocklist not configured")
	}
	return s.blocklistFunc(overlay, duration, reason)
}

func (s *Service) BlocklistRemove(overlay boson.Address) error {
	return nil
}

func (s *Service) SetPickyNotifier(f p2p.PickyNotifier) {
	s.notifierFunc = f
}

func (s *Service) SetConnectFunc(f func(ctx context.Context, addr ma.Multiaddr) (peer *p2p.Peer, err error)) {
	s.connectFunc = f
}

func (s *Service) CallHandler(ctx context.Context, last p2p.Peer, stream p2p.Stream) (relayData *pb.RouteRelayReq, w *p2p.WriterChan, r *p2p.ReaderChan, forward bool, err error) {
	return
}

// NetworkStatus implements p2p.NetworkStatuser interface.
// It always returns p2p.NetworkStatusAvailable.
func (s *Service) NetworkStatus() p2p.NetworkStatus {
	return p2p.NetworkStatusAvailable
}

type Option interface {
	apply(*Service)
}
type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
