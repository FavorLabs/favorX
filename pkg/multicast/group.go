package multicast

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/multicast/peerset"
)

type Group struct {
	gid            boson.Address
	connectedPeers *peerset.PeersManager
	keepPeers      *peerset.PeersManager // Need to maintain the connection with ping
	knownPeers     *peerset.PeersManager
	keptInPeers    *peerset.PeersManager
	srv            *Service
	option         model.ConfigNodeGroup

	multicastSub bool
	groupMsgSub  bool

	groupPeersLastSend time.Time     // groupPeersMsg last send time
	groupPeersSending  chan struct{} // whether a goroutine is sending msg. This chan needs to be declared whit "make(chan struct{}, 1)"

	once           sync.Once
	quit           chan struct{}
	chanCheckKept  chan eventCheckKept
	chanEvent      chan eventPeerAction
	chanNotifyFunc chan func()
}

type eventCheckKept struct {
	addr  boson.Address
	event checkAction
}

type checkAction int

const (
	EventCheckKeptAdd checkAction = iota
	EventCheckKeptRemove
)

type eventPeerAction struct {
	addr  boson.Address
	event peerAction
}

type peerAction int

const (
	EventAddConn peerAction = iota
	EventAddKept
	EventAddKnown
	EventDisconnect
	EventDisconnectVirtual
	EventRemove
)

func (g *Group) notifyPeerState(state PeerState) {
	if state.Type == 0 {
		info := g.srv.kad.SnapshotAddr(state.Overlay)
		state.Direction = info.SessionConnectionDirection
	} else if state.Type == 1 {
		info := g.srv.virtualConn.SnapshotAddr(state.Overlay)
		state.Direction = info.SessionConnectionDirection
	}

	_ = g.srv.subPub.Publish("group", "peerState", g.gid.String(), state)
}

func (g *Group) notifyGroupPeers() {
	// prevent other goroutine from continuing
	select {
	case g.groupPeersSending <- struct{}{}:
	default:
		return
	}

	defer func() {
		<-g.groupPeersSending
	}()

	peers := g.Peers()
	key := "notifyGroupPeers"
	b, _ := json.Marshal(peers)
	has, _ := cache.Contains(cacheCtx, key)
	if has {
		v := cache.MustGet(cacheCtx, key)
		if bytes.Equal(b, v.Bytes()) {
			return
		}
	}
	_ = cache.Set(cacheCtx, key, b, 0)

	var minInterval = time.Millisecond * 500
	ms := time.Since(g.groupPeersLastSend)
	if ms >= minInterval {
		_ = g.srv.subPub.Publish("group", "groupPeers", g.gid.String(), peers)
	} else {
		<-time.After(minInterval - ms)
		_ = g.srv.subPub.Publish("group", "groupPeers", g.gid.String(), g.Peers())
	}
	g.groupPeersLastSend = time.Now()
}

func (g *Group) Peers() *GroupPeers {
	return &GroupPeers{
		Connected: g.srv.getOptimumPeers(g.connectedPeers.Peers()),
		Keep:      g.srv.getOptimumPeersVirtual(g.keepPeers.Peers()),
	}
}

func (g *Group) PeersInfo() *GroupPeersInfo {
	list := g.Peers()
	out := &GroupPeersInfo{}
	for _, v := range list.Connected {
		out.Directed = append(out.Directed, PeerInfo{
			Overlay:   v,
			Direction: g.srv.kad.SnapshotAddr(v).SessionConnectionDirection,
		})
	}
	for _, v := range list.Keep {
		out.Virtually = append(out.Virtually, PeerInfo{
			Overlay:   v,
			Direction: g.srv.virtualConn.SnapshotAddr(v).SessionConnectionDirection,
		})
	}
	return out
}

func (g *Group) addFoundPeers(peers []boson.Address) {
	for _, p := range peers {
		g.chanEvent <- eventPeerAction{
			addr:  p,
			event: EventAddKnown,
		}
	}
}

func (g *Group) Close() {
	g.once.Do(func() {
		close(g.quit)
	})
}

func (g *Group) manage() {
	go func() {
		tick := time.NewTimer(time.Second)
		for {
			select {
			case <-g.quit:
				return
			case <-tick.C:
				g.keptInPeers.Iter(func(addr boson.Address) {
					info := g.srv.virtualConn.GetInfo(addr)
					if info != nil && info.State == peerset.ShakeDone && time.Since(info.LastSeenTime).Milliseconds() > info.MinTimeout*1e3*2 {
						// timeout
						g.srv.logger.Tracef("keep ping %s timeout %ds", addr, info.MinTimeout*2)
						g.chanEvent <- eventPeerAction{
							addr:  addr,
							event: EventDisconnectVirtual,
						}
					}
				})
				tick.Reset(time.Second)
			}
		}
	}()
	for {
		select {
		case <-g.quit:
			return
		case ev := <-g.chanCheckKept:
			switch ev.event {
			case EventCheckKeptAdd:
				g.keptInPeers.Add(ev.addr)
			case EventCheckKeptRemove:
				g.keptInPeers.Remove(ev.addr)
			}
		case ev := <-g.chanEvent:
			switch ev.event {
			case EventDisconnectVirtual:
				g.chanCheckKept <- eventCheckKept{
					addr:  ev.addr,
					event: EventCheckKeptRemove,
				}
				g.srv.virtualConn.Clean(ev.addr)
				g.keepPeers.RemoveAndNotify(ev.addr, func() {
					g.notifyPeerState(PeerState{Overlay: ev.addr, Action: 1, Type: 1})
					g.notifyGroupPeers()
					g.chanEvent <- eventPeerAction{
						addr:  ev.addr,
						event: EventAddKnown,
					}
				})
			case EventDisconnect:
				g.connectedPeers.RemoveAndNotify(ev.addr, func() {
					g.notifyPeerState(PeerState{Overlay: ev.addr, Action: 1, Type: 0})
					g.notifyGroupPeers()
					g.chanEvent <- eventPeerAction{
						addr:  ev.addr,
						event: EventAddKnown,
					}
				})
			case EventAddConn:
				g.srv.virtualConn.Clean(ev.addr)
				// remove known
				g.knownPeers.Remove(ev.addr)
				// remove kept
				var notify bool
				g.keepPeers.RemoveAndNotify(ev.addr, func() {
					g.notifyPeerState(PeerState{Overlay: ev.addr, Action: 1, Type: 1})
					notify = true
				})
				// add conn
				g.connectedPeers.AddAndNotify(ev.addr, func() {
					g.notifyPeerState(PeerState{Overlay: ev.addr, Action: 0, Type: 0})
					notify = true
				})
				if notify {
					g.notifyGroupPeers()
				}
			case EventAddKept:
				g.srv.virtualConn.BindGroup(ev.addr, g.gid, func() {
					g.chanCheckKept <- eventCheckKept{
						addr:  ev.addr,
						event: EventCheckKeptAdd,
					}
				})
				// remove known
				g.knownPeers.Remove(ev.addr)
				// add kept
				g.keepPeers.AddAndNotify(ev.addr, func() {
					g.notifyPeerState(PeerState{Overlay: ev.addr, Action: 0, Type: 1})
					g.notifyGroupPeers()
				})
			case EventAddKnown:
				// check conn and kept
				if !g.connectedPeers.Exists(ev.addr) && !g.keepPeers.Exists(ev.addr) {
					if g.knownPeers.Length() >= maxKnownPeers {
						g.knownPeers.RemoveFirst()
					}
					g.knownPeers.Add(ev.addr)
				}
			case EventRemove:
				g.srv.virtualConn.RemoveGroup(ev.addr, g.gid, func() {
					g.chanCheckKept <- eventCheckKept{
						addr:  ev.addr,
						event: EventCheckKeptRemove,
					}
				})
				g.knownPeers.Remove(ev.addr)
				var notify bool
				g.keepPeers.RemoveAndNotify(ev.addr, func() {
					g.notifyPeerState(PeerState{Overlay: ev.addr, Action: 1, Type: 1})
					notify = true
				})
				g.connectedPeers.RemoveAndNotify(ev.addr, func() {
					g.notifyPeerState(PeerState{Overlay: ev.addr, Action: 1, Type: 0})
					notify = true
				})
				if notify {
					g.notifyGroupPeers()
				}
			}
		case fn := <-g.chanNotifyFunc:
			fn()
		}
	}
}
