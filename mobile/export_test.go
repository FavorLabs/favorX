package mobile

import (
	"math/rand"
	"os"

	"github.com/FavorLabs/favorX/pkg/node"
)

func ExportOptions(o *Options) node.Options {
	return o.export()
}

// export defaultConfig for test
func ExportDefaultConfig() (o *Options, err error) {
	o = defaultOptions

	// random listen port
	o.ApiPort = rand.Intn(1234) + 23000
	o.P2PPort = o.ApiPort + 1
	o.DebugAPIPort = o.P2PPort + 1

	o.DataPath, err = os.MkdirTemp(os.TempDir(), "favorX_test")
	if err != nil {
		return nil, err
	}

	// dev chain
	o.ChainEndpoint = "ws://127.0.0.1:9944"

	return
}
