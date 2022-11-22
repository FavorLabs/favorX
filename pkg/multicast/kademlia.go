package multicast

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/multicast/pb"
	"github.com/FavorLabs/favorX/pkg/multicast/peerset"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/FavorLabs/favorX/pkg/routetab"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/FavorLabs/favorX/pkg/topology"
	"github.com/FavorLabs/favorX/pkg/topology/lightnode"
	topModel "github.com/FavorLabs/favorX/pkg/topology/model"
	"github.com/gogf/gf/v2/util/gconv"
)

const (
	protocolName    = "multicast"
	protocolVersion = "2.0.0"
	streamHandshake = "handshake"
	streamFindGroup = "findGroup"
	streamMulticast = "multicast"
	streamMessage   = "message"

	handshakeTimeout = time.Second * 15
	keepPingInterval = time.Second * 30

	multicastMsgCache = time.Minute * 1 // According to the message of the whole network arrival time to determine
)

type Service struct {
	o              Option
	nodeMode       address.Model
	self           boson.Address
	p2ps           p2p.Service
	stream         p2p.Streamer
	logger         logging.Logger
	kad            topology.Driver
	lightNodes     lightnode.LightNodes
	route          routetab.RouteTab
	connectedPeers sync.Map // key=gid, *peerset.PeersManager is peer, all is neighbor
	groups         sync.Map // key=gid, *Group
	peerGidList    sync.Map // key=peer, [][]byte gid list with peer
	msgSeq         uint64
	close          chan struct{}
	sessionStream  sync.Map // key= sessionID, value= *WsStream
	virtualConn    *peerset.VirtualManager

	chanNotifyFunc chan func()

	subPub subscribe.SubPub
}

type WsStream struct {
	done       chan struct{} // after w write successful
	sendOption SendOption
	stream     p2p.Stream
	r          protobuf.Reader
	w          protobuf.Writer
}

type PeersSubClient struct {
	notify       *rpc.Notifier
	sub          *rpc.Subscription
	lastPushTime time.Time
}

func (s *Service) newGroup(gid boson.Address, o model.ConfigNodeGroup) *Group {
	if o.KeepConnectedPeers < 0 {
		o.KeepConnectedPeers = 0
	}
	if o.KeepPingPeers < 0 {
		o.KeepPingPeers = 0
	}
	g := &Group{
		gid:                gid,
		keepPeers:          peerset.New(s.chanNotifyFunc),
		knownPeers:         peerset.New(s.chanNotifyFunc),
		keptInPeers:        peerset.New(s.chanNotifyFunc),
		srv:                s,
		option:             o,
		groupPeersLastSend: time.Now(),
		groupPeersSending:  make(chan struct{}, 1),
		quit:               make(chan struct{}),
		chanCheckKept:      make(chan eventCheckKept, 100),
		chanEvent:          make(chan eventPeerAction, 100),
		chanNotifyFunc:     s.chanNotifyFunc,
	}
	conn, ok := s.connectedPeers.Load(gid.String())
	if ok {
		g.connectedPeers = conn.(*peerset.PeersManager)
	} else {
		g.connectedPeers = peerset.New(s.chanNotifyFunc)
		s.connectedPeers.Store(gid.String(), g.connectedPeers)
	}
	go g.manage()
	return g
}

type Option struct {
	Dev bool
}

func NewService(self boson.Address, nodeMode address.Model, service p2p.Service, streamer p2p.Streamer, kad topology.Driver, light lightnode.LightNodes,
	route routetab.RouteTab, logger logging.Logger, subPub subscribe.SubPub, o Option) *Service {
	srv := &Service{
		o:              o,
		nodeMode:       nodeMode,
		self:           self,
		p2ps:           service,
		stream:         streamer,
		logger:         logger,
		kad:            kad,
		lightNodes:     light,
		route:          route,
		subPub:         subPub,
		close:          make(chan struct{}, 1),
		virtualConn:    peerset.NewVirtual(),
		chanNotifyFunc: make(chan func(), 100),
	}
	return srv
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamHandshake,
				Handler: s.HandshakeIncoming,
			},
			{
				Name:    streamFindGroup,
				Handler: s.onFindGroup,
			},
			{
				Name:    streamMulticast,
				Handler: s.onMulticast,
			},
			{
				Name:    streamMessage,
				Handler: s.onMessage,
			},
		},
	}
}

func (s *Service) Start() {
	mNotifier := subscribe.NewNotifierWithMsgChan()
	s.kad.SubscribePeerState(mNotifier)
	go func() {
		ticker := time.NewTicker(keepPingInterval)
		defer func() {
			ticker.Stop()
			close(mNotifier.ErrChan)
		}()

		for {
			select {
			case <-s.close:
				return
			case info := <-mNotifier.MsgChan:
				peer, ok := info.(p2p.PeerInfo)
				if !ok {
					continue
				}
				switch peer.State {
				case p2p.PeerStateConnectOut:
					s.logger.Tracef("event connectOut handshake with group protocol %s", peer.Overlay)
					err := s.Handshake(context.Background(), peer.Overlay)
					if err != nil {
						s.logger.Errorf("multicast handshake %s", err.Error())
					}
				case p2p.PeerStateDisconnect:
					s.onDisconnect(peer.Overlay)
				}
			case <-ticker.C:
				wg := &sync.WaitGroup{}
				doHandshake := func(g *Group, address boson.Address) {
					defer wg.Done()
					var err error
					for i := 0; i < 2; i++ {
						err = s.Handshake(context.Background(), address)
						if err == nil {
							break
						}
						s.logger.Tracef("keep ping %s %s", address, err)
					}
				}
				s.groups.Range(func(_, value interface{}) bool {
					g := value.(*Group)
					for _, v := range g.keepPeers.Peers() {
						info := s.virtualConn.GetInfo(v)
						if info != nil && info.Direction == topModel.PeerConnectionDirectionOutbound {
							wg.Add(1)
							go doHandshake(g, v)
						}
					}
					return true
				})
				wg.Wait()
				s.refreshProtectPeers()
				ticker.Reset(keepPingInterval)
			}
		}
	}()

	// discover
	if !s.o.Dev {
		s.StartDiscover()
	}
}

func (s *Service) Close() error {
	close(s.close)
	return nil
}

func (s *Service) refreshProtectPeers() {
	var list []boson.Address
	s.groups.Range(func(key, value interface{}) bool {
		g := value.(*Group)
		list = append(list, g.connectedPeers.Peers()...)
		list = append(list, g.keepPeers.Peers()...)
		return true
	})
	s.kad.RefreshProtectPeer(list)
}

func (s *Service) addToGroup(gid, p boson.Address) {
	var (
		g    *Group
		conn *peerset.PeersManager
	)
	v, ok := s.groups.Load(gid.String())
	if ok {
		g = v.(*Group)
		conn = g.connectedPeers
	} else {
		connV, has := s.connectedPeers.Load(gid.String())
		if has {
			conn = connV.(*peerset.PeersManager)
		} else {
			conn = peerset.New(s.chanNotifyFunc)
			s.connectedPeers.Store(gid.String(), conn)
		}
	}
	if s.route.IsNeighbor(p) {
		if g != nil {
			g.chanEvent <- eventPeerAction{
				addr:  p,
				event: EventAddConn,
			}
		} else if !conn.Exists(p) {
			conn.Add(p)
		}
	} else if g != nil {
		g.chanEvent <- eventPeerAction{
			addr:  p,
			event: EventAddKept,
		}
	}
}

func (s *Service) refreshGroup(peer boson.Address, nowGidList [][]byte) {
	var nowGid []boson.Address
	for _, v := range nowGidList {
		gid := boson.NewAddress(v)
		s.logger.Tracef("group: now peer %s add group %s", peer, gid)
		nowGid = append(nowGid, gid)
		s.addToGroup(gid, peer)
	}
	val, ok := s.peerGidList.Load(peer.String())
	if ok {
		oldList := val.([][]byte)
		for _, o := range oldList {
			old := boson.NewAddress(o)
			if !old.MemberOf(nowGid) {
				s.logger.Tracef("group: now peer %s remove group %s", peer, old)
				// remove from group
				s.removeFromGroup(old, peer)
			}
		}
	}
	s.peerGidList.Store(peer.String(), nowGidList)
}

func (s *Service) removeFromGroup(gid boson.Address, addr boson.Address) {
	value, ok := s.groups.Load(gid.String())
	if ok {
		g := value.(*Group)
		g.chanEvent <- eventPeerAction{
			addr:  addr,
			event: EventRemove,
		}
	} else {
		val, has := s.connectedPeers.Load(gid.String())
		if has {
			conn := val.(*peerset.PeersManager)
			conn.Remove(addr)
			if conn.Length() == 0 {
				s.connectedPeers.Delete(gid.String())
			}
		}
	}
}

func (s *Service) onDisconnect(v boson.Address) {
	s.connectedPeers.Range(func(key, value interface{}) bool {
		var g *Group
		val, ok := s.groups.Load(gconv.String(key))
		if ok {
			g = val.(*Group)
		}
		conn := value.(*peerset.PeersManager)
		if g != nil {
			g.chanEvent <- eventPeerAction{
				addr:  v,
				event: EventDisconnect,
			}
		} else {
			conn.Remove(v)
			if conn.Length() == 0 {
				s.connectedPeers.Delete(key)
			}
		}
		return true
	})
}

func (s *Service) getGIDsByte() [][]byte {
	GIDs := make([][]byte, 0)
	if s.nodeMode.IsBootNode() || !s.nodeMode.IsFull() {
		return GIDs
	}
	s.groups.Range(func(key, value interface{}) bool {
		gid := boson.MustParseHexAddress(gconv.String(key))
		g := value.(*Group)
		if g.option.GType == model.GTypeJoin {
			GIDs = append(GIDs, gid.Bytes())
		}
		return true
	})
	return GIDs
}

func (s *Service) Multicast(info *pb.MulticastMsg, skip ...boson.Address) error {
	if len(info.Origin) == 0 {
		info.CreateTime = time.Now().UnixMilli()
		info.Origin = s.self.Bytes()
		info.Id = atomic.AddUint64(&s.msgSeq, 1)
	}
	origin := boson.NewAddress(info.Origin)

	key := fmt.Sprintf("Multicast_%s_%d", origin, info.Id)
	setOK, err := cache.SetIfNotExist(cacheCtx, key, 1, multicastMsgCache)
	if err != nil {
		return err
	}
	if !setOK {
		return nil
	}

	gid := boson.NewAddress(info.Gid)

	s.logger.Tracef("multicast deliver: %s data=%v", key, info.Data)
	s.notifyLogContent(LogContent{
		Event: "multicast_deliver",
		Time:  time.Now().UnixMilli(),
		Data: Message{
			ID:         info.Id,
			CreateTime: info.CreateTime,
			GID:        gid,
			Origin:     origin,
			Data:       info.Data,
		},
	})

	g, ok := s.groups.Load(gid.String())
	if ok {
		v := g.(*Group)
		if v.connectedPeers.Length() == 0 && v.keepPeers.Length() == 0 {
			s.discover(v)
		}
		if v.connectedPeers.Length() == 0 && v.keepPeers.Length() == 0 {
			return nil
		}
		// An isolated node within the group
		send := func(address boson.Address) {
			if !address.MemberOf(skip) {
				_ = s.sendData(context.Background(), address, streamMulticast, info)
			}
		}
		v.connectedPeers.Iter(send)
		v.keepPeers.Iter(send)
		return nil
	}

	nodes := s.getForwardNodes(gid, skip...)
	s.logger.Tracef("multicast got forward %d nodes", len(nodes))
	for _, v := range nodes {
		s.logger.Tracef("multicast forward to %s", v)
		_ = s.sendData(context.Background(), v, streamMulticast, info)
	}
	return nil
}

func (s *Service) onMulticast(ctx context.Context, peer p2p.Peer, stream p2p.Stream) (err error) {
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()
	r := protobuf.NewReader(stream)
	info := &pb.MulticastMsg{}
	err = r.ReadMsgWithContext(ctx, info)
	if err != nil {
		return err
	}

	origin := boson.NewAddress(info.Origin)

	key := fmt.Sprintf("onMulticast_%s_%d", origin, info.Id)
	setOK, err := cache.SetIfNotExist(cacheCtx, key, 1, multicastMsgCache)
	if err != nil {
		return err
	}
	if !setOK {
		return nil
	}
	if origin.Equal(s.self) {
		return nil
	}

	gid := boson.NewAddress(info.Gid)
	msg := Message{
		ID:         info.Id,
		CreateTime: info.CreateTime,
		GID:        gid,
		Origin:     origin,
		Data:       info.Data,
		From:       peer.Address,
	}

	s.logger.Tracef("multicast receive: %s data=%v", key, info.Data)
	s.logger.Tracef("multicast receive from %s", peer.Address)

	notifyLog := true
	g, ok := s.groups.Load(gid.String())
	if ok && g.(*Group).option.GType == model.GTypeJoin {
		notifyLog = false
		_ = s.notifyMulticast(gid, msg)
		s.logger.Tracef("%s-multicast receive %s from %s", gid, key, peer.Address)
	}
	if notifyLog {
		s.notifyLogContent(LogContent{
			Event: "multicast_receive",
			Time:  time.Now().UnixMilli(),
			Data:  msg,
		})
	}
	return s.Multicast(info, peer.Address)
}

func (s *Service) notifyMulticast(gid boson.Address, msg Message) (e error) {
	g, ok := s.groups.Load(gid.String())
	if ok {
		v := g.(*Group)
		if v.multicastSub == false {
			return nil
		}
		defer func() {
			err := recover()
			if err != nil {
				e = fmt.Errorf("group %s , notify msg %s", gid, err)
				s.logger.Error(e)
				v.multicastSub = false
			}
		}()

		_ = s.subPub.Publish("group", "multicastMsg", gid.String(), msg)
	}
	return nil
}

func (s *Service) observeGroup(gid boson.Address, option model.ConfigNodeGroup) error {
	var g *Group
	v, ok := s.groups.Load(gid.String())
	if ok {
		g = v.(*Group)
	} else {
		option.GType = model.GTypeObserve
		g = s.newGroup(gid, option)
		s.groups.Store(gid.String(), g)
	}
	for _, addr := range option.Nodes {
		if addr.Equal(s.self) {
			continue
		}
		g.knownPeers.Add(addr)
	}
	go s.notify()
	go s.discover(g)
	s.logger.Infof("observe group success %s", gid)
	return nil
}

func (s *Service) observeGroupCancel(gid boson.Address) error {
	v, ok := s.groups.Load(gid.String())
	if !ok {
		return errors.New("group not found")
	}
	g := v.(*Group)
	if g.option.GType == model.GTypeObserve {
		s.groups.Delete(gid.String())
	}
	return nil
}

// Add yourself to the group, along with other nodes (if any)
func (s *Service) joinGroup(gid boson.Address, option model.ConfigNodeGroup) error {
	var g *Group
	value, ok := s.groups.Load(gid.String())
	if ok {
		g = value.(*Group)
		if g.option.GType == model.GTypeJoin {
			return errors.New("it's already in the group")
		}
	} else {
		g = s.newGroup(gid, option)
		s.groups.Store(gid.String(), g)
	}
	if g.option.GType == model.GTypeObserve {
		// observe group join group
		g.option.GType = model.GTypeJoin
	}
	for _, v := range option.Nodes {
		if v.Equal(s.self) {
			continue
		}
		g.knownPeers.Add(v)
	}

	go s.notify()
	go s.discover(g)
	s.logger.Infof("join group success %s", gid)
	return nil
}

func (s *Service) SubscribeMulticastMsg(n *rpc.Notifier, sub *rpc.Subscription, gid boson.Address) (err error) {
	notifier := subscribe.NewNotifier(n, sub)
	var g *Group
	value, ok := s.groups.Load(gid.String())
	if ok {
		g = value.(*Group)
		if g.option.GType == model.GTypeJoin && g.multicastSub == false {
			g.multicastSub = true
			_ = s.subPub.Subscribe(notifier, "group", "multicastMsg", gid.String())
			return nil
		}
		if g.multicastSub == true {
			return errors.New("multicast message subscription already exists")
		}
	}
	return ErrorGroupNotFound
}

func (s *Service) AddGroup(groups []model.ConfigNodeGroup) error {
	defer s.refreshProtectPeers()
	for _, optionGroup := range groups {
		if optionGroup.Name == "" {
			continue
		}
		var gAddr boson.Address
		addr, err := boson.ParseHexAddress(optionGroup.Name)
		if err != nil {
			gAddr = GenerateGID(optionGroup.Name)
		} else {
			gAddr = addr
		}
		switch optionGroup.GType {
		case model.GTypeJoin:
			err = s.joinGroup(gAddr, optionGroup)
		case model.GTypeObserve:
			err = s.observeGroup(gAddr, optionGroup)
		}
		if err != nil {
			s.logger.Errorf("Groups: Join group failed :%v ", err.Error())
		}
		return err
	}
	return nil
}

func (s *Service) RemoveGroup(group string, gType model.GType) error {
	gid, err := boson.ParseHexAddress(group)
	if err != nil {
		gid = GenerateGID(group)
	}
	defer s.refreshProtectPeers()
	switch gType {
	case model.GTypeObserve:
		return s.observeGroupCancel(gid)
	case model.GTypeJoin:
		return s.leaveGroup(gid)
	default:
		return errors.New("gType not support")
	}
}

// LeaveGroup For yourself
func (s *Service) leaveGroup(gid boson.Address) error {
	value, ok := s.groups.Load(gid.String())
	if !ok {
		return errors.New("group not found")
	}
	g := value.(*Group)
	if g.connectedPeers.Length() == 0 {
		s.connectedPeers.Delete(gid.String())
	}

	copyGroups := make([]*Group, 0)
	s.groups.Range(func(_, value interface{}) bool {
		v := value.(*Group)
		copyGroups = append(copyGroups, v)
		return true
	})
	s.groups.Delete(gid.String())

	g.Close()
	go s.notify(copyGroups...)

	s.logger.Infof("leave group success %s", gid)
	return nil
}

func (s *Service) notify(groups ...*Group) {
	send := func(address boson.Address, u uint8) (stop, jumpToNext bool, err error) {
		_ = s.Handshake(context.Background(), address)
		return false, false, nil
	}
	_ = s.kad.EachPeerRev(send, topology.Filter{Reachable: false})
	_ = s.lightNodes.EachPeer(send)

	if len(groups) == 0 {
		s.groups.Range(func(_, value interface{}) bool {
			v := value.(*Group)
			_ = v.keepPeers.EachBin(send)
			return true
		})
	} else {
		for _, v := range groups {
			_ = v.keepPeers.EachBin(send)
		}
	}
}

func (s *Service) sendData(ctx context.Context, address boson.Address, streamName string, msg protobuf.Message) (err error) {
	var stream p2p.Stream
	stream, err = s.getStream(ctx, address, streamName)
	if err != nil {
		s.logger.Error(err)
		return err
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	w := protobuf.NewWriter(stream)
	err = w.WriteMsgWithContext(ctx, msg)
	if err != nil {
		s.logger.Errorf("%s/%s/%s send data to %s %s", protocolName, protocolVersion, streamName, address, err.Error())
		return err
	}
	return nil
}

func (s *Service) Snapshot() *model.KadParams {
	connected, ss := s.kad.SnapshotConnected()

	return &model.KadParams{
		BaseAddr:      s.self,
		Connected:     connected,
		Timestamp:     time.Now(),
		Groups:        s.getModelGroupInfo(),
		ConnectedInfo: s.getConnectedInfo(ss),
	}
}

func (s *Service) getModelGroupInfo() []*model.GroupInfo {
	ss := s.virtualConn.SnapshotNow()
	out := make([]*model.GroupInfo, 0)
	s.groups.Range(func(_, value interface{}) bool {
		v := value.(*Group)
		out = append(out, &model.GroupInfo{
			GroupID:   v.gid,
			Option:    v.option,
			KeepPeers: s.getVirtualConnectedInfo(ss, v.keepPeers.Peers()),
			KnowPeers: s.getVirtualConnectedInfo(ss, v.knownPeers.Peers()),
		})
		return true
	})
	return out
}

func (s *Service) getConnectedInfo(ss map[string]*topModel.PeerInfo) (out []*model.ConnectedInfo) {
	peerInfoFunc := func(list []boson.Address) (infos []*topModel.PeerInfo) {
		infos = make([]*topModel.PeerInfo, 0)
		for _, v := range list {
			val, ok := ss[v.String()]
			if ok {
				infos = append(infos, val)
			} else {
				infos = append(infos, &topModel.PeerInfo{
					Address: v,
					Metrics: &topModel.MetricSnapshotView{},
				})
			}
		}
		return infos
	}
	out = make([]*model.ConnectedInfo, 0)
	s.connectedPeers.Range(func(key, value interface{}) bool {
		gid := boson.MustParseHexAddress(gconv.String(key))
		v := value.(*peerset.PeersManager)
		out = append(out, &model.ConnectedInfo{
			GroupID:        gid,
			Connected:      v.Length(),
			ConnectedPeers: peerInfoFunc(v.Peers()),
		})
		return true
	})
	return out
}

func (s *Service) getVirtualConnectedInfo(ss map[string]model.SnapshotView, list []boson.Address) (now []model.VirtualConnectedInfo) {
	now = make([]model.VirtualConnectedInfo, 0)
	for _, v := range list {
		now = append(now, model.VirtualConnectedInfo{Address: v, Metrics: ss[v.ByteString()]})
	}
	return now
}

func (s *Service) SubscribeLogContent(n *rpc.Notifier, sub *rpc.Subscription) {
	notifier := subscribe.NewNotifier(n, sub)
	_ = s.subPub.Subscribe(notifier, "group", "logContent", "")
	return
}

func (s *Service) notifyLogContent(data LogContent) {
	_ = s.subPub.Publish("group", "logContent", "", data)
}

func (s *Service) GetGroup(groupName string) (out *Group, err error) {
	gid, err := boson.ParseHexAddress(groupName)
	if err != nil {
		gid = GenerateGID(groupName)
		err = nil
	}
	v, ok := s.groups.Load(gid.String())
	if !ok {
		return nil, errors.New("group not found")
	}
	return v.(*Group), nil
}

func (s *Service) getStream(ctx context.Context, dest boson.Address, streamName string) (stream p2p.Stream, err error) {
	if !s.route.IsNeighborContainLightNode(dest) {
		stream, err = s.stream.NewConnChainRelayStream(ctx, dest, nil, protocolName, protocolVersion, streamName)
	} else {
		stream, err = s.stream.NewStream(ctx, dest, nil, protocolName, protocolVersion, streamName)
	}
	if err != nil {
		err = fmt.Errorf("p2p stream create failed, %s", err)
	}
	return
}

func (s *Service) Send(ctx context.Context, data []byte, gid, dest boson.Address) (err error) {
	var stream p2p.Stream
	stream, err = s.getStream(ctx, dest, streamMessage)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()
	req := &pb.GroupMsg{
		Gid:  gid.Bytes(),
		Data: data,
		Type: int32(SendOnly),
	}
	w, r := protobuf.NewWriterAndReader(stream)
	err = w.WriteMsgWithContext(ctx, req)
	if err != nil {
		return err
	}

	res := &pb.GroupMsg{}
	err = r.ReadMsgWithContext(ctx, res)
	if err != nil {
		return
	}
	if res.Err != "" {
		err = fmt.Errorf(res.Err)
		return
	}
	return nil
}

func (s *Service) SendReceive(ctx context.Context, timeout int64, data []byte, gid, dest boson.Address) (result []byte, err error) {
	var stream p2p.Stream
	stream, err = s.getStream(ctx, dest, streamMessage)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()
	req := &pb.GroupMsg{
		Gid:     gid.Bytes(),
		Data:    data,
		Type:    int32(SendReceive),
		Timeout: timeout,
	}
	w, r := protobuf.NewWriterAndReader(stream)
	err = w.WriteMsgWithContext(ctx, req)
	if err != nil {
		return
	}
	res := &pb.GroupMsg{}
	err = r.ReadMsgWithContext(ctx, res)
	if err != nil {
		return
	}
	if res.Err != "" {
		err = fmt.Errorf(res.Err)
		return
	}
	result = res.Data
	_ = w.WriteMsgWithContext(ctx, nil)
	return
}

func (s *Service) GetSendStream(ctx context.Context, gid, dest boson.Address) (out SendStreamCh, err error) {
	var stream p2p.Stream
	stream, err = s.getStream(ctx, dest, streamMessage)
	if err != nil {
		return
	}
	w, r := protobuf.NewWriterAndReader(stream)
	out.Read = make(chan []byte, 1)
	out.ReadErr = make(chan error, 1)
	out.Write = make(chan []byte, 1)
	out.WriteErr = make(chan error, 1)
	out.Close = make(chan struct{}, 1)
	go func() {
		defer stream.Reset()
		for {
			select {
			case d := <-out.Write:
				err = w.WriteMsgWithContext(ctx, &pb.GroupMsg{Data: d, Gid: gid.Bytes(), Type: int32(SendStream)})
				if err != nil {
					out.WriteErr <- err
					return
				}
			case <-out.Close:
				return
			}
		}
	}()
	go func() {
		defer stream.Reset()
		for {
			res := &pb.GroupMsg{}
			err = r.ReadMsgWithContext(ctx, res)
			if err != nil {
				out.ReadErr <- err
				return
			}
			out.Read <- res.Data
		}
	}()
	return
}

func (s *Service) onMessage(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
	r := protobuf.NewReader(stream)
	info := &pb.GroupMsg{}
	err := r.ReadMsgWithContext(ctx, info)
	if err != nil {
		_ = stream.Reset()
		return err
	}

	msg := GroupMessage{
		GID:     boson.NewAddress(info.Gid),
		Data:    info.Data,
		From:    peer.Address,
		Timeout: info.Timeout,
	}

	st := &WsStream{
		done:       make(chan struct{}, 1),
		sendOption: SendOption(info.Type),
		stream:     stream,
		w:          protobuf.NewWriter(stream),
		r:          r,
	}

	var (
		notifyErr  error
		haveNotify bool
	)
	s.groups.Range(func(_, value interface{}) bool {
		g := value.(*Group)
		if g.gid.Equal(msg.GID) && g.option.GType == model.GTypeJoin {
			haveNotify = true
			if g.groupMsgSub == false {
				notifyErr = fmt.Errorf("target group no subscribe msg")
				return false
			}
			_ = s.notifyMessage(g, msg, st)
			return false
		}
		return true
	})
	if !haveNotify {
		notifyErr = fmt.Errorf("target not in the group")
	}
	if notifyErr != nil {
		err = protobuf.NewWriter(stream).WriteMsgWithContext(ctx, &pb.GroupMsg{
			Gid:  info.Gid,
			Type: info.Type,
			Err:  notifyErr.Error(),
		})
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}
	return nil
}

func (s *Service) SubscribeGroupMessage(n *rpc.Notifier, sub *rpc.Subscription, gid boson.Address) (err error) {
	notifier := subscribe.NewNotifier(n, sub)
	var g *Group
	value, ok := s.groups.Load(gid.String())
	if ok {
		g = value.(*Group)
		if g.option.GType == model.GTypeJoin {
			g.groupMsgSub = true
			_ = s.subPub.Subscribe(notifier, "group", "groupMessage", gid.String())
			return nil
		}
	}
	return ErrorGroupNotFound
}

func (s *Service) SubscribeGroupMessageWithChan(notifier *subscribe.NotifierWithMsgChan, gid boson.Address) (err error) {
	value, ok := s.groups.Load(gid.String())
	if ok {
		g := value.(*Group)
		if g.option.GType == model.GTypeJoin {
			g.groupMsgSub = true
			_ = s.subPub.Subscribe(notifier, "group", "groupMessage", gid.String())
			return nil
		}
	}
	return ErrorGroupNotFound
}

func (s *Service) notifyMessage(g *Group, msg GroupMessage, st *WsStream) (e error) {
	if g.groupMsgSub == false {
		return nil
	}
	defer func() {
		err := recover()
		if err != nil {
			e = fmt.Errorf("group %s , notify msg %s", g.gid, err)
			s.logger.Error(e)
			g.groupMsgSub = false
		}
	}()

	if st.sendOption != SendOnly {
		msg.SessionID = rpc.NewID()
	}

	_ = s.subPub.Publish("group", "groupMessage", g.gid.String(), msg)

	s.logger.Debugf("group: sessionID %s %s from %s", msg.SessionID, st.sendOption, msg.From)
	switch st.sendOption {
	case SendOnly:
		err := st.w.WriteMsg(&pb.GroupMsg{})
		if err != nil {
			s.logger.Tracef("group: sessionID %s reply err %v", msg.SessionID, err)
			_ = st.stream.Reset()
		} else {
			s.logger.Tracef("group: sessionID %s reply success", msg.SessionID)
			go st.stream.FullClose()
		}
	case SendReceive:
		go func() {
			s.sessionStream.Store(msg.SessionID, st)
			defer s.sessionStream.Delete(msg.SessionID)
			timeout := time.Second * time.Duration(msg.Timeout)
			select {
			case <-time.After(timeout):
				s.logger.Debugf("group: sessionID %s timeout %s when wait receive reply from websocket", msg.SessionID, timeout)
				_ = st.stream.Reset()
			case <-st.done:
				_ = st.stream.FullClose()
			}
		}()
		go func() {
			defer close(st.done)
			for {
				var nothing protobuf.Message
				err := st.r.ReadMsg(nothing)
				s.logger.Tracef("group: sessionID %s close from the sender %v", msg.SessionID, err)
				return
			}
		}()
	case SendStream:
	}
	return nil
}

func (s *Service) ReplyGroupMessage(sessionID string, data []byte) (err error) {
	v, ok := s.sessionStream.Load(rpc.ID(sessionID))
	if !ok {
		s.logger.Tracef("group: sessionID %s reply err invalid or has expired", sessionID)
		return fmt.Errorf("sessionID %s is invalid or has expired", sessionID)
	}
	st := v.(*WsStream)
	defer func() {
		if err != nil {
			s.logger.Errorf("group: sessionID %s reply err %v", sessionID, err)
			_ = st.stream.Reset()
		} else {
			switch st.sendOption {
			case SendReceive:
				s.logger.Tracef("group: sessionID %s reply success", sessionID)
			}
		}
	}()
	return st.w.WriteMsg(&pb.GroupMsg{
		Data: data,
	})
}

func (s *Service) subscribeGroupPeers(n *rpc.Notifier, sub *rpc.Subscription, group string) (err error) {
	gid, err := boson.ParseHexAddress(group)
	if err != nil {
		gid = GenerateGID(group)
	}
	notifier := subscribe.NewNotifier(n, sub)
	val, ok := s.groups.Load(gid.String())
	if ok {
		g := val.(*Group)
		_ = s.subPub.Subscribe(notifier, "group", "groupPeers", gid.String())
		go func() {
			peers := g.Peers()
			_ = n.Notify(sub.ID, peers)
		}()
		return nil
	}
	return ErrorGroupNotFound
}

func (s *Service) getOptimumPeers(list []boson.Address) (now []boson.Address) {
	return s.kad.GetPeersWithLatencyEWMA(list)
}

func (s *Service) getOptimumPeersVirtual(list []boson.Address) (now []boson.Address) {
	sortIdx := make([][]int64, len(list))
	for i, v := range list {
		ss := s.virtualConn.SnapshotAddr(v)
		t := ss.LatencyEWMA
		sortIdx[i] = []int64{int64(i), t}
	}

	sort.Slice(sortIdx, func(i, j int) bool {
		if sortIdx[i][1] != sortIdx[j][1] {
			return sortIdx[i][1] < sortIdx[j][1]
		} else {
			return sortIdx[i][0] < sortIdx[j][0]
		}
	})

	for _, v := range sortIdx {
		now = append(now, list[v[0]])
	}
	return now
}

func (s *Service) subscribePeerState(n *rpc.Notifier, sub *rpc.Subscription, group string) error {
	gid, err := boson.ParseHexAddress(group)
	if err != nil {
		gid = GenerateGID(group)
	}
	notifier := subscribe.NewNotifier(n, sub)
	val, ok := s.groups.Load(gid.String())
	if ok {
		g := val.(*Group)
		_ = s.subPub.Subscribe(notifier, "group", "peerState", gid.String())
		go func() {
			peers := g.PeersInfo()
			_ = n.Notify(sub.ID, peers)
		}()
		return nil
	}
	return ErrorGroupNotFound
}
