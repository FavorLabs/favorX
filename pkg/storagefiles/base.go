package storagefiles

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/base"
	"github.com/FavorLabs/favorX/pkg/chunkinfo"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/oracle"
	"github.com/FavorLabs/favorX/pkg/subscribe"
)

type Services struct {
	panel  *Panel
	signer crypto.Signer
}

func CheckAndUnRegisterMerchant(signer crypto.Signer, subClient *chain.SubChainClient) error {
	info, err := subClient.Storage.GetMerchantInfo(signer.Public().Encode())
	if err != nil && !errors.Is(err, base.KeyEmptyError) {
		return fmt.Errorf("merchant info check err %s", err)
	}
	if info != nil {
		err = subClient.Storage.MerchantUnregisterWatch(context.Background())
		if err != nil {
			return fmt.Errorf("merchant unregister err %s", err)
		}
	}
	return nil
}

func NewServices(cfg Config, signer crypto.Signer, logger logging.Logger, subPub subscribe.SubPub, subClient *chain.SubChainClient,
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

	p, err := NewPanel(context.Background(), cfg, dm, logger, subPub, subClient, chunkInfo, fileInfo, oracle)
	if err != nil {
		return nil, err
	}
	return &Services{
		panel:  p,
		signer: signer,
	}, nil
}

func (s *Services) Start() {
	for {
		info, e := s.panel.manager.subClient.Storage.GetMerchantInfo(s.signer.Public().Encode())
		if e != nil && !errors.Is(e, base.KeyEmptyError) {
			s.panel.logger.Errorf("merchant info check err %s", e)
			<-time.After(time.Second * 5)
			continue
		}
		if info == nil || uint64(info.DiskTotal) != s.panel.options.Capacity {
			e = s.panel.manager.subClient.Storage.MerchantRegisterWatch(context.Background(), s.panel.options.Capacity)
			if e != nil {
				s.panel.logger.Errorf("merchant register err %s", e)
				<-time.After(time.Second * 5)
				continue
			}
			s.panel.logger.Infof("merchant register successful")
		}
		break
	}
	s.panel.Start()
}

func (s *Services) Close() error {
	s.panel.Close()
	return nil
}
