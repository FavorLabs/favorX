package multicast

import (
	"context"
	"fmt"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/multicast/pb"
	"github.com/FavorLabs/favorX/pkg/rpc"
)

func (s *Service) API() rpc.API {
	return rpc.API{
		Namespace: "group",
		Service:   &apiService{s: s},
	}
}

type apiService struct {
	s *Service
}

// Message subscribe the group message
func (a *apiService) Message(ctx context.Context, name string) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	gid, err := boson.ParseHexAddress(name)
	if err != nil {
		gid = GenerateGID(name)
	}
	err = a.s.SubscribeGroupMessage(notifier, sub, gid)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// Multicast subscribe the group multicast message
func (a *apiService) Multicast(ctx context.Context, name string) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	gid, err := boson.ParseHexAddress(name)
	if err != nil {
		gid = GenerateGID(name)
	}
	err = a.s.SubscribeMulticastMsg(notifier, sub, gid)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (a *apiService) Peers(ctx context.Context, name string) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	err := a.s.subscribeGroupPeers(notifier, sub, name)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// Reply to the group message to give the session ID
func (a *apiService) Reply(sessionID string, data []byte) error {
	return a.s.ReplyGroupMessage(sessionID, data)
}

func (a *apiService) Join(req model.ConfigNodeGroup) error {
	return a.s.AddGroup([]model.ConfigNodeGroup{req})
}

func (a *apiService) Leave(group string) error {
	return a.s.RemoveGroup(group, model.GTypeJoin)
}

func (a *apiService) PeersInfo(group string) (*GroupPeersInfo, error) {
	g, err := a.s.GetGroup(group)
	if err != nil {
		return &GroupPeersInfo{}, err
	}
	return g.PeersInfo(), nil
}

func (a *apiService) Broadcast(group string, msg []byte) error {
	gid, err := boson.ParseHexAddress(group)
	if err != nil {
		gid = GenerateGID(group)
	}
	err = a.s.Multicast(&pb.MulticastMsg{
		Gid:  gid.Bytes(),
		Data: msg,
	})
	return err
}

// SendRequest the timeout, After the node at the other node is notified of the subscription channel,
// it waits for the message reply time,
// and if it does not wait for the message reply after the time, it will discard the cached p2p stream.
func (a *apiService) SendRequest(ctx context.Context, timeout int64, group string, target boson.Address, msg []byte) (resp []byte, err error) {
	gid, err := boson.ParseHexAddress(group)
	if err != nil {
		gid = GenerateGID(group)
	}
	resp, err = a.s.SendReceive(ctx, timeout, msg, gid, target)
	fmt.Printf("%v", resp)
	return
}

func (a *apiService) Notify(ctx context.Context, group string, target boson.Address, msg []byte) error {
	gid, err := boson.ParseHexAddress(group)
	if err != nil {
		gid = GenerateGID(group)
	}
	return a.s.Send(ctx, msg, gid, target)
}

func (a *apiService) PeerState(ctx context.Context, group string) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	err := a.s.subscribePeerState(notifier, sub, group)
	if err != nil {
		return nil, err
	}
	return sub, nil
}
