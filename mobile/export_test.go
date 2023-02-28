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
	o.OracleContract = "0x7F578e5ade91A30aC8ABf120d102E282821bd142"
	o.ChainEndpoint = "https://data-seed-prebsc-1-s1.binance.org:8545"

	o.Group = "[" +
		"{\"name\":\"dao-proxy\",\"type\":1,\"keep-connected-peers\":1,\"nodes\":[\"5e93f90b5c5ab366080d81a7eb25cb20428432a950d24b44f82639673bee14dd\"]}" +
		"]"

	return
}
