package mobile

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	fx "github.com/FavorLabs/favorX"
	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/keystore/p2pkey"
	"github.com/FavorLabs/favorX/pkg/keystore/subkey"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/node"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/sirupsen/logrus"
)

type Node struct {
	node   *node.Favor
	opts   *Options
	logger logging.Logger
}

func Version() string {
	return fx.Version
}

func NewNode(o *Options) (*Node, error) {
	logger, err := newLogger(o.Verbosity)
	if err != nil {
		return nil, err
	}

	// put keys into dataDir
	keyPath := filepath.Join(o.DataPath, "keys")

	sig, err := configureSigner(keyPath, o.Password, uint64(o.NetworkID), logger)
	if err != nil {
		return nil, err
	}

	logger.Infof("version: %v", Version())

	mode := address.NewModel()
	if o.EnableFullNode {
		mode.SetMode(address.FullNode)
		logger.Info("start node mode full.")
	} else {
		logger.Info("start node mode light.")
	}

	config := o.export()
	p2pAddr := fmt.Sprintf("%s:%d", listenAddress, o.P2PPort)

	favorXNode, err := node.NewNode(mode, p2pAddr, uint64(o.NetworkID), logger, sig, config)
	if err != nil {
		return nil, err
	}

	return &Node{node: favorXNode, opts: o, logger: logger}, nil
}

func (n *Node) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return n.node.Shutdown(ctx)
}

func configureSigner(path, password string, networkID uint64, logger logging.Logger) (*crypto.SignerConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("keystore directory not provided")
	}

	kp, created, err := subkey.New(path).Key("boson", password)
	if err != nil {
		return nil, fmt.Errorf("boson key: %w", err)
	}

	var addr boson.Address
	addr, err = crypto.NewOverlayAddress(kp.Public().Encode(), networkID)
	if err != nil {
		return nil, err
	}

	if created {
		logger.Infof("new boson network address created: %s", addr)
	} else {
		logger.Infof("using existing boson network address: %s", addr)
	}

	logger.Infof("boson public key %s", kp.Public().Hex())
	logger.Infof("boson ss58 address %s", kp.Public().Address())

	libp2pPrivateKey, created, err := p2pkey.New(path).Key("libp2p", password)
	if err != nil {
		return nil, fmt.Errorf("libp2p key: %w", err)
	}
	if created {
		logger.Debugf("new libp2p key created")
	} else {
		logger.Debugf("using existing libp2p key")
	}

	keyPair, err := signature.KeyringPairFromSecret(fmt.Sprintf("0x%x", kp.GetSecretKey64()), 42)
	if err != nil {
		return nil, fmt.Errorf("subkey keyPair: %w", err)
	}

	return &crypto.SignerConfig{
		Signer:           kp,
		Overlay:          addr,
		Libp2pPrivateKey: libp2pPrivateKey,
		SubKey:           keyPair,
	}, nil
}

func cmdOutput() io.Writer {
	return os.Stdout
}

func newLogger(verbosity string) (logging.Logger, error) {
	var logger logging.Logger
	switch verbosity {
	case "0", "silent":
		logger = logging.New(io.Discard, 0)
	case "1", "error":
		logger = logging.New(cmdOutput(), logrus.ErrorLevel)
	case "2", "warn":
		logger = logging.New(cmdOutput(), logrus.WarnLevel)
	case "3", "info":
		logger = logging.New(cmdOutput(), logrus.InfoLevel)
	case "4", "debug":
		logger = logging.New(cmdOutput(), logrus.DebugLevel)
	case "5", "trace":
		logger = logging.New(cmdOutput(), logrus.TraceLevel)
	default:
		return nil, fmt.Errorf("unknown verbosity level %q", verbosity)
	}

	return logger, nil
}
