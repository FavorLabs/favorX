package subkey

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/twystd/tweetnacl-go/tweetnacl"
	"golang.org/x/crypto/scrypt"
)

var Pkcs8Header = []byte{48, 83, 2, 1, 1, 48, 5, 6, 3, 43, 101, 112, 4, 34, 4, 32}
var Pkcs8Divider = []byte{161, 35, 3, 33, 0}
var SeedOffset = len(Pkcs8Header)
var ENCODING = []string{"scrypt", "xsalsa20-poly1305"}
var CONTENT = []string{"pkcs8", "sr25519"}

const (
	EncodingVersion = "3"
	NonceLength     = 24
	ScryptLength    = 32 + (3 * 4)
	PubLength       = 32
	SecLength       = 64
	SeedLength      = 32
)

type encryptedSubKey struct {
	Encoded  string   `json:"encoded"`
	Encoding Encoding `json:"encoding"`
	Address  string   `json:"address"`
	Meta     Meta     `json:"meta"`
}

type Encoding struct {
	Content []string `json:"content"`
	Type    []string `json:"type"`
	Version string   `json:"version"`
}

type Meta struct {
	IsHardware  bool     `json:"isHardware"`
	Name        string   `json:"name"`
	Tags        []string `json:"tags"`
	WhenCreated int64    `json:"whenCreated"`
}

type Params struct {
	N uint32
	P uint32
	r uint32
}

type encode struct {
	params   Params
	password []byte
	salt     []byte
}

func scryptToBytes(params Params, salt []byte) (out []byte) {
	salt = binary.LittleEndian.AppendUint32(salt, params.N)
	salt = binary.LittleEndian.AppendUint32(salt, params.P)
	salt = binary.LittleEndian.AppendUint32(salt, params.r)
	return salt
}

func scryptEncode(passphrase string) (e encode, err error) {
	salt := make([]byte, 32)
	io.ReadFull(rand.Reader, salt)
	password, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, scryptDKLen)
	if err != nil {
		return encode{}, err
	}
	return encode{
		params: Params{
			N: scryptN,
			P: scryptP,
			r: scryptR,
		},
		password: password,
		salt:     salt,
	}, nil
}

func naclEncrypt(encoded []byte, password []byte) (encrypted, nonce []byte, err error) {
	nonce = make([]byte, 24)
	io.ReadFull(rand.Reader, nonce)
	encrypted, err = tweetnacl.CryptoSecretBox(encoded, nonce, password)
	if err != nil {
		return nil, nil, err
	}
	return
}

func decodePair(passphrase string, encrypted []byte) (secretKey []byte, publicKey []byte, err error) {
	decrypted, err := jsonDecryptData(encrypted, passphrase)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(decrypted[:len(Pkcs8Header)], Pkcs8Header) {
		err = errors.New("invalid Pkcs8 header found in body")
		return
	}
	divOffset := SeedOffset + SecLength
	secretKey = decrypted[SeedOffset:divOffset]
	divider := decrypted[divOffset : divOffset+len(Pkcs8Divider)]
	// old-style, we have the seed here
	if !bytes.Equal(divider, Pkcs8Divider) {
		divOffset = SeedOffset + SeedLength
		secretKey = decrypted[SeedOffset:divOffset]
		divider = decrypted[divOffset : divOffset+len(Pkcs8Divider)]
		if !bytes.Equal(divider, Pkcs8Divider) {
			err = errors.New("invalid Pkcs8 divider found in body")
			return
		}
	}
	pubOffset := divOffset + len(Pkcs8Divider)
	publicKey = decrypted[pubOffset : pubOffset+PubLength]
	return
}

func jsonDecryptData(encrypted []byte, passphrase string) (encoded []byte, err error) {
	_, salt, err := scryptFromBytes(encrypted)
	if err != nil {
		return
	}
	var password []byte
	password, err = scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, scryptDKLen)
	if err != nil {
		return
	}
	encrypted = encrypted[ScryptLength:]
	encoded, err = tweetnacl.CryptoSecretBoxOpen(encrypted[NonceLength:], encrypted[:NonceLength], password)
	return
}

func scryptFromBytes(data []byte) (params Params, salt []byte, err error) {
	salt = data[:32]
	N := binary.LittleEndian.Uint32(data[32+0 : 32+4])
	P := binary.LittleEndian.Uint32(data[32+4 : 32+8])
	r := binary.LittleEndian.Uint32(data[32+8 : 32+12])
	// FIXME At this moment we assume these to be fixed params, this is not a great idea since we lose flexibility
	// and updates for greater security. However, we need some protection against carefully-crafted params that can
	// eat up CPU since these are user inputs. So we need to get very clever here, but atm we only allow the defaults
	// and if no match, bail out
	if N != scryptN || P != scryptP || r != scryptR {
		err = errors.New("invalid injected scrypt params found")
		return
	}
	return Params{
		N: N,
		P: P,
		r: r,
	}, salt, nil
}

func encryptKeyPair(kp crypto.Signer, password string) (marshal []byte, err error) {
	pk := kp.GetExportPrivateKey()
	var encoded []byte
	encoded = append(encoded, Pkcs8Header...)
	encoded = append(encoded, pk[:]...)
	encoded = append(encoded, Pkcs8Divider...)
	encoded = append(encoded, kp.Public().Encode()...)

	res, err := scryptEncode(password)
	if err != nil {
		return
	}
	encrypted, nonce, err := naclEncrypt(encoded, res.password[:32])

	var out []byte
	out = append(out, scryptToBytes(res.params, res.salt)...)
	out = append(out, nonce...)
	out = append(out, encrypted...)

	encodedOut := base64.StdEncoding.EncodeToString(out)

	v := encryptedSubKey{
		Encoded: encodedOut,
		Encoding: Encoding{
			Content: CONTENT,
			Type:    ENCODING,
			Version: EncodingVersion,
		},
		Address: string(kp.Public().Address()),
		Meta: Meta{
			WhenCreated: time.Now().Unix(),
			Tags:        []string{},
		},
	}
	return json.Marshal(v)
}

func decryptKeyPair(decryptData []byte, password string) (crypto.Signer, error) {
	var data encryptedSubKey
	err := json.Unmarshal(decryptData, &data)
	if err != nil {
		return nil, err
	}
	decodeBytes, err := base64.StdEncoding.DecodeString(data.Encoded)
	if err != nil {
		return nil, err
	}

	sk, pub, err := decodePair(password, decodeBytes)
	if err != nil {
		return nil, err
	}
	signer, err := crypto.NewKeypairFromExportPrivateKey(sk)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(signer.Public().Encode(), pub) {
		return nil, errors.New("invalid scrypt key ")
	}
	return signer, nil
}
