package multicast

import (
	"context"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/multicast/pb"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/subscribe"
)

type GroupInterface interface {
	Multicast(info *pb.MulticastMsg, skip ...boson.Address) error
	AddGroup(groups []model.ConfigNodeGroup) error
	RemoveGroup(group string, gType model.GType) error
	Snapshot() *model.KadParams
	StartDiscover()
	SubscribeLogContent(n *rpc.Notifier, sub *rpc.Subscription)
	SubscribeMulticastMsg(n *rpc.Notifier, sub *rpc.Subscription, gid boson.Address) (err error)
	GetGroupPeers(groupName string) (out *GroupPeers, err error)
	GetOptimumPeer(groupName string) (peer boson.Address, err error)
	GetSendStream(ctx context.Context, gid, dest boson.Address) (out SendStreamCh, err error)
	SendReceive(ctx context.Context, timeout int64, data []byte, gid, dest boson.Address) (result []byte, err error)
	Send(ctx context.Context, data []byte, gid, dest boson.Address) (err error)
}

type GroupStorageFiles interface {
	RemoveGroup(gid boson.Address, gType model.GType) error
	SubscribeGroupMessageWithChan(notifier *subscribe.NotifierWithMsgChan, gid boson.Address) (err error)
	ReplyGroupMessage(sessionID string, data []byte) (err error)
}

// Message multicast message
type Message struct {
	ID         uint64
	CreateTime int64
	GID        boson.Address
	Origin     boson.Address
	Data       []byte
	From       boson.Address
}

type GroupMessage struct {
	SessionID rpc.ID        `json:"sessionID,omitempty"`
	GID       boson.Address `json:"gid"`
	Data      []byte        `json:"data"`
	From      boson.Address `json:"from"`
	Timeout   int64         `json:"timeout"`
}

type LogContent struct {
	Event string
	Time  int64 // ms
	Data  Message
}

type GroupPeers struct {
	Connected []boson.Address `json:"connected"`
	Keep      []boson.Address `json:"keep"`
}

// PeerState
// Action 0 add 1 remove,
// Type   0 connected 1 kept,
type PeerState struct {
	Overlay boson.Address
	Action  int // 0 add 1 remove
	Type    int // 0 connected 1 kept
}

type SendOption int

const (
	SendOnly SendOption = iota
	SendReceive
	SendStream
)

func (s SendOption) String() string {
	switch s {
	case SendOnly:
		return "SendOnly"
	case SendReceive:
		return "SendReceive"
	case SendStream:
		return "SendStream"
	default:
		return ""
	}
}

type SendStreamCh struct {
	Read     chan []byte
	ReadErr  chan error
	Write    chan []byte
	WriteErr chan error
	Close    chan struct{}
}
