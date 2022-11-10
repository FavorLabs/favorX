package storagefiles

import (
	"fmt"
	"net/http"
	"time"
)

type Task struct {
	Info    UploadRequest `json:"info"` // Note that this field is compatible with version 1, leveldb storage
	Option  Option
	Running RunningParams
}

type RunningParams struct {
	Size           int64
	Time           int64
	CurSize        int64
	Retry          int // progress retry count
	RetryPinOracle int // pin/oracle retry count
}

type Option struct {
	CacheBuffer int
	Retry       int // allow progress retry count
	Force       bool
}

func (t *Task) Display() string {
	return t.Info.Hash.String() + "," + t.Info.Source.String()
}

func (t *Task) SetRequest(req UploadRequest) *Task {
	t.Info = req
	return t
}

func (t *Task) SetOption(o Option) *Task {
	t.Option = o
	return t
}

func (t *Task) canRetry() bool {
	return t.Option.Retry > t.Running.Retry
}

func (t *Task) tryAgain(async bool, fn func()) {
	t.Running.Retry++

	if async {
		go fn()
		return
	}
	fn()
}

func (t *Task) updateSize(val int64) {
	t.Running.CurSize += val
}

func (t *Task) updateTime(val int64) {
	t.Running.Time += val
}

func (t *Task) Range() http.Header {
	h := make(http.Header)

	if t.Running.Size-t.Running.CurSize > int64(t.Option.CacheBuffer) {
		h["Range"] = []string{fmt.Sprintf("bytes=%d-%d", t.Running.CurSize, t.Running.CurSize+int64(t.Option.CacheBuffer))}
	} else {
		h["Range"] = []string{fmt.Sprintf("bytes=%d-", t.Running.CurSize)}
	}
	return h
}

func (t *Task) During() time.Duration {
	return time.Duration(t.Running.Time)
}

func (t *Task) BytesPerSecond() float64 {
	return float64(t.Running.Size) / (time.Duration(t.Running.Time).Seconds())
}

func (t *Task) Progress() float64 {
	if t.Running.Size <= 0 {
		return 0
	}
	return float64(t.Running.CurSize) / float64(t.Running.Size)
}

func (t *Task) Complete() bool {
	return t.Running.CurSize >= t.Running.Size
}

func (t *Task) Remain() (val int64) {
	defer func() {
		if val < 0 {
			val = 0
		}
	}()

	return t.Running.Size - t.Running.CurSize
}
