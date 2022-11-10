package handshake

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/FavorLabs/favorX/pkg/topology/lightnode"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	// ProtocolName is the text of the name of the handshake protocol.
	ProtocolName = "handshake"
	// ProtocolVersion is the current handshake protocol version.
	ProtocolVersion = "5.0.0"
	// StreamName is the name of the stream used for handshake purposes.
	StreamName = "handshake"
	// MaxWelcomeMessageLength is maximum number of characters allowed in the welcome message.
	MaxWelcomeMessageLength = 140
	handshakeTimeout        = 15 * time.Second
)

var (
	// ErrNetworkIDIncompatible is returned if response from the other peer does not have valid networkID.
	ErrNetworkIDIncompatible = errors.New("incompatible network ID")

	// ErrInvalidAck is returned if data in received in ack is not valid (invalid signature for example).
	ErrInvalidAck = errors.New("invalid ack")

	// ErrInvalidSyn is returned if observable address in ack is not a valid.
	ErrInvalidSyn = errors.New("invalid syn")

	// ErrWelcomeMessageLength is returned if the welcome message is longer than the maximum length
	ErrWelcomeMessageLength = fmt.Errorf("handshake welcome message longer than maximum of %d characters", MaxWelcomeMessageLength)

	// ErrPicker is returned if the picker (kademlia) rejects the peer
	ErrPicker = fmt.Errorf("picker rejection")

	ErrPickerLight = fmt.Errorf("picker rejection light")
)

// AdvertisableAddressResolver can Resolve a Multiaddress.
type AdvertisableAddressResolver interface {
	Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error)
}

// Service can perform initiate or handle a handshake between peers.
type Service struct {
	signer                crypto.Signer
	advertisableAddresser AdvertisableAddressResolver
	overlay               boson.Address
	nodeMode              address.Model
	networkID             uint64
	welcomeMessage        atomic.Value
	logger                logging.Logger
	libp2pID              libp2ppeer.ID
	metrics               metrics
	picker                p2p.Picker
	lightNodes            lightnode.LightNodes
	lightNodeLimit        int
}

// New creates a new handshake Service.
func New(signer crypto.Signer, advertisableAddresser AdvertisableAddressResolver, overlay boson.Address, networkID uint64, nodeMode address.Model, welcomeMessage string, ownPeerID libp2ppeer.ID, logger logging.Logger, lightNodes *lightnode.Container, lightLimit int) (*Service, error) {
	if len(welcomeMessage) > MaxWelcomeMessageLength {
		return nil, ErrWelcomeMessageLength
	}

	svc := &Service{
		signer:                signer,
		advertisableAddresser: advertisableAddresser,
		overlay:               overlay,
		networkID:             networkID,
		nodeMode:              nodeMode,
		logger:                logger,
		libp2pID:              ownPeerID,
		metrics:               newMetrics(),
		lightNodes:            lightNodes,
		lightNodeLimit:        lightLimit,
	}
	svc.welcomeMessage.Store(welcomeMessage)

	return svc, nil
}

func (s *Service) SetPicker(n p2p.Picker) {
	s.picker = n
}

// Handshake initiates a handshake with a peer.
func (s *Service) Handshake(ctx context.Context, stream p2p.Stream, peerMultiaddr ma.Multiaddr, peerID libp2ppeer.ID) (i *address.AddressInfo, err error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)
	fullRemoteMA, err := buildFullMA(peerMultiaddr, peerID)
	if err != nil {
		return nil, err
	}

	fullRemoteMABytes, err := fullRemoteMA.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Syn{
		ObservedUnderlay: fullRemoteMABytes,
	}); err != nil {
		return nil, fmt.Errorf("write syn message: %w", err)
	}

	var resp pb.SynAck
	if err := r.ReadMsgWithContext(ctx, &resp); err != nil {
		return nil, fmt.Errorf("read synack message: %w", err)
	}

	observedUnderlay, err := ma.NewMultiaddrBytes(resp.Syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	observedUnderlayAddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(observedUnderlay)
	if err != nil {
		return nil, fmt.Errorf("extract addr from P2P: %w", err)
	}

	if s.libp2pID != observedUnderlayAddrInfo.ID {
		// NOTE eventually we will return error here, but for now we want to gather some statistics
		s.logger.Warningf("received peer ID %s does not match ours: %s", observedUnderlayAddrInfo.ID, s.libp2pID)
	}

	advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
	if err != nil {
		return nil, err
	}

	addr, err := address.NewAddress(s.signer, advertisableUnderlay, s.overlay, s.networkID)
	if err != nil {
		return nil, err
	}

	advertisableUnderlayBytes, err := addr.Underlay.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if resp.Ack.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	remoteAddress, err := s.parseCheckAck(resp.Ack)
	if err != nil {
		return nil, err
	}

	// Synced read:
	welcomeMessage := s.GetWelcomeMessage()
	if err = w.WriteMsgWithContext(ctx, &pb.Ack{
		Address: &pb.Address{
			Underlay:  advertisableUnderlayBytes,
			PublicKey: addr.PublicKey,
			Overlay:   addr.Overlay.Bytes(),
			Signature: addr.Signature,
		},
		NetworkID:      s.networkID,
		NodeMode:       s.nodeMode.Bv.Bytes(),
		WelcomeMessage: welcomeMessage,
	}); err != nil {
		return nil, fmt.Errorf("write ack message: %w", err)
	}

	s.logger.Tracef("handshake finished for peer (outbound) %s", remoteAddress.Overlay.String())
	if len(resp.Ack.WelcomeMessage) > 0 {
		s.logger.Infof("greeting \"%s\" from peer: %s", resp.Ack.WelcomeMessage, remoteAddress.Overlay.String())
	}

	md, err := address.NewModelFromBytes(resp.Ack.NodeMode)
	if err != nil {
		return nil, address.ErrInvalidNodeMode
	}
	return &address.AddressInfo{
		Address:  remoteAddress,
		NodeMode: md,
	}, nil
}

// Handle handles an incoming handshake from a peer.
func (s *Service) Handle(ctx context.Context, stream p2p.Stream, remoteMultiaddr ma.Multiaddr, remotePeerID libp2ppeer.ID) (i *address.AddressInfo, err error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)
	fullRemoteMA, err := buildFullMA(remoteMultiaddr, remotePeerID)
	if err != nil {
		return nil, err
	}

	fullRemoteMABytes, err := fullRemoteMA.MarshalBinary()
	if err != nil {
		return nil, err
	}

	var syn pb.Syn
	if err := r.ReadMsgWithContext(ctx, &syn); err != nil {
		s.metrics.SynRxFailed.Inc()
		return nil, fmt.Errorf("read syn message: %w", err)
	}
	s.metrics.SynRx.Inc()

	observedUnderlay, err := ma.NewMultiaddrBytes(syn.ObservedUnderlay)
	if err != nil {
		return nil, ErrInvalidSyn
	}

	advertisableUnderlay, err := s.advertisableAddresser.Resolve(observedUnderlay)
	if err != nil {
		return nil, err
	}

	addr, err := address.NewAddress(s.signer, advertisableUnderlay, s.overlay, s.networkID)
	if err != nil {
		return nil, err
	}

	advertisableUnderlayBytes, err := addr.Underlay.MarshalBinary()
	if err != nil {
		return nil, err
	}

	welcomeMessage := s.GetWelcomeMessage()

	if err = w.WriteMsgWithContext(ctx, &pb.SynAck{
		Syn: &pb.Syn{
			ObservedUnderlay: fullRemoteMABytes,
		},
		Ack: &pb.Ack{
			Address: &pb.Address{
				Underlay:  advertisableUnderlayBytes,
				PublicKey: addr.PublicKey,
				Overlay:   addr.Overlay.Bytes(),
				Signature: addr.Signature,
			},
			NetworkID:      s.networkID,
			NodeMode:       s.nodeMode.Bv.Bytes(),
			WelcomeMessage: welcomeMessage,
		},
	}); err != nil {
		s.metrics.SynAckTxFailed.Inc()
		return nil, fmt.Errorf("write synack message: %w", err)
	}
	s.metrics.SynAckTx.Inc()

	var ack pb.Ack
	if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
		s.metrics.AckRxFailed.Inc()
		return nil, fmt.Errorf("read ack message: %w", err)
	}
	s.metrics.AckRx.Inc()

	if ack.NetworkID != s.networkID {
		return nil, ErrNetworkIDIncompatible
	}

	mode, err := address.NewModelFromBytes(ack.NodeMode)
	if err != nil {
		return nil, address.ErrInvalidNodeMode
	}
	overlay := boson.NewAddress(ack.Address.Overlay)

	if s.picker != nil {
		if mode.IsFull() {
			if !s.picker.Pick(p2p.Peer{Address: overlay, Mode: mode}) {
				return nil, ErrPicker
			}
		} else {
			if s.lightNodes.Count() >= s.lightNodeLimit {
				s.logger.Warningf("%s %s with limit %d", ErrPickerLight.Error(), overlay.String(), s.lightNodeLimit)
				return nil, ErrPickerLight
			}
		}
	}

	remoteAddress, err := s.parseCheckAck(&ack)
	if err != nil {
		return nil, err
	}

	s.logger.Tracef("handshake finished for peer (inbound) %s", remoteAddress.Overlay.String())
	if len(ack.WelcomeMessage) > 0 {
		s.logger.Infof("greeting \"%s\" from peer: %s", ack.WelcomeMessage, remoteAddress.Overlay.String())
	}

	return &address.AddressInfo{
		Address:  remoteAddress,
		NodeMode: mode,
	}, nil
}

// SetWelcomeMessage sets the new handshake welcome message.
func (s *Service) SetWelcomeMessage(msg string) (err error) {
	if len(msg) > MaxWelcomeMessageLength {
		return ErrWelcomeMessageLength
	}
	s.welcomeMessage.Store(msg)
	return nil
}

// GetWelcomeMessage returns the current handshake welcome message.
func (s *Service) GetWelcomeMessage() string {
	return s.welcomeMessage.Load().(string)
}

func buildFullMA(addr ma.Multiaddr, peerID libp2ppeer.ID) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr.String(), peerID.Pretty()))
}

func (s *Service) parseCheckAck(ack *pb.Ack) (*address.Address, error) {
	addr, err := address.ParseAddress(ack.Address.Underlay, ack.Address.PublicKey, ack.Address.Overlay, ack.Address.Signature, s.networkID)
	if err != nil {
		return nil, ErrInvalidAck
	}

	return addr, nil
}
