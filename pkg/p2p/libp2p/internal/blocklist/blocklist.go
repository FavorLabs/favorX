package blocklist

import (
	"strings"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/storage"
)

var keyPrefix = "blocklist-"

// timeNow is used to deterministically mock time.Now() in tests.
var timeNow = time.Now

type Blocklist struct {
	store storage.StateStorer
}

func NewBlocklist(store storage.StateStorer) *Blocklist {
	return &Blocklist{store: store}
}

type entry struct {
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"` // Duration is string because the time.Duration does not implement MarshalJSON/UnmarshalJSON methods.
}

func (b *Blocklist) Exists(overlay boson.Address) (bool, error) {
	key := generateKey(overlay)
	timestamp, duration, err := b.get(key)
	if err != nil {
		if err == storage.ErrNotFound {
			return false, nil
		}

		return false, err
	}

	// using timeNow.Sub() so it can be mocked in unit tests
	if timeNow().Sub(timestamp) > duration && duration != 0 {
		_ = b.store.Delete(key)
		return false, nil
	}

	return true, nil
}

func (b *Blocklist) Remove(overlay boson.Address) error {
	key := generateKey(overlay)
	return b.store.Delete(key)
}

func (b *Blocklist) Add(overlay boson.Address, duration time.Duration) (err error) {
	key := generateKey(overlay)
	_, d, err := b.get(key)
	if err != nil {
		if err != storage.ErrNotFound {
			return err
		}
	}

	// if peer is already blacklisted, blacklist it for the maximum amount of time
	if duration < d && duration != 0 || d == 0 {
		duration = d
	}

	return b.store.Put(key, &entry{
		Timestamp: timeNow(),
		Duration:  duration.String(),
	})
}

// Peers returns all currently blocklisted peers.
func (b *Blocklist) Peers() ([]p2p.BlockPeers, error) {
	peers := make([]p2p.BlockPeers, 0)
	if err := b.store.Iterate(keyPrefix, func(k, v []byte) (bool, error) {
		if !strings.HasPrefix(string(k), keyPrefix) {
			return true, nil
		}
		addr, err := unmarshalKey(string(k))
		if err != nil {
			return true, err
		}

		t, d, err := b.get(string(k))
		if err != nil {
			return true, err
		}

		if timeNow().Sub(t) > d && d != 0 {
			// skip to the next item
			return false, nil
		}

		p := p2p.BlockPeers{
			Address:   addr,
			Timestamp: t.Format(time.RFC3339),
			Duration:  d.Seconds(),
		}

		peers = append(peers, p)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return peers, nil
}

func (b *Blocklist) get(key string) (timestamp time.Time, duration time.Duration, err error) {
	var e entry
	if err := b.store.Get(key, &e); err != nil {
		return time.Time{}, -1, err
	}

	duration, err = time.ParseDuration(e.Duration)
	if err != nil {
		return time.Time{}, -1, err
	}

	return e.Timestamp, duration, nil
}

func generateKey(overlay boson.Address) string {
	return keyPrefix + overlay.String()
}

func unmarshalKey(s string) (boson.Address, error) {
	addr := strings.TrimPrefix(s, keyPrefix)
	return boson.ParseHexAddress(addr)
}
