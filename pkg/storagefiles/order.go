package storagefiles

import (
	"context"
	"fmt"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/FavorLabs/favorX/pkg/routetab"
	"github.com/FavorLabs/favorX/pkg/storagefiles/pb"
	"github.com/FavorLabs/favorX/pkg/subscribe"
)

const (
	protocolName    = "storagefiles"
	protocolVersion = "1.0.0"
	streamNotify    = "notify"
)

func (s *StreamService) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamNotify,
				Handler: s.onNotify,
			},
		},
	}
}

type StreamServiceInterface interface {
	Notify(ctx context.Context, target boson.Address, cid boson.Address) error
}

type StreamService struct {
	streamer p2p.Streamer
	logger   logging.Logger
	sub      subscribe.SubPub
	route    routetab.RouteTab
	signer   crypto.Signer
}

func NewNotifyService(streamer p2p.Streamer, signer crypto.Signer, logging logging.Logger, sub subscribe.SubPub, route routetab.RouteTab) *StreamService {
	return &StreamService{
		streamer: streamer,
		logger:   logging,
		sub:      sub,
		route:    route,
		signer:   signer,
	}
}

func (s *StreamService) Notify(ctx context.Context, target boson.Address, cid boson.Address) error {
	stream, err := s.streamer.NewStream(ctx, target, nil, protocolName, protocolVersion, streamNotify)
	if err != nil {
		err = s.route.Connect(ctx, target)
		if err == nil {
			stream, err = s.streamer.NewStream(ctx, target, nil, protocolName, protocolVersion, streamNotify)
		}
		if err != nil {
			s.logger.Debugf("placeorder: Notify NewStream %s, err=%s", target.String(), err)
			return err
		}
	}

	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	w := protobuf.NewWriter(stream)
	err = w.WriteMsgWithContext(ctx, &pb.Request{
		Cid:   cid.Bytes(),
		Buyer: s.signer.Public().Encode(),
	})
	return err
}

func (s *StreamService) onNotify(ctx context.Context, peer p2p.Peer, stream p2p.Stream) (err error) {
	r := protobuf.NewReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()
	req := pb.Request{}
	if err = r.ReadMsgWithContext(ctx, &req); err != nil {
		content := fmt.Sprintf("placeorder: onNotify read msg: %s", err.Error())
		s.logger.Errorf(content)
		return fmt.Errorf(content)
	}
	cid := boson.NewAddress(req.Cid)
	data := UploadRequest{
		Source: peer.Address,
		Hash:   cid,
		Buyer:  boson.NewAddress(req.Buyer),
	}
	return s.sub.Publish("storagefiles", "order", "notify", data)
}
