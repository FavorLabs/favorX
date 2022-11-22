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
	topModel "github.com/FavorLabs/favorX/pkg/topology/model"
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

	var dir topModel.PeerConnectionDirection
	if stream.IsVirtual() {
		v, shaking := s.virtualConn.Add(addr, topModel.PeerConnectionDirectionOutbound)
		if shaking {
			err = errors.New("multicast Handshake in progress")
			s.logger.Errorf("multicast Handshake in progress")
			return err
		}
		dir = v.Direction
		defer func() {
			if err != nil {
				s.DisconnectVirtual(addr)
			} else {
				s.virtualConn.HandshakeDone(addr, 0, time.Since(start))
			}
		}()
	}
	w, r := protobuf.NewWriterAndReader(stream)

	s.logger.Tracef("group: handshake send syn to %s", addr)

	err = w.WriteMsgWithContext(ctx, &pb.Syn{Direction: string(dir)})
	if err != nil {
		s.logger.Errorf("multicast Handshake write %s", err)
		return err
	}

	s.logger.Tracef("group: handshake receive ack from %s", addr)

	resp := &pb.SynAck{}
	err = r.ReadMsgWithContext(ctx, resp)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = errors.New("handshake failed")
		}
		return err
	}

	if stream.IsVirtual() && resp.Syn.Direction == string(dir) {
		err = errors.New("handshake mismatch direction")
		s.logger.Error(err)
		return err
	}

	err = w.WriteMsgWithContext(ctx, &pb.Ack{
		Gid:     s.getGIDsByte(),
		Timeout: keepPingInterval.Milliseconds() / 1e3,
	})
	if err != nil {
		return err
	}

	s.kad.RecordPeerLatency(addr, time.Since(start))

	s.logger.Tracef("group: Handshake receive gid list from %s", addr)

	s.refreshGroup(addr, resp.Ack.Gid)
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
	start := time.Now()

	var (
		timeout int64
		dir     topModel.PeerConnectionDirection
	)
	if stream.IsVirtual() {
		v, shaking := s.virtualConn.Add(peer.Address, topModel.PeerConnectionDirectionInbound)
		if shaking {
			err = errors.New("multicast Handshake in progress")
			s.logger.Errorf("multicast HandshakeIncoming Handshake in progress")
			return err
		}
		dir = v.Direction
		defer func() {
			if err != nil {
				s.DisconnectVirtual(peer.Address)
			} else {
				s.virtualConn.HandshakeDone(peer.Address, timeout, time.Since(start))
			}
		}()
	}

	w, r := protobuf.NewWriterAndReader(stream)
	resp := &pb.Syn{}

	s.logger.Tracef("group: receive handshake Syn from %s", peer.Address)

	err = r.ReadMsgWithContext(ctx, resp)
	if err != nil {
		s.logger.Errorf("multicast HandshakeIncoming read %s", err)
		return err
	}

	if stream.IsVirtual() && resp.Direction == string(dir) {
		err = errors.New("HandshakeIncoming mismatch direction")
		s.logger.Error(err)
		return err
	}

	s.logger.Tracef("group: send back handshake SynAck to %s", peer.Address)

	err = w.WriteMsgWithContext(ctx, &pb.SynAck{
		Syn: &pb.Syn{Direction: string(dir)},
		Ack: &pb.Ack{
			Gid:     s.getGIDsByte(),
			Timeout: keepPingInterval.Milliseconds() / 1e3,
		},
	})
	if err != nil {
		s.logger.Errorf("multicast HandshakeIncoming SynAck write %s", err)
		return err
	}

	var ack pb.Ack
	err = r.ReadMsgWithContext(ctx, &ack)
	if err != nil {
		s.logger.Errorf("multicast HandshakeIncoming Ack read %s", err)
		return err
	}

	timeout = ack.Timeout

	s.refreshGroup(peer.Address, ack.Gid)
	s.logger.Tracef("group: HandshakeIncoming ok")

	return nil
}

func (s *Service) DisconnectVirtual(addr boson.Address) {
	s.groups.Range(func(key, value any) bool {
		g := value.(*Group)
		g.chanEvent <- eventPeerAction{
			addr:  addr,
			event: EventDisconnectVirtual,
		}
		return true
	})
}
