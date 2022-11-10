package debugapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	favor "github.com/FavorLabs/favorX"
	accountingmock "github.com/FavorLabs/favorX/pkg/accounting/mock"
	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/debugapi"
	"github.com/FavorLabs/favorX/pkg/jsonhttp/jsonhttptest"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p/mock"
	p2pmock "github.com/FavorLabs/favorX/pkg/p2p/mock"
	"github.com/FavorLabs/favorX/pkg/pingpong"
	"github.com/FavorLabs/favorX/pkg/resolver"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/FavorLabs/favorX/pkg/topology/bootnode"
	"github.com/FavorLabs/favorX/pkg/topology/lightnode"
	topologymock "github.com/FavorLabs/favorX/pkg/topology/mock"
	"github.com/multiformats/go-multiaddr"
	"resenje.org/web"
)

type testServerOptions struct {
	Overlay            boson.Address
	PublicKey          crypto.PublicKey
	CORSAllowedOrigins []string
	P2P                *p2pmock.Service
	Pingpong           pingpong.Interface
	Storer             storage.Storer
	Resolver           resolver.Interface
	TopologyOpts       []topologymock.Option
	AccountingOpts     []accountingmock.Option
}

type testServer struct {
	Client  *http.Client
	P2PMock *p2pmock.Service
}

var logger = logging.New(io.Discard, 0)

func newTestServer(t *testing.T, o testServerOptions) *testServer {
	topologyDriver := topologymock.NewTopologyDriver(o.TopologyOpts...)
	// acc := accountingmock.NewAccounting(o.AccountingOpts...)
	// settlement := swapmock.New(o.SettlementOpts...)
	// chequebook := chequebookmock.NewChequebook(o.ChequebookOpts...)
	// swapserv := swapmock.NewApiInterface(o.SwapOpts...)
	ln := lightnode.NewContainer(o.Overlay)
	bn := bootnode.NewContainer(o.Overlay)
	s := debugapi.New(o.Overlay, o.PublicKey, logging.New(io.Discard, 0), nil, o.CORSAllowedOrigins, false, nil, debugapi.Options{NodeMode: address.NewModel()})
	s.Configure(o.P2P, o.Pingpong, nil, topologyDriver, ln, bn, o.Storer, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	client := &http.Client{
		Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			u, err := url.Parse(ts.URL + r.URL.String())
			if err != nil {
				return nil, err
			}
			r.URL = u
			return ts.Client().Transport.RoundTrip(r)
		}),
	}
	return &testServer{
		Client:  client,
		P2PMock: o.P2P,
	}
}

func mustMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	t.Helper()

	a, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestServer_Configure validates that http routes are correct when server is
// constructed with only basic routes, and after it is configured with
// dependencies.
func TestServer_Configure(t *testing.T) {
	privateKey := crypto.NewDefaultSigner()

	overlay, err := crypto.NewOverlayAddress(privateKey.Public().Encode(), 1)
	if err != nil {
		t.Error(err)
	}
	addresses := []multiaddr.Multiaddr{
		mustMultiaddr(t, "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/192.168.0.101/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/127.0.0.1/udp/7071/quic/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
	}

	o := testServerOptions{
		PublicKey: privateKey.Public(),
		Overlay:   overlay,
		P2P: mock.New(mock.WithAddressesFunc(func() ([]multiaddr.Multiaddr, error) {
			return addresses, nil
		})),
	}
	topologyDriver := topologymock.NewTopologyDriver(o.TopologyOpts...)
	// acc := accountingmock.NewAccounting(o.AccountingOpts...)
	// settlement := swapmock.New(o.SettlementOpts...)
	// chequebook := chequebookmock.NewChequebook(o.ChequebookOpts...)
	// swapserv := swapmock.NewApiInterface(o.SwapOpts...)
	ln := lightnode.NewContainer(o.Overlay)
	bn := bootnode.NewContainer(o.Overlay)
	s := debugapi.New(o.Overlay, o.PublicKey, logger, nil, nil, false, nil, debugapi.Options{
		NodeMode: address.NewModel(),
	})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	client := &http.Client{
		Transport: web.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			u, err := url.Parse(ts.URL + r.URL.String())
			if err != nil {
				return nil, err
			}
			r.URL = u
			return ts.Client().Transport.RoundTrip(r)
		}),
	}

	testBasicRouter(t, client)

	jsonhttptest.Request(t, client, http.MethodGet, "/addresses", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.AddressesResponse{
			Overlay:   o.Overlay,
			Underlay:  make([]multiaddr.Multiaddr, 0),
			NATRoute:  []string{},
			PublicIP:  *debugapi.GetPublicIp(logger),
			NetworkID: 0,
			PublicKey: o.PublicKey.Hex(),
		}),
	)

	s.Configure(o.P2P, o.Pingpong, nil, topologyDriver, ln, bn, o.Storer, nil, nil, nil, nil, nil)

	testBasicRouter(t, client)
	jsonhttptest.Request(t, client, http.MethodGet, "/readiness", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.StatusResponse{
			Status:       "ok",
			Version:      favor.Version,
			FullNode:     false,
			BootNodeMode: false,
		}),
	)
	jsonhttptest.Request(t, client, http.MethodGet, "/addresses", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.AddressesResponse{
			Overlay:   o.Overlay,
			Underlay:  addresses,
			NATRoute:  []string{"1.1.1.1"},
			PublicIP:  *debugapi.GetPublicIp(logger),
			NetworkID: 0,
			PublicKey: o.PublicKey.Hex(),
		}),
	)
}

func testBasicRouter(t *testing.T, client *http.Client) {
	t.Helper()

	jsonhttptest.Request(t, client, http.MethodGet, "/health", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.StatusResponse{
			Status:       "ok",
			Version:      favor.Version,
			FullNode:     false,
			BootNodeMode: false,
		}),
	)

	for _, path := range []string{
		"/metrics",
		"/debug/pprof",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile?seconds=1", // profile for only 1 second to check only the status code
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/vars",
	} {
		jsonhttptest.Request(t, client, http.MethodGet, path, http.StatusOK)
	}
}
