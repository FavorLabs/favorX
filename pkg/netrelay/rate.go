package netrelay

import (
	"context"
	"time"

	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/gogf/gf/v2/os/gcron"
	"github.com/inhies/go-bytesize"
	"golang.org/x/time/rate"
)

const (
	tunBuffer       = 64 * 1024
	burstMultiplier = 1
)

type RateService struct {
	*rate.Limiter
	counter           *Counter
	storer            storage.StateStorer
	availableEveryday uint64
	availableTotal    uint64
	linkSpeedMax      uint64
	linkSpeedMin      uint64
	currentSpeed      uint64
	todayUse          uint64
	today             int
}

func NewRate(counter *Counter, storer storage.StateStorer, min, max uint64, availableEveryday string) *RateService {
	if min > max || min < tunBuffer {
		panic("tun speed the min gt than max or min less 64KB")
	}
	av, _ := bytesize.Parse(availableEveryday)
	limit := float64(max) / time.Second.Seconds()
	l := rate.NewLimiter(rate.Limit(limit), int(max)*burstMultiplier)
	srv := &RateService{
		Limiter:           l,
		counter:           counter,
		storer:            storer,
		availableEveryday: uint64(av),
		availableTotal:    uint64(av),
		linkSpeedMax:      max,
		linkSpeedMin:      min,
		currentSpeed:      max,
		today:             time.Now().Day(),
	}
	srv.initTunStats()
	en, err := gcron.AddSingleton(context.TODO(), "1 * * * * *", func(ctx context.Context) {
		oldUse := srv.todayUse
		srv.todayUse = counter.GetTunInBytes() + counter.GetTunOutBytes()
		srv.availableTotal -= srv.todayUse - oldUse
		if time.Now().Day() != srv.today {
			srv.today = time.Now().Day()
			srv.counter.SetTun(0, 0)
			srv.todayUse = 0
			srv.availableTotal += srv.availableEveryday
			srv.cleanTunStats()
		}
		srv.saveTunStats()
		srv.updateLimit()
	})
	if err != nil {
		panic(err)
	}
	en.Start()
	return srv
}

func (s *RateService) saveTunStats() {
	s.storer.Put(keyPrefix+"tun_today_in"+time.Now().Format("20060102"), s.counter.GetTunInBytes())
	s.storer.Put(keyPrefix+"tun_today_out"+time.Now().Format("20060102"), s.counter.GetTunOutBytes())
}

func (s *RateService) initTunStats() {
	keyRead := keyPrefix + "tun_today_in" + time.Now().Format("20060102")
	keyWrite := keyPrefix + "tun_today_out" + time.Now().Format("20060102")
	var in, out uint64
	s.storer.Get(keyRead, &in)
	s.storer.Get(keyWrite, &out)
	s.counter.SetTun(in, out)
	s.todayUse = in + out
	s.availableTotal -= s.todayUse
	s.updateLimit()
}

func (s *RateService) updateLimit() {
	if s.todayUse >= s.availableEveryday/2 {
		s.currentSpeed = s.availableTotal / uint64(todayRemain())
		if s.currentSpeed < s.linkSpeedMin {
			s.currentSpeed = s.linkSpeedMin
		} else if s.currentSpeed > s.linkSpeedMax {
			s.currentSpeed = s.linkSpeedMax
		}
		s.Limiter.SetLimit(rate.Limit(float64(s.currentSpeed) / time.Second.Seconds()))
		s.Limiter.SetBurst(int(s.currentSpeed) * burstMultiplier)
	} else {
		s.Limiter.SetLimit(rate.Limit(float64(s.linkSpeedMax) / time.Second.Seconds()))
		s.Limiter.SetBurst(int(s.linkSpeedMax) * burstMultiplier)
	}
}

func (s *RateService) cleanTunStats() {
	_ = s.storer.Iterate(keyPrefix+"tun_today", func(key, value []byte) (stop bool, err error) {
		err = s.storer.Delete(string(key))
		return false, err
	})
}

func todayRemain() int64 {
	todayLast := time.Now().Format("2006-01-02") + " 23:59:59"
	todayLastTime, _ := time.Parse("2006-01-02 15:04:05", todayLast)
	return todayLastTime.Unix() - time.Now().Unix()
}
