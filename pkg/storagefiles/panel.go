package storagefiles

import (
	"context"
	"time"

	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chunkinfo"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/localstore/filestore"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/multicast"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/settlement/chain/oracle"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/prometheus/client_golang/prometheus"
)

type Panel struct {
	done            chan struct{}
	quitCh          chan struct{}
	ctx             context.Context
	logger          logging.Logger
	options         Config
	gClient         multicast.GroupStorageFiles
	notify          *subscribe.NotifierWithMsgChan
	manager         *Manager
	metricsRegistry *prometheus.Registry
	chunkInfo       chunkinfo.Interface
	fileInfo        fileinfo.Interface
}

func NewPanel(ctx context.Context, cfg Config, logger logging.Logger, gClient multicast.GroupStorageFiles, subClient *chain.Client,
	chunkInfo chunkinfo.Interface, fileInfo fileinfo.Interface, oracle oracle.Resolver) (*Panel, error) {
	dm, err := NewDiskManager(cfg)
	if err != nil {
		return nil, err
	}
	quit := make(chan struct{}, 1)
	m, err := NewManager(quit, cfg, dm, logger, fileInfo, oracle, subClient)
	if err != nil {
		return nil, err
	}
	p := &Panel{
		options:   cfg,
		ctx:       ctx,
		logger:    logger,
		gClient:   gClient,
		notify:    subscribe.NewNotifierWithMsgChan(),
		manager:   m,
		done:      make(chan struct{}, 1),
		quitCh:    quit,
		chunkInfo: chunkInfo,
		fileInfo:  fileInfo,
	}
	return p, nil
}

func (p *Panel) init() (err error) {
	defer func() {
		if err == nil {
			p.logger.Info("storagefiles: start to watch download job")
		}
	}()

	return p.gClient.SubscribeGroupMessageWithChan(p.notify, p.options.Gid)
}

func (p *Panel) quit() {
	_ = p.gClient.RemoveGroup(p.options.Gid, model.GTypeJoin)
}

func (p *Panel) Start() {
	go p.cleanFiles()

	initErr := make(chan error, 1)
	subErr := make(chan error, 1)
	sub := make(chan struct{}, 1)
	var first = true

	go func() {
		p.logger.Infof("start watch favorX node")
		defer p.logger.Infof("stop watch favorX node")
		for {
			select {
			case e := <-initErr:
				if e != nil {
					<-time.After(time.Second * 3)
					initErr <- p.init()
				} else {
					sub <- struct{}{}
				}
			case e := <-subErr:
				if e != nil {
					initErr <- p.init()
				} else {
					return
				}
			case <-sub:
				if first {
					first = false
					_ = p.manager.db.IterateTask(func(sessionID string, value *Task) (stop bool, err error) {
						go p.manager.AddWorker(p.ctx, value).Run()
						return false, nil
					})
				}
				go p.subOrderMessage(subErr)
			case <-p.done:
				return
			case <-p.quitCh:
				return
			}
		}
	}()

	initErr <- p.init()
}

func (p *Panel) Close() {
	p.quit()
	close(p.done)    // close cron,watch node
	p.manager.Wait() // wait all worker finish
}

func (p *Panel) cleanFiles() {
	interval := time.Duration(p.options.DelFileTime) * time.Minute
	if interval < 1 {
		return
	}
	p.logger.Infof("start cron clean files interval %.f minutes", interval.Minutes())
	ticker := time.NewTimer(interval)
	defer func() {
		ticker.Stop()
		p.logger.Infof("stop cron clean files")
	}()
	retryClean := make(chan error, 1)
	for {
		select {
		case <-p.quitCh:
			return
		case <-p.done:
			return
		case <-ticker.C:
			retryClean <- p.doCleanFiles()
		case e := <-retryClean:
			if e != nil {
				<-time.After(time.Minute)
				retryClean <- p.doCleanFiles()
			}
			ticker.Reset(interval)
		}
	}
}

func (p *Panel) doCleanFiles() (err error) {
	defer func() {
		if err == nil {
			p.logger.Infof("clean files all success")
		} else {
			p.logger.Errorf("clean files failed %v", err)
		}
	}()
	return p.doDelFiles()
}

func (p *Panel) doDelFiles() error {
	var filePage filestore.Page
	var fileFilter []filestore.Filter
	var fileSort filestore.Sort
	files, total := p.fileInfo.GetFileList(filePage, fileFilter, fileSort)
	for _, v := range files {
		if v.Pinned {
			continue
		}
		var ok bool
		for _, h := range p.manager.TaskingHashes() {
			if h == v.RootCid.String() {
				ok = true
				break
			}
		}
		if ok {
			continue
		}

		err := p.fileInfo.DeleteFile(v.RootCid)
		p.chunkInfo.CancelFindChunkInfo(v.RootCid)
		if err != nil {
			p.logger.Errorf("clean file %s err: %v", v.RootCid.String(), err)
		}
	}
	p.logger.Infof("clean files %d/%d success", len(files), total)
	return nil
}

func (p *Panel) subOrderMessage(ch chan error) {
	p.logger.Infof("start watch order message")
	defer p.logger.Infof("stop watch order message")
	for {
		select {
		case req := <-p.notify.MsgChan:
			reqInfo, ok := req.(UploadRequest)
			if !ok {
				break
			}
			wk := p.manager.HashWorker(reqInfo.Hash.String(), reqInfo.Source.String())
			if wk != nil {
				p.logger.Infof("worker %d received repeat fileHash %s request from %s", wk.id, reqInfo.Hash, reqInfo.Source)
				break
			}
			task := new(Task).SetRequest(reqInfo).SetOption(Option{
				CacheBuffer: p.options.BlockSize,
				Retry:       p.options.RetryNumber,
			})
			go p.manager.AddWorker(p.ctx, task).Run()
		case e := <-p.notify.ErrChan:
			if e != nil {
				p.logger.Errorf("order subscribe closed: %v", e)
			} else {
				p.logger.Infof("order subscribe closed")
			}
			ch <- e
			return
		}
	}
}
