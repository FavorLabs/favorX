package debugapi_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/debugapi"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/FavorLabs/favorX/pkg/jsonhttp/jsonhttptest"
	"github.com/FavorLabs/favorX/pkg/p2p/mock"
	"github.com/multiformats/go-multiaddr"
)

func TestAddresses(t *testing.T) {
	privateKey := crypto.NewDefaultSigner()

	overlay := boson.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	addresses := []multiaddr.Multiaddr{
		mustMultiaddr(t, "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/192.168.0.101/tcp/7071/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
		mustMultiaddr(t, "/ip4/127.0.0.1/udp/7071/quic/p2p/16Uiu2HAmTBuJT9LvNmBiQiNoTsxE5mtNy6YG3paw79m94CRa9sRb"),
	}

	testServer := newTestServer(t, testServerOptions{
		PublicKey: privateKey.Public(),
		Overlay:   overlay,
		P2P: mock.New(mock.WithAddressesFunc(func() ([]multiaddr.Multiaddr, error) {
			return addresses, nil
		})),
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/addresses", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.AddressesResponse{
				Overlay:   overlay,
				Underlay:  addresses,
				NATRoute:  []string{"1.1.1.1"},
				PublicIP:  *debugapi.GetPublicIp(logger),
				NetworkID: 0,
				PublicKey: privateKey.Public().Hex(),
			}),
		)
	})

	t.Run("post method not allowed", func(t *testing.T) {
		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/addresses", http.StatusMethodNotAllowed,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusMethodNotAllowed,
				Message: http.StatusText(http.StatusMethodNotAllowed),
			}),
		)
	})
}

func TestAddresses_error(t *testing.T) {
	testErr := errors.New("test error")

	testServer := newTestServer(t, testServerOptions{
		P2P: mock.New(mock.WithAddressesFunc(func() ([]multiaddr.Multiaddr, error) {
			return nil, testErr
		})),
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/addresses", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Code:    http.StatusInternalServerError,
			Message: testErr.Error(),
		}),
	)
}
