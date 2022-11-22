package model

import (
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	md "github.com/FavorLabs/favorX/pkg/topology/model"
)

// MetricSnapshotView represents snapshot of metrics counters in more human-readable form.
type MetricSnapshotView struct {
	LastSeenTimestamp          int64   `json:"lastSeenTimestamp"`
	SessionConnectionRetry     uint64  `json:"sessionConnectionRetry"`
	ConnectionTotalDuration    float64 `json:"connectionTotalDuration"`
	SessionConnectionDuration  float64 `json:"sessionConnectionDuration"`
	SessionConnectionDirection string  `json:"sessionConnectionDirection"`
}

type SnapshotView struct {
	Timeout                    int64                      `json:"timeout"`
	LastSeenTimestamp          int64                      `json:"lastSeenTimestamp"`
	SessionConnectionDirection md.PeerConnectionDirection `json:"sessionConnectionDirection"`
	LatencyEWMA                int64                      `json:"latencyEWMA"`
}

type ConnectedInfo struct {
	GroupID        boson.Address  `json:"gid"`
	Connected      int            `json:"connected"`
	ConnectedPeers []*md.PeerInfo `json:"connectedPeers"`
}

type VirtualConnectedInfo struct {
	Address boson.Address `json:"address"`
	Metrics SnapshotView  `json:"metrics,omitempty"`
}

type GroupInfo struct {
	GroupID   boson.Address          `json:"gid"`
	Option    ConfigNodeGroup        `json:"option"`
	KeepPeers []VirtualConnectedInfo `json:"keepPeers"`
	KnowPeers []VirtualConnectedInfo `json:"knowPeers"`
}
type KadParams struct {
	BaseAddr      boson.Address    `json:"baseAddr"`
	Connected     int              `json:"connected"`     // connected count
	Timestamp     time.Time        `json:"timestamp"`     // now
	Groups        []*GroupInfo     `json:"groups"`        // known
	ConnectedInfo []*ConnectedInfo `json:"connectedInfo"` // connected info
}

type ConfigNetDomain struct {
	Domain string `json:"domain"`
	Addr   string `json:"addr"`
}
type ConfigNodeGroup struct {
	Name               string            `json:"name"`
	GType              GType             `json:"type,omitempty"` // 0 join 1 observe
	KeepConnectedPeers int               `json:"keep-connected-peers"`
	KeepPingPeers      int               `json:"keep-ping-peers"`
	Nodes              []boson.Address   `json:"nodes,omitempty"`
	AgentHttp          []ConfigNetDomain `json:"agent-http,omitempty"`
	AgentWS            []ConfigNetDomain `json:"agent-ws,omitempty"`
}

type GType int

const (
	GTypeJoin GType = iota
	GTypeObserve
)
