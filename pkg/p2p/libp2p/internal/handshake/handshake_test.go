package handshake_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/bitvector"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/libp2p/internal/handshake"
	"github.com/FavorLabs/favorX/pkg/p2p/libp2p/internal/handshake/mock"
	"github.com/FavorLabs/favorX/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/FavorLabs/favorX/pkg/topology/lightnode"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func TestHandshake(t *testing.T) {
	const (
		testWelcomeMessage = "HelloWorld"
	)

	logger := logging.New(io.Discard, 0)
	networkID := uint64(3)

	node1ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}
	node2ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}
	node1maBinary, err := node1ma.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	node2maBinary, err := node2ma.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	node1AddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(node1ma)
	if err != nil {
		t.Fatal(err)
	}
	node2AddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(node2ma)
	if err != nil {
		t.Fatal(err)
	}

	signer1 := crypto.NewDefaultSigner()
	signer2 := crypto.NewDefaultSigner()
	addr1, err := crypto.NewOverlayAddress(signer1.Public().Encode(), networkID)
	if err != nil {
		t.Fatal(err)
	}
	node1Address, err := address.NewAddress(signer1, node1ma, addr1, networkID)
	if err != nil {
		t.Fatal(err)
	}
	addr2, err := crypto.NewOverlayAddress(signer2.Public().Encode(), networkID)
	if err != nil {
		t.Fatal(err)
	}
	node2Address, err := address.NewAddress(signer2, node2ma, addr2, networkID)
	if err != nil {
		t.Fatal(err)
	}

	node1Info := address.AddressInfo{
		Address:  node1Address,
		NodeMode: address.NewModel().SetMode(address.FullNode),
	}
	node2Info := address.AddressInfo{
		Address:  node2Address,
		NodeMode: address.NewModel().SetMode(address.FullNode),
	}

	aaddresser := &AdvertisableAddresserMock{}

	light := lightnode.NewContainer(node1Info.Address.Overlay)
	handshakeService, err := handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), testWelcomeMessage, node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Handshake - OK", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, r := protobuf.NewWriterAndReader(stream2)
		if err = w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.Address{
					Underlay:  node2maBinary,
					PublicKey: node2Address.PublicKey,
					Overlay:   node2Address.Overlay.Bytes(),
					Signature: node2Address.Signature,
				},
				NetworkID:      networkID,
				NodeMode:       node2Info.NodeMode.Bv.Bytes(),
				WelcomeMessage: testWelcomeMessage,
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, *res, node2Info)

		var syn pb.Syn
		if err := r.ReadMsg(&syn); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(syn.ObservedUnderlay, node2maBinary) {
			t.Fatal("bad syn")
		}

		var ack pb.Ack
		if err = r.ReadMsg(&ack); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(ack.Address.Overlay, node1Address.Overlay.Bytes()) {
			t.Fatal("bad ack - overlay")
		}
		if !bytes.Equal(ack.Address.Underlay, node1maBinary) {
			t.Fatal("bad ack - underlay")
		}

		if bytes.Equal(ack.Address.Signature, node1Address.Signature) {
			t.Fatal("bad ack - signature")
		}
		if ack.NetworkID != networkID {
			t.Fatal("bad ack - networkID")
		}
		bv1, _ := bitvector.NewFromBytes(ack.NodeMode, 1)
		nb := address.Model{Bv: bv1}
		if nb.IsFull() != true {
			t.Fatal("bad ack - full node")
		}

		if ack.WelcomeMessage != testWelcomeMessage {
			t.Fatalf("Bad ack welcome message: want %s, got %s", testWelcomeMessage, ack.WelcomeMessage)
		}
	})

	t.Run("Handshake - picker error", func(t *testing.T) {

		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}

		handshakeService.SetPicker(mockPicker(func(p p2p.Peer) bool { return false }))

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err = w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err = w.WriteMsg(&pb.Ack{
			Address: &pb.Address{
				Underlay:  node2maBinary,
				PublicKey: node2Address.PublicKey,
				Overlay:   node2Address.Overlay.Bytes(),
				Signature: node2Address.Signature,
			},
			NetworkID: networkID,
			NodeMode:  address.NewModel().SetMode(address.FullNode).Bv.Bytes(),
		}); err != nil {
			t.Fatal(err)
		}

		_, err = handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		expectedErr := handshake.ErrPicker
		if !errors.Is(err, expectedErr) {
			t.Fatal("expected:", expectedErr, "got:", err)
		}
	})

	t.Run("Handshake - picker light error", func(t *testing.T) {

		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, 0)
		if err != nil {
			t.Fatal(err)
		}

		handshakeService.SetPicker(mockPicker(func(p p2p.Peer) bool { return true }))

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err = w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err = w.WriteMsg(&pb.Ack{
			Address: &pb.Address{
				Underlay:  node2maBinary,
				PublicKey: node2Address.PublicKey,
				Overlay:   node2Address.Overlay.Bytes(),
				Signature: node2Address.Signature,
			},
			NetworkID: networkID,
			NodeMode:  address.NewModel().Bv.Bytes(),
		}); err != nil {
			t.Fatal(err)
		}

		_, err = handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		expectedErr := handshake.ErrPickerLight
		if !errors.Is(err, expectedErr) {
			t.Fatal("expected:", expectedErr, "got:", err)
		}
	})

	t.Run("Handshake - welcome message too long", func(t *testing.T) {
		const LongMessage = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Morbi consectetur urna ut lorem sollicitudin posuere. Donec sagittis laoreet sapien."

		expectedErr := handshake.ErrWelcomeMessageLength
		_, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), LongMessage, node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}
	})

	t.Run("Handshake - dynamic welcome message too long", func(t *testing.T) {
		const LongMessage = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Morbi consectetur urna ut lorem sollicitudin posuere. Donec sagittis laoreet sapien."

		expectedErr := handshake.ErrWelcomeMessageLength
		err := handshakeService.SetWelcomeMessage(LongMessage)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}
	})

	t.Run("Handshake - set welcome message", func(t *testing.T) {
		const TestMessage = "Hi im the new test message"

		err := handshakeService.SetWelcomeMessage(TestMessage)
		if err != nil {
			t.Fatal("Got error:", err)
		}
		got := handshakeService.GetWelcomeMessage()
		if got != TestMessage {
			t.Fatal("expected:", TestMessage, ", got:", got)
		}
	})

	t.Run("Handshake - Syn write error", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetWriteErr(testErr, 0)
		res, err := handshakeService.Handshake(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handshake - Syn read error", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read synack message: %w", testErr)
		stream := mock.NewStream(nil, &bytes.Buffer{})
		stream.SetReadErr(testErr, 0)
		res, err := handshakeService.Handshake(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handshake - ack write error", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write ack message: %w", testErr)
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream1.SetWriteErr(testErr, 1)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.Address{
					Underlay:  node2maBinary,
					PublicKey: node2Address.PublicKey,
					Overlay:   node2Address.Overlay.Bytes(),
					Signature: node2Address.Signature,
				},
				NetworkID: networkID,
				NodeMode:  node2Info.NodeMode.Bv.Bytes(),
			},
		},
		); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handshake - networkID mismatch", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.Address{
					Underlay:  node2maBinary,
					PublicKey: node2Address.PublicKey,
					Overlay:   node2Address.Overlay.Bytes(),
					Signature: node2Address.Signature,
				},
				NetworkID: 5,
				NodeMode:  node2Info.NodeMode.Bv.Bytes(),
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != handshake.ErrNetworkIDIncompatible {
			t.Fatalf("expected %s, got %s", handshake.ErrNetworkIDIncompatible, err)
		}
	})

	t.Run("Handshake - invalid ack", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.Address{
					Underlay:  node2maBinary,
					PublicKey: node2Address.PublicKey,
					Overlay:   node2Address.Overlay.Bytes(),
					Signature: node1Address.Signature,
				},
				NetworkID: networkID,
				NodeMode:  node2Info.NodeMode.Bv.Bytes(),
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != handshake.ErrInvalidAck {
			t.Fatalf("expected %s, got %s", handshake.ErrInvalidAck, err)
		}
	})

	t.Run("Handshake - error advertisable address", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		testError := errors.New("test error")
		aaddresser.err = testError
		defer func() {
			aaddresser.err = nil
		}()

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.Address{
					Underlay:  node2maBinary,
					PublicKey: node2Address.PublicKey,
					Overlay:   node2Address.Overlay.Bytes(),
					Signature: node2Address.Signature,
				},
				NetworkID: networkID,
				NodeMode:  node2Info.NodeMode.Bv.Bytes(),
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != testError {
			t.Fatalf("expected error %v got %v", testError, err)

		}

		if res != nil {
			t.Fatal("expected nil res")
		}

	})

	t.Run("Handle - OK", func(t *testing.T) {
		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err = w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err = w.WriteMsg(&pb.Ack{
			Address: &pb.Address{
				Underlay:  node2maBinary,
				PublicKey: node2Address.PublicKey,
				Overlay:   node2Address.Overlay.Bytes(),
				Signature: node2Address.Signature,
			},
			NetworkID: networkID,
			NodeMode:  node2Info.NodeMode.Bv.Bytes(),
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, *res, node2Info)

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.SynAck
		if err = r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Syn.ObservedUnderlay, node2maBinary) {
			t.Fatalf("got bad syn")
		}

		ackAddr, err := address.ParseAddress(got.Ack.Address.Underlay, got.Ack.Address.PublicKey, got.Ack.Address.Overlay, got.Ack.Address.Signature, got.Ack.NetworkID)
		if err != nil {
			t.Fatal(err)
		}

		nb2, err := address.NewModelFromBytes(got.Ack.NodeMode)
		if err != nil {
			t.Fatal(err)
		}
		testInfo(t, node1Info, address.AddressInfo{
			Address:  ackAddr,
			NodeMode: nb2,
		})
	})

	t.Run("Handle - read error ", func(t *testing.T) {
		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetReadErr(testErr, 0)
		res, err := handshakeService.Handle(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handle returned non-nil res")
		}
	})

	t.Run("Handle - write error ", func(t *testing.T) {
		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write synack message: %w", testErr)
		var buffer bytes.Buffer
		stream := mock.NewStream(&buffer, &buffer)
		stream.SetWriteErr(testErr, 1)
		w := protobuf.NewWriter(stream)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handle - ack read error ", func(t *testing.T) {
		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read ack message: %w", testErr)
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)
		stream1.SetReadErr(testErr, 1)
		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handle - networkID mismatch ", func(t *testing.T) {
		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err = w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err = w.WriteMsg(&pb.Ack{
			Address: &pb.Address{
				Underlay:  node2maBinary,
				Overlay:   node2Address.Overlay.Bytes(),
				Signature: node2Address.Signature,
			},
			NetworkID: 5,
			NodeMode:  node2Info.NodeMode.Bv.Bytes(),
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != handshake.ErrNetworkIDIncompatible {
			t.Fatalf("expected %s, got %s", handshake.ErrNetworkIDIncompatible, err)
		}
	})

	t.Run("Handle - invalid ack", func(t *testing.T) {
		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{
			Address: &pb.Address{
				Underlay:  node2maBinary,
				PublicKey: node2Address.PublicKey,
				Overlay:   node2Address.Overlay.Bytes(),
				Signature: node1Address.Signature,
			},
			NetworkID: networkID,
			NodeMode:  node2Info.NodeMode.Bv.Bytes(),
		}); err != nil {
			t.Fatal(err)
		}

		_, err = handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != handshake.ErrInvalidAck {
			t.Fatalf("expected %s, got %v", handshake.ErrInvalidAck, err)
		}
	})

	t.Run("Handle - advertisable error", func(t *testing.T) {
		handshakeService, err = handshake.New(signer1, aaddresser, node1Info.Address.Overlay, networkID, address.NewModel().SetMode(address.FullNode), "", node1AddrInfo.ID, logger, light, lightnode.DefaultLightNodeLimit)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		testError := errors.New("test error")
		aaddresser.err = testError
		defer func() {
			aaddresser.err = nil
		}()

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != testError {
			t.Fatal("expected error")
		}

		if res != nil {
			t.Fatal("expected nil res")
		}
	})
}

func mockPicker(f func(p2p.Peer) bool) p2p.Picker {
	return &picker{pickerFunc: f}
}

type picker struct {
	pickerFunc func(p2p.Peer) bool
}

func (p *picker) Pick(peer p2p.Peer) bool {
	return p.pickerFunc(peer)
}

// testInfo validates if two Info instances are equal.
func testInfo(t *testing.T, got, want address.AddressInfo) {
	t.Helper()
	if !got.Address.Equal(want.Address) || got.NodeMode.Bv.String() != want.NodeMode.Bv.String() {
		t.Fatalf("got info %+v, want %+v", got.NodeMode.Bv.String(), want.NodeMode.Bv.String())
	}
}

type AdvertisableAddresserMock struct {
	advertisableAddress ma.Multiaddr
	err                 error
}

func (a *AdvertisableAddresserMock) Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error) {
	if a.err != nil {
		return nil, a.err
	}

	if a.advertisableAddress != nil {
		return a.advertisableAddress, nil
	}

	return observedAddress, nil
}
