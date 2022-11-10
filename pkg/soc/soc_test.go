package soc_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/cac"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/soc"
)

func TestNew(t *testing.T) {
	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, soc.IdSize)
	s := soc.New(id, ch)

	// check SOC fields
	if !bytes.Equal(s.ID(), id) {
		t.Fatalf("id mismatch. got %x want %x", s.ID(), id)
	}

	chunkData := s.WrappedChunk().Data()
	spanBytes := make([]byte, boson.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	if !bytes.Equal(chunkData[:boson.SpanSize], spanBytes) {
		t.Fatalf("span mismatch. got %x want %x", chunkData[:boson.SpanSize], spanBytes)
	}

	if !bytes.Equal(chunkData[boson.SpanSize:], payload) {
		t.Fatalf("payload mismatch. got %x want %x", chunkData[boson.SpanSize:], payload)
	}
}

func TestNewSigned(t *testing.T) {
	owner, _ := crypto.HexToBytes("0x307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f7313")
	// signature of hash(id + chunk address of foo)
	sig, err := hex.DecodeString("5acd384febc133b7b245e5ddc62d82d2cded9182d2716126cd8844509af65a053deb418208027f548e3e88343af6f84a8772fb3cebc0a1833a0ea7ec0c134831")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, soc.IdSize)
	s, err := soc.NewSigned(id, ch, owner, sig)
	if err != nil {
		t.Fatal(err)
	}

	// check signed SOC fields
	if !bytes.Equal(s.ID(), id) {
		t.Fatalf("id mismatch. got %x want %x", s.ID(), id)
	}

	if !bytes.Equal(s.OwnerAddress(), owner) {
		t.Fatalf("owner mismatch. got %x want %x", s.OwnerAddress(), owner)
	}

	if !bytes.Equal(s.Signature(), sig) {
		t.Fatalf("signature mismatch. got %x want %x", s.Signature(), sig)
	}

	chunkData := s.WrappedChunk().Data()
	spanBytes := make([]byte, boson.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	if !bytes.Equal(chunkData[:boson.SpanSize], spanBytes) {
		t.Fatalf("span mismatch. got %x want %x", chunkData[:boson.SpanSize], spanBytes)
	}

	if !bytes.Equal(chunkData[boson.SpanSize:], payload) {
		t.Fatalf("payload mismatch. got %x want %x", chunkData[boson.SpanSize:], payload)
	}
}

// TestChunk verifies that the chunk created from the SOC object
// corresponds to the SOC spec.
func TestChunk(t *testing.T) {
	owner, _ := crypto.HexToBytes("0x307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f7313")
	sig, err := hex.DecodeString("9c3ae8f3cc5ed0220e3f27d103787fcf61ced6488c1a90705542c3bb9c2e6f64cac570b65dc2cbecd24b3d1a01671772c1b3f4f34a22c92cb02a95051c00838c")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, soc.IdSize)
	// creates a new signed SOC
	s, err := soc.NewSigned(id, ch, owner, sig)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := soc.Hash(id, owner)
	if err != nil {
		t.Fatal(err)
	}
	expectedSOCAddress := boson.NewAddress(sum)

	// creates SOC chunk
	sch, err := s.Chunk()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(sch.Address().Bytes(), expectedSOCAddress.Bytes()) {
		t.Fatalf("soc address mismatch. got %x want %x", sch.Address().Bytes(), expectedSOCAddress.Bytes())
	}

	chunkData := sch.Data()
	println(sch.Address().String())
	println(crypto.BytesToHex(chunkData))
	// verifies that id, signature, payload is in place in the SOC chunk
	cursor := 0
	if !bytes.Equal(chunkData[cursor:soc.IdSize], id) {
		t.Fatalf("id mismatch. got %x want %x", chunkData[cursor:soc.IdSize], id)
	}
	cursor += soc.IdSize

	ow := chunkData[cursor : cursor+soc.OwnerSize]
	if !bytes.Equal(ow, owner) {
		t.Fatalf("owner mismatch. got %x want %x", ow, owner)
	}
	cursor += soc.OwnerSize

	signature := chunkData[cursor : cursor+soc.SignatureSize]
	if !bytes.Equal(signature, sig) {
		t.Fatalf("signature mismatch. got %x want %x", signature, sig)
	}
	cursor += soc.SignatureSize

	spanBytes := make([]byte, boson.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	if !bytes.Equal(chunkData[cursor:cursor+boson.SpanSize], spanBytes) {
		t.Fatalf("span mismatch. got %x want %x", chunkData[cursor:cursor+boson.SpanSize], spanBytes)
	}
	cursor += boson.SpanSize

	if !bytes.Equal(chunkData[cursor:], payload) {
		t.Fatalf("payload mismatch. got %x want %x", chunkData[cursor:], payload)
	}
}

func TestChunkErrorWithoutOwner(t *testing.T) {
	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, soc.IdSize)

	// creates a new soc
	s := soc.New(id, ch)

	_, err = s.Chunk()
	if !errors.Is(err, soc.ErrInvalidAddress) {
		t.Fatalf("expect error. got `%v` want `%v`", err, soc.ErrInvalidAddress)
	}
}

// TestSign tests whether a soc is correctly signed.
func TestSign(t *testing.T) {
	signer := crypto.NewDefaultSigner()

	payload := []byte("foo")
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, soc.IdSize)
	// creates the soc
	s := soc.New(id, ch)

	// signs the chunk
	sch, err := s.Sign(signer)
	if err != nil {
		t.Fatal(err)
	}

	chunkData := sch.Data()
	// get signature in the chunk
	cursor := soc.IdSize

	pub := chunkData[cursor : cursor+soc.OwnerSize]
	cursor += soc.OwnerSize

	signature := chunkData[cursor : cursor+soc.SignatureSize]

	publicKey, err := crypto.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	toSignBytes, err := soc.Hash(id, ch.Address().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// verifies if the owner matches
	ok, err := publicKey.Verify(toSignBytes, signature)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatalf("signature error")
	}
}

// TestFromChunk verifies that valid chunk data deserializes to
// a fully populated soc object.
func TestFromChunk(t *testing.T) {
	socAddress := boson.MustParseHexAddress("034d0670c51f3737f3d08256c20440e72001aea2628ad0bfdd2a774eb5364d39")

	dataBytes, _ := crypto.HexToBytes("0x0000000000000000000000000000000000000000000000000000000000000000307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f73139c3ae8f3cc5ed0220e3f27d103787fcf61ced6488c1a90705542c3bb9c2e6f64cac570b65dc2cbecd24b3d1a01671772c1b3f4f34a22c92cb02a95051c00838c0300000000000000666f6f")
	// signed soc chunk of:
	// id: 0
	// wrapped chunk of: `foo`
	// owner: 0x307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f7313
	sch := boson.NewChunk(socAddress, dataBytes)

	data := sch.Data()
	id := data[:soc.IdSize]
	cursor := soc.IdSize

	owner := data[cursor : cursor+soc.OwnerSize]
	cursor += soc.OwnerSize

	sig := data[cursor : cursor+soc.SignatureSize]
	cursor += soc.SignatureSize

	chunkData := data[cursor:]

	chunkAddress := boson.MustParseHexAddress("8a74889a73c23fe2be037886c6b709e3175b95b8deea9c95eeda0dbc60740bd8")
	ch := boson.NewChunk(chunkAddress, chunkData)

	signedDigest, err := soc.Hash(id, ch.Address().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err := crypto.NewPublicKey(owner)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := publicKey.Verify(signedDigest, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("signer error")
	}

	// attempt to recover soc from signed chunk
	recoveredSOC, err := soc.FromChunk(sch)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(recoveredSOC.ID(), id) {
		t.Fatalf("id mismatch. got %x want %x", recoveredSOC.ID(), id)
	}

	if !bytes.Equal(recoveredSOC.Signature(), sig) {
		t.Fatalf("signature mismatch. got %x want %x", recoveredSOC.Signature(), sig)
	}

	if !ch.Equal(recoveredSOC.WrappedChunk()) {
		t.Fatalf("wrapped chunk mismatch. got %s want %s", recoveredSOC.WrappedChunk().Address(), ch.Address())
	}
}

func TestCreateAddress(t *testing.T) {
	id := make([]byte, soc.IdSize)
	owner, err := crypto.HexToBytes("0x307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f7313")
	if err != nil {
		t.Fatal(err)
	}
	socAddress := boson.MustParseHexAddress("034d0670c51f3737f3d08256c20440e72001aea2628ad0bfdd2a774eb5364d39")

	addr, err := soc.CreateAddress(id, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !addr.Equal(socAddress) {
		t.Fatalf("soc address mismatch. got %s want %s", addr, socAddress)
	}
}

func TestRecoverAddress(t *testing.T) {
	owner, _ := crypto.HexToBytes("0x307bfefaeabe8c8c1e78698808f2919eaf87aa0bbcab11fd434dd063398f7313")
	id := make([]byte, soc.IdSize)
	chunkAddress := boson.MustParseHexAddress("8a74889a73c23fe2be037886c6b709e3175b95b8deea9c95eeda0dbc60740bd8")
	signedDigest, err := soc.Hash(id, chunkAddress.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	sig, err := hex.DecodeString("9c3ae8f3cc5ed0220e3f27d103787fcf61ced6488c1a90705542c3bb9c2e6f64cac570b65dc2cbecd24b3d1a01671772c1b3f4f34a22c92cb02a95051c00838c")
	if err != nil {
		t.Fatal(err)
	}

	publicKey, _ := crypto.NewPublicKey(owner)

	ok, err := publicKey.Verify(signedDigest, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("signer error")
	}
}

func TestKeyPair(t *testing.T) {
	kp, err := crypto.NewKeypairFromSeedHex("0xbc6e9c8bd7c38fbbde9e8f0d460b58fb330903f401878010d2f1f7bcca5558ec")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(kp.Private().Hex())
	t.Log(kp.Public().Hex())

	id := make([]byte, soc.IdSize)
	chunkAddress := boson.MustParseHexAddress("8a74889a73c23fe2be037886c6b709e3175b95b8deea9c95eeda0dbc60740bd8")
	signedDigest, err := soc.Hash(id, chunkAddress.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	sign, err := kp.Private().Sign(signedDigest)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(crypto.BytesToHex(sign))
}
