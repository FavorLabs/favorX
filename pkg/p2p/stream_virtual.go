package p2p

import (
	"bytes"
	"github.com/gauss-project/aurorafs/pkg/p2p"
	"io"
)

var _ p2p.Stream = (*virtualStream)(nil)

type virtualStream struct {
	p2p.Stream
	ReadPipe  *io.PipeReader
	WritePipe *io.PipeWriter
}

func NewVirtualStream(s p2p.Stream) *virtualStream {
	srv := &virtualStream{
		Stream: s,
	}
	srv.ReadPipe, srv.WritePipe = io.Pipe()
	return srv
}

func (s *virtualStream) Read(p []byte) (int, error) {
	return s.ReadPipe.Read(p)
}

func (s *virtualStream) Reset() error {
	return nil
}

func (s *virtualStream) FullClose() error {
	return nil
}

func (s *virtualStream) Close() error {
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
