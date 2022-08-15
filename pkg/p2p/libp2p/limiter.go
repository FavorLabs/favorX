package libp2p

import (
	rcmgr "github.com/libp2p/go-libp2p-resource-manager"
)

var (
	connFraction   = 10
	streamFraction = 8
)

func newLimier(o Options) rcmgr.Limiter {
	if o.LightNodeLimit < 1 {
		o.LightNodeLimit = 100
	}
	if o.KadBinMaxPeers < 1 {
		o.KadBinMaxPeers = 20
	}
	limit := rcmgr.DefaultLimits

	limit.SystemBaseLimit.ConnsOutbound = o.KadBinMaxPeers * connFraction
	limit.SystemBaseLimit.ConnsInbound = limit.SystemBaseLimit.ConnsOutbound/2 + o.LightNodeLimit
	limit.SystemBaseLimit.Conns = limit.SystemBaseLimit.ConnsOutbound + limit.SystemBaseLimit.ConnsInbound
	limit.SystemBaseLimit.FD = limit.SystemBaseLimit.Conns
	limit.SystemBaseLimit.StreamsInbound = limit.SystemBaseLimit.ConnsInbound * streamFraction
	limit.SystemBaseLimit.StreamsOutbound = limit.SystemBaseLimit.ConnsOutbound * streamFraction
	limit.SystemBaseLimit.Streams = limit.SystemBaseLimit.StreamsInbound + limit.SystemBaseLimit.StreamsOutbound
	limit.SystemLimitIncrease.Memory = 2 << 30

	limit.ServiceBaseLimit = limit.SystemBaseLimit
	limit.ProtocolBaseLimit = limit.SystemBaseLimit

	limit.ServicePeerBaseLimit.StreamsInbound = 512
	limit.ServicePeerBaseLimit.StreamsOutbound = 512
	limit.ServicePeerBaseLimit.Streams = 1024

	limit.ProtocolPeerBaseLimit = limit.ServicePeerBaseLimit

	return rcmgr.NewFixedLimiter(limit.AutoScale())
}
