package soc_test

import (
	"strings"
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/soc"
)

// TestValid verifies that the validator can detect
// valid soc chunks.
func TestValid(t *testing.T) {
	socAddress := boson.MustParseHexAddress("034d0670c51f3737f3d08256c20440e72001aea2628ad0bfdd2a774eb5364d39")

	dataBytes, _ := crypto.HexToBytes("0x0000000000000000000000000000000000000000000000000000000000000000307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f73139c3ae8f3cc5ed0220e3f27d103787fcf61ced6488c1a90705542c3bb9c2e6f64cac570b65dc2cbecd24b3d1a01671772c1b3f4f34a22c92cb02a95051c00838c0300000000000000666f6f")

	// signed soc chunk of:
	// id: 0
	// wrapped chunk of: `foo`
	// owner: 0x307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f7313
	sch := boson.NewChunk(socAddress, dataBytes)

	// check valid chunk
	if !soc.Valid(sch) {
		t.Fatal("valid chunk evaluates to invalid")
	}
}

// TestInvalid verifies that the validator can detect chunks
// with invalid data and invalid address.
func TestInvalid(t *testing.T) {
	socAddress := boson.MustParseHexAddress("034d0670c51f3737f3d08256c20440e72001aea2628ad0bfdd2a774eb5364d39")

	dataBytes, _ := crypto.HexToBytes("0x0000000000000000000000000000000000000000000000000000000000000000307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f73139c3ae8f3cc5ed0220e3f27d103787fcf61ced6488c1a90705542c3bb9c2e6f64cac570b65dc2cbecd24b3d1a01671772c1b3f4f34a22c92cb02a95051c00838c0300000000000000666f6f")

	// signed soc chunk of:
	// id: 0
	// wrapped chunk of: `foo`
	// owner: 0x307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f7313
	sch := boson.NewChunk(socAddress, dataBytes)

	for _, c := range []struct {
		name  string
		chunk func() boson.Chunk
	}{
		{
			name: "wrong soc address",
			chunk: func() boson.Chunk {
				wrongAddressBytes := sch.Address().Bytes()
				wrongAddressBytes[0] = 255 - wrongAddressBytes[0]
				wrongAddress := boson.NewAddress(wrongAddressBytes)
				return boson.NewChunk(wrongAddress, sch.Data())
			},
		},
		{
			name: "invalid data",
			chunk: func() boson.Chunk {
				data := make([]byte, len(sch.Data()))
				copy(data, sch.Data())
				cursor := soc.IdSize + soc.SignatureSize
				chunkData := data[cursor:]
				chunkData[0] = 0x01
				return boson.NewChunk(socAddress, data)
			},
		},
		{
			name: "invalid id",
			chunk: func() boson.Chunk {
				data := make([]byte, len(sch.Data()))
				copy(data, sch.Data())
				id := data[:soc.IdSize]
				id[0] = 0x01
				return boson.NewChunk(socAddress, data)
			},
		},
		{
			name: "invalid signature",
			chunk: func() boson.Chunk {
				data := make([]byte, len(sch.Data()))
				copy(data, sch.Data())
				// modify signature
				cursor := soc.IdSize + soc.SignatureSize
				sig := data[soc.IdSize:cursor]
				sig[0] = 0x01
				return boson.NewChunk(socAddress, data)
			},
		},
		{
			name: "nil data",
			chunk: func() boson.Chunk {
				return boson.NewChunk(socAddress, nil)
			},
		},
		{
			name: "small data",
			chunk: func() boson.Chunk {
				return boson.NewChunk(socAddress, []byte("small"))
			},
		},
		{
			name: "large data",
			chunk: func() boson.Chunk {
				return boson.NewChunk(socAddress, []byte(strings.Repeat("a", boson.ChunkSize+boson.SpanSize+1)))
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if soc.Valid(c.chunk()) {
				t.Fatal("chunk with invalid data evaluates to valid")
			}
		})
	}
}
