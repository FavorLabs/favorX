package libp2p

import (
	"context"
	"fmt"
	"time"

	hpb "github.com/FavorLabs/favorX/pkg/p2p/libp2p/internal/headers/pb"
	"github.com/gauss-project/aurorafs/pkg/boson"
	"github.com/gauss-project/aurorafs/pkg/p2p"
	"github.com/gauss-project/aurorafs/pkg/p2p/protobuf"
)

var sendHeadersTimeout = 2 * time.Second

func sendHeaders(ctx context.Context, headers p2p.Headers, stream *stream) error {
	w, r := protobuf.NewWriterAndReader(stream)

	ctx, cancel := context.WithTimeout(ctx, sendHeadersTimeout)
	defer cancel()

	if err := w.WriteMsgWithContext(ctx, headersP2PToPB(headers)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	h := new(hpb.Headers)
	if err := r.ReadMsgWithContext(ctx, h); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	stream.headers = headersPBToP2P(h)

	return nil
}

func handleHeaders(headler p2p.HeadlerFunc, stream *stream, peerAddress boson.Address) error {
	w, r := protobuf.NewWriterAndReader(stream)

	ctx, cancel := context.WithTimeout(context.Background(), sendHeadersTimeout)
	defer cancel()

	headers := new(hpb.Headers)
	if err := r.ReadMsgWithContext(ctx, headers); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	stream.headers = headersPBToP2P(headers)

	var h p2p.Headers
	if headler != nil {
		h = headler(stream.headers, peerAddress)
	}

	stream.responseHeaders = h

	if err := w.WriteMsgWithContext(ctx, headersP2PToPB(h)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

func headersPBToP2P(h *hpb.Headers) p2p.Headers {
	p2ph := make(p2p.Headers)
	for _, rh := range h.Headers {
		p2ph[rh.Key] = rh.Value
	}
	return p2ph
}

func headersP2PToPB(h p2p.Headers) *hpb.Headers {
	pbh := new(hpb.Headers)
	pbh.Headers = make([]*hpb.Header, 0)
	for key, value := range h {
		pbh.Headers = append(pbh.Headers, &hpb.Header{
			Key:   key,
			Value: value,
		})
	}
	return pbh
}
