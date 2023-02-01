package storagefiles

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/chain/rpc/storage"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/oracle"
)

type Manager struct {
	done      chan struct{}
	nextID    uint64
	logger    logging.Logger
	workers   *list.List
	lock      sync.RWMutex
	disk      *DiskManager
	db        *Database
	wg        sync.WaitGroup
	options   Config
	fileInfo  fileinfo.Interface
	oracle    oracle.Resolver
	subClient *chain.SubChainClient
}

func NewManager(done chan struct{}, cfg Config, dm *DiskManager, logger logging.Logger, fileInfo fileinfo.Interface, oracle oracle.Resolver, subClient *chain.SubChainClient) (*Manager, error) {
	database, err := NewDB(cfg.CacheDir)
	if err != nil {
		logger.Errorf("leveldb init %v", err)
		return nil, err
	}
	s := &Manager{
		options:   cfg,
		done:      done,
		workers:   new(list.List),
		disk:      dm,
		logger:    logger,
		db:        database,
		fileInfo:  fileInfo,
		oracle:    oracle,
		subClient: subClient,
	}
	return s, nil
}

func (m *Manager) workerID() uint64 {
	var next uint64

	for {
		current := atomic.LoadUint64(&m.nextID)

		if current >= math.MaxUint64-1 {
			next = 0
		} else {
			next = current + 1
		}

		if atomic.CompareAndSwapUint64(&m.nextID, current, next) {
			break
		}
	}

	return next
}

func (m *Manager) AddWorker(ctx context.Context, task *Task) *Worker {
	m.lock.Lock()
	defer m.lock.Unlock()
	w := &Worker{
		id:               m.workerID(),
		ctx:              ctx,
		manager:          m,
		task:             task,
		err:              make(chan interface{}, 1),
		retryInitCh:      make(chan error, 1),
		retryProgressCh:  make(chan error, 1),
		retryPinOracleCh: make(chan struct{}, 1),
	}
	w.element = m.workers.PushBack(w)
	return w
}

func (m *Manager) RemainTotal() (total uint64) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for w := m.workers.Front(); w != nil; w = w.Next() {
		wk := w.Value.(*Worker)
		total += uint64(wk.task.Remain())
	}
	return
}

func (m *Manager) TaskingHashes() (hashes []string) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for w := m.workers.Front(); w != nil; w = w.Next() {
		wk := w.Value.(*Worker)
		hashes = append(hashes, wk.task.Info.Hash.String())
	}
	return
}

func (m *Manager) HashWorker(fileHash, source string) *Worker {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for w := m.workers.Front(); w != nil; w = w.Next() {
		wk := w.Value.(*Worker)
		if wk.task.Info.Hash.String() == fileHash && wk.task.Info.Source.String() == source {
			return wk
		}
	}
	return nil
}

func (m *Manager) Wait() {
	m.wg.Wait()
}

type Worker struct {
	id               uint64
	ctx              context.Context
	element          *list.Element
	manager          *Manager
	task             *Task
	err              chan interface{}
	retryInitCh      chan error
	retryProgressCh  chan error
	retryPinOracleCh chan struct{}
}

func (w *Worker) Run() {
	w.manager.logger.Infof("worker %d start, download file %s from %s", w.id, w.task.Info.Hash, w.task.Info.Source)
	go func() {
		err := w.manager.db.SaveTask(w.task)
		if err != nil {
			w.manager.logger.Errorf("worker %d task %s storage %v", w.id, w.task.Display(), err)
		}
		w.manager.wg.Add(1)
		defer func() {
			err = w.manager.db.DeleteTask(w.task)
			if err != nil {
				w.manager.logger.Errorf("task %s delete form db %v", w.task.Display(), err)
			}
			w.manager.lock.Lock()
			w.manager.workers.Remove(w.element)
			w.manager.lock.Unlock()
			w.manager.wg.Done()
		}()
		for {
			select {
			case <-w.retryPinOracleCh:
				_ = w.retryPinOracle()
			case e := <-w.retryInitCh:
				if errors.Is(e, io.ErrUnexpectedEOF) {
					e = fmt.Errorf("maybe aurora p2p is disconnected, %v", e)
				}
				err1 := w.retryInit()
				if err1 != nil {
					w.err <- err1
					break
				}
				w.printProgressInfo()
				w.manager.logger.Debugf("worker %d retry, %d reason %v", w.id, w.task.Running.Retry, e)
			case e := <-w.retryProgressCh:
				if errors.Is(e, io.ErrUnexpectedEOF) {
					e = fmt.Errorf("maybe aurora p2p is disconnected, %v", e)
				}
				err1 := w.retryProgress()
				if err1 != nil {
					w.err <- err1
					break
				}
				w.printProgressInfo()
				w.manager.logger.Debugf("worker %d retry, %d reason %v", w.id, w.task.Running.Retry, e)
			case <-w.ctx.Done():
				w.manager.logger.Infof("worker %d quit, %v", w.id, w.ctx.Err())
				return
			case e := <-w.err:
				w.printProgressInfo()
				if e != nil {
					w.manager.logger.Errorf("worker %d quit, %v", w.id, e)
				} else {
					w.manager.logger.Infof("worker %d quit, upload finish", w.id)
				}
				return
			}
		}
	}()
	w.init()
}

func (w *Worker) init() {
	err := w.manager.subClient.Storage.CheckOrder(w.task.Info.Buyer.Bytes(), w.task.Info.Hash.Bytes(), w.manager.subClient.Default.Signer.PublicKey)
	if err != nil {
		w.manager.logger.Infof("worker %d, init check order err: %s", w.id, w.task.Info.Hash, w.task.Info.Source, err)
		w.err <- err
		return
	}
	size, err := GetSize(w.ctx, w.manager.options.ApiGateway, w.task.Info.Hash.String(), w.task.Info.Source.String())
	if err != nil {
		w.retryInitCh <- err
		return
	}
	if size == 0 {
		w.retryInitCh <- fmt.Errorf("failed to retrieve content length")
		return
	}
	w.task.Running.Size = size

	// check disk
	logicAvail, err := w.manager.disk.LogicAvail()
	if err != nil {
		w.err <- err
		return
	}
	free := logicAvail - w.manager.RemainTotal()
	if free < uint64(size) {
		w.err <- ErrDiskFull
		if free < w.manager.disk.Redundant {
			select {
			case w.manager.done <- struct{}{}:
			default:
			}
		}
		return
	}

	w.progress()
}

func (w *Worker) retryInit() error {
	if !w.task.canRetry() {
		return fmt.Errorf("try to init download file many times")
	}
	w.task.tryAgain(true, func() {
		<-time.After(time.Second * 1)
		w.init()
	})
	return nil
}

func (w *Worker) retryProgress() error {
	if !w.task.canRetry() {
		return fmt.Errorf("try to download file many times")
	}
	w.task.tryAgain(true, func() {
		<-time.After(time.Second * 1)
		w.progress()
	})
	return nil
}

func (w *Worker) progress() {
	err := w.manager.subClient.Storage.CheckOrder(w.task.Info.Buyer.Bytes(), w.task.Info.Hash.Bytes(), w.manager.subClient.Default.Signer.PublicKey)
	if err != nil {
		w.manager.logger.Infof("worker %d, progress check order err: %s", w.id, w.task.Info.Hash, w.task.Info.Source, err)
		w.err <- err
		return
	}
	for !w.task.Complete() {
		logicAvail, err := w.manager.disk.LogicAvail()
		if err != nil {
			w.err <- err
			return
		}
		if logicAvail < uint64(w.task.Remain()) {
			w.err <- ErrDiskFull
			return
		}

		resp, err := DownloadFile(w.ctx, w.manager.options.ApiGateway, w.task.Info.Hash.String(), w.task.Info.Source.String(), w.task.Range())
		if err != nil {
			w.retryProgressCh <- err
			return
		}
		resp.Wait()

		if resp.Err() != nil {
			w.retryProgressCh <- resp.Err()
			return
		}
		w.task.updateSize(resp.BytesComplete())
		w.task.updateTime(resp.Duration().Nanoseconds())
		err = w.manager.db.SaveTask(w.task)
		if err != nil {
			w.manager.logger.Warningf("task %s storege %v", w.task.Display(), err)
		}
	}

	w.pinOracle()
}

func (w *Worker) retryPinOracle() error {
	<-time.After(time.Second * 5)
	w.task.Running.RetryPinOracle++
	w.pinOracle()
	return nil
}

func (w *Worker) pinOracle() {
	err := PinFile(w.ctx, w.manager.options.ApiGateway, w.task.Info.Hash.String())
	if err != nil {
		w.manager.logger.Infof("worker %d pin err %s", w.id, err)
		w.retryPinOracleCh <- struct{}{}
		return
	}
	err = w.oracleRegister()
	if err != nil {
		w.retryPinOracleCh <- struct{}{}
		return
	}
	w.online()
}

func (w *Worker) oracleRegister() (err error) {
	err = w.storageFile(w.task.Info.Hash)
	if err == nil {
		e := w.manager.oracle.Register(w.task.Info.Hash)
		if e != nil {
			w.manager.logger.Errorf("worker %d oracle local file register %s", w.id, e)
		}
		return
	}
	if errors.Is(err, storage.OrderNotFound) {
		return nil
	}
	return err
}

func (w *Worker) storageFile(cid boson.Address) (err error) {
	defer func() {
		if err == nil {
			w.manager.logger.Infof("worker %d oracle on chain success", w.id)
		} else {
			w.manager.logger.Warningf("worker %d oracle on chain %d err %s", w.id, w.task.Running.RetryPinOracle+1, err)
		}
	}()
	w.manager.logger.Infof("worker %d oracle on chain start", w.id)

	return w.manager.subClient.Storage.StorageFileWatch(w.ctx, w.task.Info.Buyer.Bytes(), cid.Bytes())
}

func (w *Worker) online() {
	w.err <- nil
}

func (w *Worker) printProgressInfo() {
	w.manager.logger.Tracef(
		"worker %d downloaded file %s size %.2fMB => %.2f%% %s during %s",
		w.id,
		w.task.Info.Hash,
		float64(w.task.Running.Size)/1024/1024,
		w.task.Progress()*100,
		func() string {
			if w.task.BytesPerSecond() > 1024*1024 {
				return fmt.Sprintf("%.2fMB/s", w.task.BytesPerSecond()/1024/1024)
			} else if w.task.BytesPerSecond() > 1024 {
				return fmt.Sprintf("%.2fKB/s", w.task.BytesPerSecond()/1024)
			}
			return fmt.Sprintf("%.2fB/s", w.task.BytesPerSecond())
		}(),
		w.task.During(),
	)
}
