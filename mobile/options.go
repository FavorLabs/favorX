package mobile

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/FavorLabs/favorX/pkg/node"
	"github.com/FavorLabs/favorX/pkg/resolver/multiresolver"
)

// Options represents the collection of configuration values to fine tune the
// node embedded into a mobile process. The available values are a subset of the
// entire API provided by node to reduce the maintenance surface and dev
// complexity.
type Options struct {
	// api setting
	EnableTLS      bool
	ApiPort        int
	DebugAPIPort   int
	EnableDebugAPI bool

	// rpc setting
	WebsocketPort int

	// p2p setup
	NetworkID      int64 // default type uint64
	P2PPort        int
	WelcomeMessage string

	// kademlia
	BinMaxPeers   int
	LightMaxPeers int

	// cache size
	CacheCapacity int64 // default type uint64

	// node bootstrap
	BootNodes      string // default type []string
	EnableDevNode  bool
	EnableFullNode bool

	// chain setting
	ChainEndpoint string

	// traffic stat
	EnableFlowStat bool

	// domain resolver
	ResolverOptions string // default type []string

	// security
	Password string
	DataPath string

	// leveldb opts
	BlockCacheCapacity     int64 // default type uint64
	OpenFilesLimit         int64 // default type uint64
	WriteBufferSize        int64 // default type uint64
	DisableSeeksCompaction bool

	// misc
	Verbosity string
}

// defaultOptions contains the default node configuration values to use if all
// or some fields are missing from the user's specified list.
var defaultOptions = &Options{
	EnableTLS:          true,
	ApiPort:            1633,
	DebugAPIPort:       1635,
	WebsocketPort:      1637,
	P2PPort:            1634,
	CacheCapacity:      4000,
	EnableFullNode:     false,
	BinMaxPeers:        20,
	LightMaxPeers:      100,
	BlockCacheCapacity: 8 * 1024 * 1024,
	OpenFilesLimit:     1000,
	WriteBufferSize:    4 * 1024 * 1024,
	Verbosity:          "info",
}

const listenAddress = "localhost"

func (o Options) DataDir(c *node.Options) {
	c.DataDir = o.DataPath
}

func (o Options) APIAddr(c *node.Options) {
	c.APIAddr = fmt.Sprintf("%s:%d", listenAddress, o.ApiPort)
}

func (o Options) EnableApiTLS(c *node.Options) {
	c.EnableApiTLS = o.EnableTLS
}

func (o Options) DebugAPIAddr(c *node.Options) {
	if o.EnableDebugAPI {
		c.DebugAPIAddr = fmt.Sprintf("%s:%d", listenAddress, o.DebugAPIPort)
	}
}

func (o Options) WSAddr(c *node.Options) {
	c.WSAddr = fmt.Sprintf("%s:%d", listenAddress, o.WebsocketPort)
	c.CORSAllowedOrigins = []string{"*"}
}

func (o Options) Bootnodes(c *node.Options) {
	bootNodes := strings.Split(o.BootNodes, ",")
	c.Bootnodes = append(c.Bootnodes, bootNodes...)
}

func (o Options) ResolverConnectionCfgs(c *node.Options) {
	resolverOptions := strings.Split(o.ResolverOptions, ",")
	resolverCfgs, err := multiresolver.ParseConnectionStrings(resolverOptions)
	if err == nil {
		c.ResolverConnectionCfgs = resolverCfgs
	}
}

func (o Options) IsDev(c *node.Options) {
	c.IsDev = o.EnableDevNode
}

func (o Options) KadBinMaxPeers(c *node.Options) {
	c.KadBinMaxPeers = o.BinMaxPeers
}

func (o Options) LightNodeMaxPeers(c *node.Options) {
	c.LightNodeMaxPeers = o.LightMaxPeers
}

func (o Options) TrafficEnable(c *node.Options) {
	c.TrafficEnable = o.EnableFlowStat
}

// Export exports Options to node.Options, skipping all other extra fields
func (o *Options) export() (c node.Options) {
	localVal := reflect.ValueOf(o).Elem()
	remotePtr := reflect.ValueOf(&c)
	remoteVal := reflect.ValueOf(&c).Elem()
	remoteType := reflect.TypeOf(&c).Elem()

	for i := 0; i < remoteVal.NumField(); i++ {
		remoteFieldVal := remoteVal.Field(i)
		localFieldVal := localVal.FieldByName(remoteType.Field(i).Name)
		if reflect.ValueOf(localFieldVal).IsZero() {
			localMethod := localVal.MethodByName(remoteType.Field(i).Name)
			if localMethod.IsValid() {
				localMethod.Call([]reflect.Value{remotePtr})
			}
		} else if localFieldVal.IsValid() {
			if remoteFieldVal.IsValid() && remoteFieldVal.Type() == localFieldVal.Type() {
				remoteFieldVal.Set(localFieldVal)
			}
		}
	}

	return remoteVal.Interface().(node.Options)
}
