package storagefiles

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/chunkinfo"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/settlement/chain/oracle"
	"github.com/FavorLabs/favorX/pkg/subscribe"
)

type Services struct {
	panel *Panel
}

func CheckAndUnRegisterMerchant(my boson.Address, subClient *chain.Client) error {
	info, err := subClient.Storage.GetMerchantInfo(my.Bytes())
	if err != nil && !errors.Is(err, base.KeyEmptyError) {
		return err
	}
	if info != nil {
		err = subClient.Storage.MerchantUnregisterWatch(context.Background())
		return fmt.Errorf("merchant unregister err %s", err)
	}
	return nil
}

func NewServices(cfg Config, logger logging.Logger, subPub subscribe.SubPub, subClient *chain.Client,
	chunkInfo chunkinfo.Interface, fileInfo fileinfo.Interface, oracle oracle.Resolver) (*Services, error) {

	if cfg.CacheDir == "" {
		return nil, errors.New("storagefiles: param cache-dir is empty")
	}
	if !path.IsAbs(cfg.DataDir) {
		return nil, errors.New("param data-dir must be an absolute path, otherwise disk size calculation will fail")
	}

	dm, err := NewDiskManager(cfg)
	if err != nil {
		return nil, err
	}
	freeDisk, err := dm.LogicAvail()
	if err != nil {
		return nil, err
	}
	if freeDisk < 10<<30 {
		return nil, errors.New("the available storage space is less than 10GB")
	}

	info, err := subClient.Storage.GetMerchantInfo(cfg.Overlay.Bytes())
	if err != nil && !errors.Is(err, base.KeyEmptyError) {
		return nil, err
	}
	if info == nil {
		err = subClient.Storage.MerchantRegisterWatch(context.Background(), freeDisk)
		if err != nil {
			return nil, fmt.Errorf("merchant register err %s", err)
		}
	}

	p, err := NewPanel(context.Background(), cfg, dm, logger, subPub, subClient, chunkInfo, fileInfo, oracle)
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
