package storagefiles

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/settlement/chain/oracle"
)

type Manager struct {
	done     chan struct{}
	nextID   uint64
	logger   logging.Logger
	workers  *list.List
	lock     sync.RWMutex
	disk     *DiskManager
	db       *Database
	wg       sync.WaitGroup
	options  Config
	fileInfo fileinfo.Interface
	oracle   oracle.Resolver
}

func NewManager(done chan struct{}, cfg Config, dm *DiskManager, logger logging.Logger, fileInfo fileinfo.Interface, oracle oracle.Resolver) (*Manager, error) {
	database, err := NewDB(cfg.CacheDir)
	if err != nil {
		logger.Errorf("leveldb init %v", err)
		return nil, err
	}
	s := &Manager{
		options:  cfg,
		done:     done,
		workers:  new(list.List),
		disk:     dm,
		logger:   logger,
		db:       database,
		fileInfo: fileInfo,
		oracle:   oracle,
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

func (m *Manager) AddWorker(ctx context.Context, task *Task, replyChan chan GroupMessageReply) *Worker {
	m.lock.Lock()
	defer m.lock.Unlock()
	w := &Worker{
		id:               m.workerID(),
		ctx:              ctx,
		manager:          m,
		task:             task,
		err:              make(chan interface{}, 1),
		retryProgressCh:  make(chan error, 1),
		retryPinOracleCh: make(chan struct{}, 1),
		replyChan:        replyChan,
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
		hashes = append(hashes, wk.task.Info.Hash)
	}
	return
}

func (m *Manager) HashWorker(fileHash string) *Worker {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for w := m.workers.Front(); w != nil; w = w.Next() {
		wk := w.Value.(*Worker)
		if wk.task.Info.Hash == fileHash {
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
	retryProgressCh  chan error
	retryPinOracleCh chan struct{}
	replyChan        chan GroupMessageReply
}

func (w *Worker) Run() {
	w.manager.logger.Infof("worker %d start, sessionID=%q download file %s", w.id, w.task.Session, w.task.Info.Hash)
	go func() {
		err := w.manager.db.SaveTask(w.task.Session, w.task)
		if err != nil {
			w.manager.logger.Errorf("worker %d task %s storage %v", w.id, w.task.Session, err)
		}
		w.manager.wg.Add(1)
		defer func() {
			err := w.manager.db.DeleteTask(w.task.Session)
			if err != nil {
				w.manager.logger.Errorf("task %s delete form db %v", w.task.Session, err)
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
					switch er := e.(type) {
					case Error:
						w.manager.logger.Warningf("worker %d quit, %v", w.id, e)
						if er.IsPublic() {
							w.replyGroup(e)
						}
					default:
						w.manager.logger.Errorf("worker %d quit, %v", w.id, e)
					}
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
	size, err := GetSize(w.ctx, w.manager.options.ApiGateway, w.task.Info.Hash, w.task.Info.Source)
	if err != nil {
		w.err <- WrapError(1000, err)
		return
	}
	if size == 0 {
		w.err <- WrapError(1000, fmt.Errorf("failed to retrieve content length"))
		return
	}
	w.task.Running.Size = size

	// check disk
	logicAvail, err := w.manager.disk.LogicAvail()
	if err != nil {
		w.err <- WrapError(1000, err)
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
	firstDownload := true

	for !w.task.Complete() {
		logicAvail, err := w.manager.disk.LogicAvail()
		if err != nil {
			w.err <- WrapError(1000, err)
			return
		}
		if logicAvail < uint64(w.task.Remain()) {
			w.err <- ErrDiskFull
			return
		}

		if firstDownload {
			cid := boson.MustParseHexAddress(w.task.Info.Hash)
			res := w.manager.fileInfo.GetChunkInfoServerOverlays(cid)
			for _, b := range res {
				if b.Overlay == w.manager.options.Overlay.String() {
					w.replyStartDownload(b.Bit)
					break
				}
			}
			firstDownload = false
		}

		resp, err := DownloadFile(w.ctx, w.manager.options.ApiGateway, w.task.Info.Hash, w.task.Info.Source, w.task.Range())
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
		err = w.manager.db.SaveTask(w.task.Session, w.task)
		if err != nil {
			w.manager.logger.Warningf("task %s storege %v", w.task.Session, err)
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
	err := PinFile(w.ctx, w.manager.options.ApiGateway, w.task.Info.Hash)
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
	if w.manager.options.SkipOracleRegister {
		w.manager.logger.Infof("worker %d oracle register skipped", w.id)
		return nil
	}
	defer func() {
		if err == nil {
			w.manager.logger.Infof("worker %d oracle register success", w.id)
		} else {
			w.manager.logger.Warningf("worker %d oracle register %d err %s", w.id, w.task.Running.RetryPinOracle+1, err)
		}
	}()
	w.manager.logger.Infof("worker %d oracle register start", w.id)
	cid := boson.MustParseHexAddress(w.task.Info.Hash)
	have := w.manager.oracle.GetNodesFromCid(cid.Bytes())
	for _, v := range have {
		if v.Equal(w.manager.options.Overlay) {
			return nil
		}
	}
	err = w.manager.oracle.RegisterCidAndNode(w.ctx, cid, w.manager.options.Overlay, nil, nil)
	if err != nil {
		return err
	}
	err = w.manager.fileInfo.RegisterFile(cid, true)
	if err != nil {
		return err
	}
	return nil
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

func (w *Worker) replyStartDownload(vector address.BitVectorApi, session ...string) {
	var notify UploadResponse
	notify.WorkerID = w.id
	notify.Vector = vector
	notify.Message = "file upload started"
	w.replyGroup(notify, session...)
}

func (w *Worker) replyGroup(notify interface{}, session ...string) {
	data, err := json.Marshal(notify)
	if err != nil {
		w.manager.logger.Errorf("worker %d reply group message: marshal notify %v", w.id, err)
	}
	w.manager.logger.Tracef("worker %d reply group message: %s", w.id, data)
	ch := make(chan error, 1)
	chData := GroupMessageReply{
		SessionID: func() string {
			if len(session) > 0 {
				return session[0]
			}
			return w.task.Session
		}(),
		Data:  data,
		ErrCh: ch,
	}
	w.replyChan <- chData
	err = <-ch
	if err != nil {
		w.manager.logger.Errorf("worker %d sessionID %s reply group message: %v", w.id, chData.SessionID, err)
	}
	w.manager.logger.Tracef("worker %d sessionID %s reply group message: success", w.id, chData.SessionID)
}
