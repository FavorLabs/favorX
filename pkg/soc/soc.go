// Package soc provides the single-owner chunk implementation
// and validator.
package soc

import (
	"bytes"
	"errors"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/cac"
	"github.com/FavorLabs/favorX/pkg/crypto"
)

const (
	IdSize        = 32
	OwnerSize     = 32
	SignatureSize = 64
	minChunkSize  = IdSize + OwnerSize + SignatureSize + boson.SpanSize
)

var (
	errInvalidAddress = errors.New("soc: invalid address")
	errWrongChunkSize = errors.New("soc: chunk length is less than minimum")
)

// ID is a SOC identifier
type ID []byte

// SOC wraps a content-addressed chunk.
type SOC struct {
	id        ID
	owner     []byte // owner is the address in bytes of SOC owner.
	signature []byte
	chunk     boson.Chunk // wrapped chunk.
}

// New creates a new SOC representation from arbitrary id and
// a content-addressed chunk.
func New(id ID, ch boson.Chunk) *SOC {
	return &SOC{
		id:    id,
		chunk: ch,
	}
}

// NewSigned creates a single-owner chunk based on already signed data.
func NewSigned(id ID, ch boson.Chunk, owner, sig []byte) (*SOC, error) {
	s := New(id, ch)
	if len(owner) != OwnerSize {
		return nil, errInvalidAddress
	}
	s.owner = owner
	s.signature = sig
	return s, nil
}

// address returns the SOC chunk address.
func (s *SOC) address() (boson.Address, error) {
	if len(s.owner) != OwnerSize {
		return boson.ZeroAddress, errInvalidAddress
	}
	return CreateAddress(s.id, s.owner)
}

// WrappedChunk returns the chunk wrapped by the SOC.
func (s *SOC) WrappedChunk() boson.Chunk {
	return s.chunk
}

// Chunk returns the SOC chunk.
func (s *SOC) Chunk() (boson.Chunk, error) {
	socAddress, err := s.address()
	if err != nil {
		return nil, err
	}
	return boson.NewChunk(socAddress, s.toBytes()), nil
}

// toBytes is a helper function to convert the SOC data to bytes.
func (s *SOC) toBytes() []byte {
	buf := bytes.NewBuffer(nil)
	buf.Write(s.id)
	buf.Write(s.owner)
	buf.Write(s.signature)
	buf.Write(s.chunk.Data())
	return buf.Bytes()
}

// Sign signs a SOC using the given signer.
// It returns a signed SOC chunk ready for submission to the network.
func (s *SOC) Sign(signer crypto.Signer) (boson.Chunk, error) {
	s.owner = signer.Public().Encode()

	// generate the data to sign
	toSignBytes, err := hash(s.id, s.chunk.Address().Bytes())
	if err != nil {
		return nil, err
	}

	// sign the chunk
	signature, err := signer.Sign(toSignBytes)
	if err != nil {
		return nil, err
	}
	s.signature = signature

	return s.Chunk()
}

// FromChunk recreates a SOC representation from boson.Chunk data.
func FromChunk(sch boson.Chunk) (*SOC, error) {
	chunkData := sch.Data()
	if len(chunkData) < minChunkSize {
		return nil, errWrongChunkSize
	}

	// add all the data fields to the SOC
	s := &SOC{}
	cursor := 0

	s.id = chunkData[cursor:IdSize]
	cursor += IdSize

	s.owner = chunkData[cursor : cursor+OwnerSize]
	cursor += OwnerSize

	s.signature = chunkData[cursor : cursor+SignatureSize]
	cursor += SignatureSize

	ch, err := cac.NewWithDataSpan(chunkData[cursor:])
	if err != nil {
		return nil, err
	}

	toSignBytes, err := hash(s.id, ch.Address().Bytes())
	if err != nil {
		return nil, err
	}

	pub, err := crypto.NewPublicKey(s.owner)
	if err != nil {
		return nil, err
	}
	_, err = pub.Verify(toSignBytes, s.signature)
	if err != nil {
		return nil, err
	}
	s.chunk = ch

	return s, nil
}

// CreateAddress creates a new SOC address from the id and
// the ethereum address of the owner.
func CreateAddress(id ID, owner []byte) (boson.Address, error) {
	sum, err := hash(id, owner)
	if err != nil {
		return boson.ZeroAddress, err
	}
	return boson.NewAddress(sum), nil
}

// hash the given values in order.
func hash(values ...[]byte) ([]byte, error) {
	h := boson.NewHasher()
	for _, v := range values {
		_, err := h.Write(v)
		if err != nil {
			return nil, err
		}
	}
	return h.Sum(nil), nil
}
