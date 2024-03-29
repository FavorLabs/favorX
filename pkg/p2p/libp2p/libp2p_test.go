package libp2p_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"github.com/FavorLabs/favorX/pkg/address"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/FavorLabs/favorX/pkg/addressbook"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/libp2p"
	"github.com/FavorLabs/favorX/pkg/statestore/mock"
	"github.com/FavorLabs/favorX/pkg/topology/bootnode"
	"github.com/FavorLabs/favorX/pkg/topology/lightnode"
	"github.com/multiformats/go-multiaddr"
)

type libp2pServiceOpts struct {
	Logger      logging.Logger
	Addressbook addressbook.Interface
	PrivateKey  *ecdsa.PrivateKey
	MockPeerKey *ecdsa.PrivateKey
	libp2pOpts  libp2p.Options
	lightNodes  *lightnode.Container
	bootNodes   *bootnode.Container
	notifier    p2p.PickyNotifier
}

// newService constructs a new libp2p service.
func newService(t *testing.T, networkID uint64, o libp2pServiceOpts) (s *libp2p.Service, overlay boson.Address) {
	t.Helper()

	k, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err = crypto.NewOverlayAddress(k.PublicKey, networkID)
	if err != nil {
		t.Fatal(err)
	}

	addr := ":0"

	if o.Logger == nil {
		o.Logger = logging.New(io.Discard, 0)
	}

	statestore := mock.NewStateStore()
	if o.Addressbook == nil {
		o.Addressbook = addressbook.New(statestore)
	}

	if o.PrivateKey == nil {
		libp2pKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}

		o.PrivateKey = libp2pKey
	}

	ctx, cancel := context.WithCancel(context.Background())

	if o.lightNodes == nil {
		o.lightNodes = lightnode.NewContainer(overlay)
	}
	if o.bootNodes == nil {
		o.bootNodes = bootnode.NewContainer(overlay)
	}
	if o.libp2pOpts.NodeMode.Bv == nil {
		o.libp2pOpts.NodeMode = address.NewModel()
	}
	opts := o.libp2pOpts

	s, err = libp2p.New(ctx, crypto.NewDefaultSigner(k), networkID, overlay, addr, o.Addressbook, statestore, o.lightNodes, o.bootNodes, o.Logger, nil, opts)
	if err != nil {
		t.Fatal(err)
	}

	if o.notifier != nil {
		s.SetPickyNotifier(o.notifier)
	}

	s.Ready()

	t.Cleanup(func() {
		cancel()
		s.Close()
	})
	return s, overlay
}

// expectPeers validates that peers with addresses are connected.
func expectPeers(t *testing.T, s *libp2p.Service, addrs ...boson.Address) {
	t.Helper()

	peers := s.Peers()

	if len(peers) != len(addrs) {
		t.Fatalf("got peers %v, want %v", len(peers), len(addrs))
	}

	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) == -1
	})
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})

	for i, got := range peers {
		want := addrs[i]
		if !got.Address.Equal(want) {
			t.Errorf("got %v peer %s, want %s", i, got.Address, want)
		}
	}
}

// expectPeersEventually validates that peers with addresses are connected with
// retries. It is supposed to be used to validate asynchronous connecting on the
// peer that is connected to.
func expectPeersEventually(t *testing.T, s *libp2p.Service, addrs ...boson.Address) {
	t.Helper()

	var peers []p2p.Peer
	for i := 0; i < 100; i++ {
		peers = s.Peers()
		if len(peers) == len(addrs) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(peers) != len(addrs) {
		t.Fatalf("got peers %v, want %v", len(peers), len(addrs))
	}

	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) == -1
	})
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})

	for i, got := range peers {
		want := addrs[i]
		if !got.Address.Equal(want) {
			t.Errorf("got %v peer %s, want %s", i, got.Address, want)
		}
	}
}

func serviceUnderlayAddress(t *testing.T, s *libp2p.Service) multiaddr.Multiaddr {
	t.Helper()

	addrs, err := s.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	return addrs[0]
}
