package p2p

import (
	"bytes"
	"io"
)

var _ Stream = (*StreamVirtual)(nil)

type StreamVirtual struct {
	Stream
	ReadPipe  *io.PipeReader
	WritePipe *io.PipeWriter
}

func NewVirtualStream(s Stream) *StreamVirtual {
	srv := &StreamVirtual{
		Stream: s,
	}
	srv.ReadPipe, srv.WritePipe = io.Pipe()
	return srv
}

func (s *StreamVirtual) Read(p []byte) (int, error) {
	return s.ReadPipe.Read(p)
}

func (s *StreamVirtual) Reset() error {
	return nil
}

func (s *StreamVirtual) FullClose() error {
	return nil
}

func (s *StreamVirtual) Close() error {
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
