package subkey

import (
	"bytes"
	"testing"

	"github.com/FavorLabs/favorX/pkg/crypto"
)

var node1 = "{\"encoded\":\"/iRHztPVc12bejWA/IdsxAYI9fTes0PC16bft8n+beAAgAAAAQAAAAgAAAC8thFAyVDvkz0ekTuuvEM5F8o+tGTeyzaDnZff4rS0hNvAsILd1E5xfgmvkfe3HcnG+rEI1+8yTGUasUgD8VdtRNWVNX2qcA96OSHs3VE3nvxH8C3SHT0Gnz9prKKZREUOSwGCEAhniAH8W6loWbgfv9a+FROvBxNGOb991s2LaQ+rznyWj328MPTdXHdGS3S2cilbzOLfIVpVuOhs\",\"encoding\":{\"content\":[\"pkcs8\",\"sr25519\"],\"type\":[\"scrypt\",\"xsalsa20-poly1305\"],\"version\":\"3\"},\"address\":\"5G1FkjcvB1ct2dk7S8k4nUGjRooETh71UFyJiVFeyZk4tHvL\",\"meta\":{\"isHardware\":false,\"name\":\"\",\"tags\":[],\"whenCreated\":1668478979}}"
var node1Appjs = "{\"encoded\":\"F5sUho/1cSQBErJdzgyBxjeOuS0q445zkYs7kSMnOeEAgAAAAQAAAAgAAAABqXXZ8yEo7Ulco9fSuGR9RUd2qqhWvFMBqHhqwXX07KeP6Q5CfV8r5pOoyFav8e0+zffNQYdHke2M617srQCyp/joKLY/NIpCu/qs70QXPvCuPCv8s2RqCgCqCxXqurmFd0UPkq0J3VlmXd5DUV2FBBlbBQPL+0HbVmSLmZ/GcmPWQy9IJWKfJS6vHX1/nGl645u2J9VrbWHJFkNX\",\"encoding\":{\"content\":[\"pkcs8\",\"sr25519\"],\"type\":[\"scrypt\",\"xsalsa20-poly1305\"],\"version\":\"3\"},\"address\":\"5G1FkjcvB1ct2dk7S8k4nUGjRooETh71UFyJiVFeyZk4tHvL\",\"meta\":{\"isHardware\":false,\"name\":\"node1\",\"tags\":[],\"whenCreated\":1668422186388}}"

func Test_EncodePair(t *testing.T) {
	kp, err := crypto.NewKeypairFromMnemonic("nominee wet cruise supply distance orphan spider alert dream enact rather salmon")
	if err != nil {
		t.Fatal(err)
	}
	b, err := encryptKeyPair(kp, "")
	if err != nil {
		t.Fatal(err)
	}
	pair, err := decryptKeyPair(b, "")
	if err != nil {
		t.Fatal(err)
	}
	if pair.GetExportPrivateKey() != kp.GetExportPrivateKey() {
		t.Fatalf("mismatch scrypt key")
	}
}

func Test_DecodePair(t *testing.T) {
	kp, _ := crypto.NewKeypairFromMnemonic("nominee wet cruise supply distance orphan spider alert dream enact rather salmon")

	pair1, err := decryptKeyPair([]byte(node1), "123456")
	if err != nil {
		t.Fatal(err)
	}

	pair, err := decryptKeyPair([]byte(node1Appjs), "123456")
	if err != nil {
		t.Fatal(err)
	}
	if pair.GetExportPrivateKey() != pair1.GetExportPrivateKey() {
		t.Fatalf("mismatch private key")
	}
	sign, err := kp.Sign([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	verify, err := pair.Public().Verify([]byte("hello"), sign)
	if err != nil {
		t.Fatal(err)
	}
	if !verify {
		t.Fatalf("verify failed")
	}
	sk := pair.GetExportPrivateKey()
	key, err := crypto.NewKeypairFromExportPrivateKey(sk[:])
	if err != nil {
		t.Fatal(err)
	}
	if key.Private().Hex() != pair.Private().Hex() || pair.Private().Hex() != kp.Private().Hex() {
		t.Fatalf("mismatch private key")
	}
	if !bytes.Equal(key.GetSecretKey64(), pair.GetSecretKey64()) {
		t.Fatalf("mismatch secret key")
	}
	if !bytes.Equal(kp.GetSecretKey64(), pair.GetSecretKey64()) {
		t.Fatalf("mismatch secret key")
	}
}
