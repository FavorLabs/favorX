package store

import (
	"context"
	"encoding/binary"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/encryption"
	"github.com/FavorLabs/favorX/pkg/storage"
	"golang.org/x/crypto/sha3"
)

type decryptingStore struct {
	storage.Getter
}

func New(s storage.Getter) storage.Getter {
	return &decryptingStore{s}
}

func (s *decryptingStore) Get(ctx context.Context, mode storage.ModeGet, addr boson.Address, index int64) (ch boson.Chunk, err error) {
	switch l := len(addr.Bytes()); l {
	case boson.HashSize:
		// normal, unencrypted content
		return s.Getter.Get(ctx, mode, addr, index)

	case encryption.ReferenceSize:
		// encrypted reference
		ref := addr.Bytes()
		address := boson.NewAddress(ref[:boson.HashSize])
		ch, err := s.Getter.Get(ctx, mode, address, index)
		if err != nil {
			return nil, err
		}

		d, err := decryptChunkData(ch.Data(), ref[boson.HashSize:])
		if err != nil {
			return nil, err
		}
		return boson.NewChunk(address, d), nil

	default:
		return nil, storage.ErrReferenceLength
	}
}

func decryptChunkData(chunkData []byte, encryptionKey encryption.Key) ([]byte, error) {
	decryptedSpan, decryptedData, err := decrypt(chunkData, encryptionKey)
	if err != nil {
		return nil, err
	}

	// removing extra bytes which were just added for padding
	length := binary.LittleEndian.Uint64(decryptedSpan)
	refSize := int64(boson.HashSize + encryption.KeyLength)
	for length > boson.ChunkSize {
		length = length + (boson.ChunkSize - 1)
		length = length / boson.ChunkSize
		length *= uint64(refSize)
	}

	c := make([]byte, length+8)
	copy(c[:8], decryptedSpan)
	copy(c[8:], decryptedData[:length])

	return c, nil
}

func decrypt(chunkData []byte, key encryption.Key) ([]byte, []byte, error) {
	decryptedSpan, err := newSpanEncryption(key).Decrypt(chunkData[:boson.SpanSize])
	if err != nil {
		return nil, nil, err
	}
	decryptedData, err := newDataEncryption(key).Decrypt(chunkData[boson.SpanSize:])
	if err != nil {
		return nil, nil, err
	}
	return decryptedSpan, decryptedData, nil
}

func newSpanEncryption(key encryption.Key) encryption.Interface {
	refSize := int64(boson.HashSize + encryption.KeyLength)
	return encryption.New(key, 0, uint32(boson.ChunkSize/refSize), sha3.NewLegacyKeccak256)
}

func newDataEncryption(key encryption.Key) encryption.Interface {
	return encryption.New(key, int(boson.ChunkSize), 0, sha3.NewLegacyKeccak256)
}
