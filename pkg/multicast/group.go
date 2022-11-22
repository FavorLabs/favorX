package multicast

import (
	"bytes"
	"encoding/json"
	"time"
)

func (g *Group) notifyPeerState(state PeerState) {
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

	peers := g.peers()
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
		_ = g.srv.subPub.Publish("group", "groupPeers", g.gid.String(), g.peers())
	}
	g.groupPeersLastSend = time.Now()
}

func (g *Group) peers() *GroupPeers {
	return &GroupPeers{
		Connected: g.srv.getOptimumPeers(g.connectedPeers.BinPeers(0)),
		Keep:      g.srv.getOptimumPeers(g.keepPeers.BinPeers(0)),
	}
}
