package libp2p

import (
	"bytes"
	"github.com/gauss-project/aurorafs/pkg/p2p"
	"io"
)

var _ p2p.Stream = (*virtualStream)(nil)

type virtualStream struct {
	p2p.Stream
	read  *io.PipeReader
	write *io.PipeWriter
}

func newVirtualStream(s p2p.Stream) *virtualStream {
	srv := &virtualStream{
		Stream: s,
	}
	srv.read, srv.write = io.Pipe()
	return srv
}

func (s *virtualStream) Read(p []byte) (int, error) {
	return s.read.Read(p)
}

func (s *virtualStream) Reset() error {
	return nil
}

type BuffMessage struct {
	bytes.Buffer
}

func (b *BuffMessage) ProtoMessage() {}

func (b *BuffMessage) Unmarshal(p []byte) error {
	_, err := b.Write(p)
	return err
}

func (b *BuffMessage) Marshal() ([]byte, error) {
	return b.Bytes(), nil
}
