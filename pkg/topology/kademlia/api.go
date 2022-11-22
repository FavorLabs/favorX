package kademlia

import (
	"context"

	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/libp2p/go-libp2p-core/peer"
)

func (k *Kad) API() rpc.API {
	return rpc.API{
		Namespace: "p2p",
		Version:   "1.0",
		Service:   &apiService{kad: k},
		Public:    true,
	}
}

type apiService struct {
	kad *Kad
}

type Connected struct {
	FullNodes  int  `json:"full_nodes"`
	LightNodes uint `json:"light_nodes"`
	BootNodes  uint `json:"boot_nodes"`
}

type KadInfo struct {
	Depth      uint8     `json:"depth"`
	Population int       `json:"population"`
	Connected  Connected `json:"connected"`
}

func (a *apiService) KadInfo(ctx context.Context) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	iNotifier := subscribe.NewNotifierWithDelay(notifier, sub, 1, false)
	a.kad.SubscribePeersChange(iNotifier)

	return sub, nil
}

func (a *apiService) PeerState(ctx context.Context) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	iNotifier := subscribe.NewNotifier(notifier, sub)
	a.kad.SubscribePeerState(iNotifier)
	return sub, nil
}

type AddressInfo struct {
	PeerID    peer.ID
	PublicKey string
	Overlay   string
}

func (a *apiService) Address() AddressInfo {
	peerID, _ := peer.IDFromPrivateKey(a.kad.p2p.PrivateKey())
	return AddressInfo{
		PeerID:    peerID,
		PublicKey: a.kad.signer.Public().Hex(),
		Overlay:   a.kad.base.String(),
	}
}
