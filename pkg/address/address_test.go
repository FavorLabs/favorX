package address_test

import (
	"testing"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/crypto"

	ma "github.com/multiformats/go-multiaddr"
)

func TestAddress(t *testing.T) {
	node1ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	signer1 := crypto.NewDefaultSigner()

	overlay, err := crypto.NewOverlayAddress(signer1.Public().Encode(), 3)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := address.NewAddress(signer1, node1ma, overlay, 3)
	if err != nil {
		t.Fatal(err)
	}

	addr2, err := address.ParseAddress(node1ma.Bytes(), signer1.Public().Encode(), overlay.Bytes(), addr.Signature, 3)
	if err != nil {
		t.Fatal(err)
	}

	if !addr.Equal(addr2) {
		t.Fatalf("got %s expected %s", addr2, addr)
	}

	bytes, err := addr.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var nowAddr address.Address
	if err = nowAddr.UnmarshalJSON(bytes); err != nil {
		t.Fatal(err)
	}

	if !nowAddr.Equal(addr) {
		t.Fatalf("got %s expected %s", nowAddr, addr)
	}
}
