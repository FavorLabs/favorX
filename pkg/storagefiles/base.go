package storagefiles

import (
	"context"
	"errors"
	"path"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chunkinfo"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/multicast"
	"github.com/FavorLabs/favorX/pkg/settlement/chain/oracle"
)

type Services struct {
	panel *Panel
}

func NewServices(cfg Config, logger logging.Logger, gClient multicast.GroupStorageFiles, subClient *chain.Client,
	chunkInfo chunkinfo.Interface, fileInfo fileinfo.Interface, oracle oracle.Resolver) (*Services, error) {

	addr, err := boson.ParseHexAddress(cfg.GroupName)
	if err != nil {
		cfg.Gid = multicast.GenerateGID(cfg.GroupName)
	} else {
		cfg.Gid = addr
	}

	if cfg.CacheDir == "" {
		return nil, errors.New("storagefiles: param cache-dir is empty")
	}
	if !path.IsAbs(cfg.DataDir) {
		return nil, errors.New("param data-dir must be an absolute path, otherwise disk size calculation will fail")
	}

	p, err := NewPanel(context.Background(), cfg, logger, gClient, subClient, chunkInfo, fileInfo, oracle)
	if err != nil {
		return nil, err
	}
	return &Services{
		panel: p,
	}, nil
}

func (s *Services) Start() {
	s.panel.Start()
}

func (s *Services) Close() error {
	s.panel.Close()
	return nil
}
