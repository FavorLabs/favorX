package subkey_test

var PKCS8_HEADER = []byte{48, 83, 2, 1, 1, 48, 5, 6, 3, 43, 101, 112, 4, 34, 4, 32}
var PKCS8_DIVIDER = []byte{161, 35, 3, 33, 0}
var SEED_OFFSET = len(PKCS8_HEADER)

const (
	PUB_LENGTH  = 32
	SALT_LENGTH = 32
	SEC_LENGTH  = 64
	SEED_LENGTH = 32
)

//
// func TestNew(t *testing.T) {
// 	secretKeyBytes, err := hex.DecodeString("6368616e676520746869732070617373776f726420746f206120736563726574")
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	var secretKey [32]byte
// 	copy(secretKey[:], secretKeyBytes)
//
// 	// You must use a different nonce for each message you encrypt with the
// 	// same key. Since the nonce here is 192 bits long, a random value
// 	// provides a sufficiently small probability of repeats.
// 	var nonce [24]byte
// 	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
// 		panic(err)
// 	}
//
// 	// This encrypts "hello world" and appends the result to the nonce.
// 	encrypted := secretbox.Seal(nonce[:], []byte("hello world"), &nonce, &secretKey)
//
// 	// When you decrypt, you must use the same nonce and key you used to
// 	// encrypt the message. One way to achieve this is to store the nonce
// 	// alongside the encrypted message. Above, we stored the nonce in the first
// 	// 24 bytes of the encrypted text.
// 	var decryptNonce [24]byte
// 	copy(decryptNonce[:], encrypted[:24])
// 	decrypted, ok := secretbox.Open(nil, encrypted[24:], &decryptNonce, &secretKey)
// 	if !ok {
// 		panic("decryption error")
// 	}
//
// 	fmt.Println(string(decrypted))
// }
//
// func TestService_Exists(t *testing.T) {
// 	// encrypted, nonce, secret
//
// }
//
// func decodePair(passphrase string, encrypted []byte) (secretKey []byte, publicKey []byte, err error) {
// 	encType := []string{"scrypt", "xsalsa20-poly1305"}
// 	decrypted := jsonDecryptData(encrypted, passphrase, encType)
// 	if !bytes.Equal(decrypted[:len(PKCS8_HEADER)], PKCS8_HEADER) {
// 		err = errors.New("invalid Pkcs8 header found in body")
// 		return
// 	}
// 	divOffset := SEED_OFFSET + SEC_LENGTH
// 	secretKey = decrypted[SEED_OFFSET:divOffset]
// 	divider := decrypted[divOffset : divOffset+len(PKCS8_DIVIDER)]
// 	// old-style, we have the seed here
// 	if !bytes.Equal(divider, PKCS8_DIVIDER) {
// 		divOffset = SEED_OFFSET + SEED_LENGTH
// 		secretKey = decrypted[SEED_OFFSET:divOffset]
// 		divider = decrypted[divOffset : divOffset+len(PKCS8_DIVIDER)]
// 		if !bytes.Equal(divider, PKCS8_DIVIDER) {
// 			err = errors.New("invalid Pkcs8 divider found in body")
// 			return
// 		}
// 	}
// 	pubOffset := divOffset + len(PKCS8_DIVIDER)
// 	publicKey = decrypted[pubOffset : pubOffset+PUB_LENGTH]
// 	return
// }
//
// func jsonDecryptData(encrypted []byte, passphrase string) (encoded []byte, err error) {
// 	var res Result
// 	res, err = scryptFromBytes(encrypted)
// 	if err != nil {
// 		return
// 	}
// 	var password []byte
// 	if res.salt != nil {
// 		password, err := scrypt.Key([]byte(passphrase), res.salt, scryptN, scryptR, scryptP, scryptDKLen)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
//
// 	return PKCS8_DIVIDER
// }
//
// type Params struct {
// 	N int
// 	P int
// 	r int
// }
// type Result struct {
// 	params Params
// 	salt   []byte
// }
//
// const (
// 	scryptN     = 1 << 15
// 	scryptP     = 1
// 	scryptR     = 8
// 	scryptDKLen = 32
// )
//
// func scryptFromBytes(data []byte) (res Result, err error) {
// 	salt := data[:32]
// 	var N, P, r int
// 	_ = binary.Read(bytes.NewBuffer(data[32+0:32+4]), binary.LittleEndian, &N)
// 	_ = binary.Read(bytes.NewBuffer(data[32+4:32+8]), binary.LittleEndian, &P)
// 	_ = binary.Read(bytes.NewBuffer(data[32+8:32+12]), binary.LittleEndian, &r)
// 	// FIXME At this moment we assume these to be fixed params, this is not a great idea since we lose flexibility
// 	// and updates for greater security. However we need some protection against carefully-crafted params that can
// 	// eat up CPU since these are user inputs. So we need to get very clever here, but atm we only allow the defaults
// 	// and if no match, bail out
// 	if N != scryptN || P != scryptP || r != scryptR {
// 		err = errors.New("invalid injected scrypt params found")
// 		return
// 	}
// 	res = Result{
// 		params: Params{
// 			N: N,
// 			P: P,
// 			r: r,
// 		},
// 		salt: salt,
// 	}
// 	return
// }
//
// var (
// 	ENCODING      = []string{"scrypt", "xsalsa20-poly1305"}
// 	ENCODING_NONE = []string{"none"}
// )
//
// const (
// 	ENCODING_VERSION = "3"
// 	NONCE_LENGTH     = 24
// 	SCRYPT_LENGTH    = 32 + (3 * 4)
// )
