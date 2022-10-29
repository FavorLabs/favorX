// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/FavorLabs/favorX/pkg/boson"
)

// Version constants.
const (
	versionNameString   = "mantaray"
	versionCode01String = "0.1"
	versionCode02String = "0.2"

	versionSeparatorString = ":"

	version01String     = versionNameString + versionSeparatorString + versionCode01String   // "mantaray:0.1"
	version01HashString = "025184789d63635766d78c41900196b57d7400875ebe4d9b5d1e76bd9652a9b7" // pre-calculated version string, Keccak-256

	version02String     = versionNameString + versionSeparatorString + versionCode02String   // "mantaray:0.2"
	version02HashString = "5768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f7b" // pre-calculated version string, Keccak-256
)

// Node header fields constants.
const (
	nodeObfuscationKeySize = 32
	versionHashSize        = 31
	nodeRefBytesSize       = 1

	// nodeHeaderSize defines the total size of the header part
	nodeHeaderSize = nodeObfuscationKeySize + versionHashSize + nodeRefBytesSize
)

// Node fork constats.
const (
	nodeForkTypeBytesSize    = 1
	nodeForkPrefixBytesSize  = 1
	nodeForkHeaderSize       = nodeForkTypeBytesSize + nodeForkPrefixBytesSize // 2
	nodeForkPreReferenceSize = 32
	nodePrefixMaxSize        = nodeForkPreReferenceSize - nodeForkHeaderSize // 30
	// "mantaray:0.2"
	nodeForkMetadataBytesSize = 2
	nodeTypeWithMetadata      = uint8(16)
)
const nodeTypeValue = uint8(2)

var (
	version01HashBytes []byte
	version02HashBytes []byte
	zero32             []byte
)

func init() {
	initVersion(version01HashString, &version01HashBytes)
	initVersion(version02HashString, &version02HashBytes)
	zero32 = make([]byte, 32)
}

func initVersion(hash string, bytes *[]byte) {
	b, err := hex.DecodeString(hash)
	if err != nil {
		panic(err)
	}

	*bytes = make([]byte, versionHashSize)
	copy(*bytes, b)
}

var (
	// ErrTooShort signals too short input.
	ErrTooShort = errors.New("serialised input too short")
	// ErrInvalidInput signals invalid input to serialise.
	ErrInvalidInput = errors.New("input invalid")
	// ErrInvalidVersionHash signals unknown version of hash.
	ErrInvalidVersionHash = errors.New("invalid version hash")
)

func nodeTypeIsWithMetadataType(nodeType uint8) bool {
	return nodeType&nodeTypeWithMetadata == nodeTypeWithMetadata
}

// bitsForBytes is a set of bytes represented as a 256-length bitvector
type bitsForBytes struct {
	bits [32]byte
}

func (bb *bitsForBytes) bytes() (b []byte) {
	b = append(b, bb.bits[:]...)
	return b
}

func (bb *bitsForBytes) fromBytes(b []byte) {
	copy(bb.bits[:], b)
}

func (bb *bitsForBytes) set(b byte) {
	bb.bits[b/8] |= 1 << (b % 8)
}

// nolint,unused
func (bb *bitsForBytes) get(b byte) bool {
	return bb.getUint8(b)
}

func (bb *bitsForBytes) getUint8(i uint8) bool {
	return (bb.bits[i/8]>>(i%8))&1 > 0
}

func (bb *bitsForBytes) iter(f func(byte) error) error {
	for i := uint8(0); ; i++ {
		if bb.getUint8(i) {
			if err := f(i); err != nil {
				return err
			}
		}
		if i == 255 {
			return nil
		}
	}
}

type node struct {
	nodeType uint8
	address  boson.Address
}

// UnmarshalBinary deserialises a node
func UnmarshalBinary(data []byte) ([]*node, error) {
	if len(data) < nodeHeaderSize {
		return nil, ErrTooShort
	}

	obfuscationKey := append([]byte{}, data[0:nodeObfuscationKeySize]...)

	// perform XOR decryption on bytes after obfuscation key
	xorDecryptedBytes := make([]byte, len(data))

	copy(xorDecryptedBytes, data[0:nodeObfuscationKeySize])

	for i := nodeObfuscationKeySize; i < len(data); i += nodeObfuscationKeySize {
		end := i + nodeObfuscationKeySize
		if end > len(data) {
			end = len(data)
		}

		decrypted := encryptDecrypt(data[i:end], obfuscationKey)
		copy(xorDecryptedBytes[i:end], decrypted)
	}

	data = xorDecryptedBytes

	// Verify version hash.
	versionHash := data[nodeObfuscationKeySize : nodeObfuscationKeySize+versionHashSize]

	if bytes.Equal(versionHash, version01HashBytes) {

		refBytesSize := int(data[nodeHeaderSize-1])

		offset := nodeHeaderSize + refBytesSize // skip entry
		nodes := make([]*node, 0, 2)
		bb := &bitsForBytes{}
		bb.fromBytes(data[offset:])
		offset += 32 // skip forks
		err := bb.iter(func(b byte) error {
			if len(data) < offset+nodeForkPreReferenceSize+refBytesSize {
				err := fmt.Errorf("not enough bytes for node fork: %d (%d)", (len(data) - offset), (nodeForkPreReferenceSize + refBytesSize))
				return fmt.Errorf("%w on byte '%x'", err, []byte{b})
			}

			n, err := fromBytes(data[offset : offset+nodeForkPreReferenceSize+refBytesSize])
			if err != nil {
				return fmt.Errorf("%w on byte '%x'", err, []byte{b})
			}
			nodes = append(nodes, n)
			offset += nodeForkPreReferenceSize + refBytesSize
			return nil
		})
		return nodes, err
	} else if bytes.Equal(versionHash, version02HashBytes) {

		refBytesSize := int(data[nodeHeaderSize-1])

		offset := nodeHeaderSize + refBytesSize // skip entry

		nodes := make([]*node, 0, 2)
		bb := &bitsForBytes{}
		bb.fromBytes(data[offset:])
		offset += 32 // skip forks
		err := bb.iter(func(b byte) error {

			if len(data) < offset+nodeForkTypeBytesSize {
				return fmt.Errorf("not enough bytes for node fork: %d (%d) on byte '%x'", (len(data) - offset), (nodeForkTypeBytesSize), []byte{b})
			}

			nodeType := data[offset]

			nodeForkSize := nodeForkPreReferenceSize + refBytesSize

			if nodeTypeIsWithMetadataType(nodeType) {
				if len(data) < offset+nodeForkPreReferenceSize+refBytesSize+nodeForkMetadataBytesSize {
					return fmt.Errorf("not enough bytes for node fork: %d (%d) on byte '%x'", (len(data) - offset), (nodeForkPreReferenceSize + refBytesSize + nodeForkMetadataBytesSize), []byte{b})
				}

				metadataBytesSize := binary.BigEndian.Uint16(data[offset+nodeForkSize : offset+nodeForkSize+nodeForkMetadataBytesSize])

				nodeForkSize += nodeForkMetadataBytesSize
				nodeForkSize += int(metadataBytesSize)

				n, err := fromBytes02(data[offset:offset+nodeForkSize], refBytesSize)
				if err != nil {
					return fmt.Errorf("%w on byte '%x'", err, []byte{b})
				}
				nodes = append(nodes, n)
			} else {
				if len(data) < offset+nodeForkPreReferenceSize+refBytesSize {
					return fmt.Errorf("not enough bytes for node fork: %d (%d) on byte '%x'", (len(data) - offset), (nodeForkPreReferenceSize + refBytesSize), []byte{b})
				}

				n, err := fromBytes(data[offset : offset+nodeForkSize])
				if err != nil {
					return fmt.Errorf("%w on byte '%x'", err, []byte{b})
				}
				nodes = append(nodes, n)
			}
			offset += nodeForkSize
			return nil
		})
		return nodes, err
	}

	return nil, fmt.Errorf("%x: %w", versionHash, ErrInvalidVersionHash)
}

func fromBytes(b []byte) (*node, error) {
	n := &node{}
	nodeType := b[0]
	prefixLen := int(b[1])

	if prefixLen == 0 || prefixLen > nodePrefixMaxSize {
		return nil, fmt.Errorf("invalid prefix length: %d", prefixLen)
	}
	n.nodeType = nodeType
	n.address = boson.NewAddress(b[nodeForkPreReferenceSize:])
	return n, nil

}

func fromBytes02(b []byte, refBytesSize int) (*node, error) {
	n := &node{}
	nodeType := b[0]
	prefixLen := int(b[1])
	if prefixLen == 0 || prefixLen > nodePrefixMaxSize {
		return nil, fmt.Errorf("invalid prefix length: %d", prefixLen)
	}
	n.nodeType = nodeType
	n.address = boson.NewAddress(b[nodeForkPreReferenceSize : nodeForkPreReferenceSize+refBytesSize])
	return n, nil
}

// encryptDecrypt runs a XOR encryption on the input bytes, encrypting it if it
// hasn't already been, and decrypting it if it has, using the key provided.
func encryptDecrypt(input, key []byte) []byte {
	output := make([]byte, len(input))

	for i := 0; i < len(input); i++ {
		output[i] = input[i] ^ key[i%len(key)]
	}

	return output
}
func IsValueType(nodeType uint8) bool {
	return nodeType&nodeTypeValue == nodeTypeValue
}
