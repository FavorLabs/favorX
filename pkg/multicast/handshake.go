package multicast

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/multicast/pb"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
)

func (s *Service) Handshake(ctx context.Context, addr boson.Address) (err error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	start := time.Now()
	var stream p2p.Stream
	stream, err = s.getStream(ctx, addr, streamHandshake)
	if err != nil {
		s.logger.Tracef("group: handshake new stream %s %s", addr, err)
		return
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	w, r := protobuf.NewWriterAndReader(stream)

	GIDs := s.getGIDsByte()

	s.logger.Tracef("group: handshake send syn to %s", addr)

	err = w.WriteMsgWithContext(ctx, &pb.GIDs{Gid: GIDs})
	if err != nil {
		s.logger.Errorf("multicast Handshake write %s", err)
		return err
	}

	s.logger.Tracef("group: handshake receive ack from %s", addr)

	resp := &pb.GIDs{}
	err = r.ReadMsgWithContext(ctx, resp)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = errors.New("handshake failed")
		}
		return err
	}

	s.kad.RecordPeerLatency(addr, time.Since(start))

	s.logger.Tracef("group: Handshake receive gid list from %s", addr)
	s.refreshGroup(addr, resp.Gid)
	s.logger.Tracef("group: Handshake ok")
	return nil
}

func (s *Service) HandshakeIncoming(ctx context.Context, peer p2p.Peer, stream p2p.Stream) (err error) {
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()
	w, r := protobuf.NewWriterAndReader(stream)
	resp := &pb.GIDs{}

	s.logger.Tracef("group: receive handshake syn from %s", peer.Address)

	err = r.ReadMsgWithContext(ctx, resp)
	if err != nil {
		s.logger.Errorf("multicast HandshakeIncoming read %s", err)
		return err
	}

	s.logger.Tracef("group: receive handshake gid list from %s", peer.Address)
	go s.refreshGroup(peer.Address, resp.Gid)

	GIDs := s.getGIDsByte()

	s.logger.Tracef("group: send back handshake ack to %s", peer.Address)

	err = w.WriteMsgWithContext(ctx, &pb.GIDs{Gid: GIDs})
	if err != nil {
		s.logger.Errorf("multicast HandshakeIncoming write %s", err)
		return err
	}

	s.logger.Tracef("group: HandshakeIncoming ok")

	return nil
}
