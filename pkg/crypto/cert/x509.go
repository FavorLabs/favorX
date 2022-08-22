package cert

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

const rootName = "rootCA.pem"
const rootKeyName = "rootCA-key.pem"

var userAndHostname string

func init() {
	u, err := user.Current()
	if err == nil {
		userAndHostname = u.Username + "@"
	}
	if h, err := os.Hostname(); err == nil {
		userAndHostname += h
	}
	if err == nil && u.Name != "" && u.Name != u.Username {
		userAndHostname += " (" + u.Name + ")"
	}
}

type Cert struct {
	KeyFile, CertFile, P12File string
	dir                        string
	caCert                     *x509.Certificate
	caKey                      crypto.PrivateKey
}

func New(dir string) *Cert {
	dir += "/cert"
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.Mkdir(dir, 0755)
		}
	}
	return &Cert{
		dir:      dir,
		KeyFile:  filepath.Join(dir, "localhost-key.pem"),
		CertFile: filepath.Join(dir, "localhost.pem"),
		P12File:  filepath.Join(dir, "localhost.p12"),
	}
}

func (m *Cert) MakeCert() *Cert {
	hosts := []string{
		"localhost",
		"127.0.0.1",
		"::1",
	}
	if m.caKey == nil {
		return m
	}

	priv, _ := generateKey(false)
	pub := priv.(crypto.Signer).Public()

	// Certificates last for 2 years and 3 months, which is always less than
	// 825 days, the limit that macOS/iOS apply to all certificates,
	// including custom roots. See https://support.apple.com/en-us/HT210176.
	expiration := time.Now().AddDate(2, 3, 0)

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{"favorX node certificate"},
			OrganizationalUnit: []string{userAndHostname},
		},

		NotBefore: time.Now(), NotAfter: expiration,

		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else if email, err := mail.ParseAddress(h); err == nil && email.Address == h {
			tpl.EmailAddresses = append(tpl.EmailAddresses, h)
		} else if uriName, err := url.Parse(h); err == nil && uriName.Scheme != "" && uriName.Host != "" {
			tpl.URIs = append(tpl.URIs, uriName)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}

	if len(tpl.IPAddresses) > 0 || len(tpl.DNSNames) > 0 || len(tpl.URIs) > 0 {
		tpl.ExtKeyUsage = append(tpl.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	}
	if len(tpl.EmailAddresses) > 0 {
		tpl.ExtKeyUsage = append(tpl.ExtKeyUsage, x509.ExtKeyUsageEmailProtection)
	}

	crt, _ := x509.CreateCertificate(rand.Reader, tpl, m.caCert, pub, m.caKey)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: crt})
	privDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	_ = os.WriteFile(m.CertFile, certPEM, 0644)
	_ = os.WriteFile(m.KeyFile, privPEM, 0600)
	return m
}

func (m *Cert) LoadCA() *Cert {
	rootFile := filepath.Join(m.dir, rootName)
	rootKeyFile := filepath.Join(m.dir, rootKeyName)
	_, err1 := os.Stat(rootFile)
	_, err2 := os.Stat(rootKeyFile)
	if err1 != nil || err2 != nil {
		newCA(m.dir)
	}
	certPEMBlock, err := os.ReadFile(rootFile)
	if err != nil {
		return m
	}
	certDERBlock, _ := pem.Decode(certPEMBlock)
	if certDERBlock == nil || certDERBlock.Type != "CERTIFICATE" {
		return m
	}
	m.caCert, _ = x509.ParseCertificate(certDERBlock.Bytes)

	keyPEMBlock, _ := os.ReadFile(rootKeyFile)
	keyDERBlock, _ := pem.Decode(keyPEMBlock)
	if keyDERBlock == nil || keyDERBlock.Type != "PRIVATE KEY" {
		return m
	}
	m.caKey, _ = x509.ParsePKCS8PrivateKey(keyDERBlock.Bytes)

	return m
}

func newCA(dir string) {
	caKey, _ := generateKey(true)
	pub := caKey.(crypto.Signer).Public()

	spkiASN1, _ := x509.MarshalPKIXPublicKey(pub)

	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	_, _ = asn1.Unmarshal(spkiASN1, &spki)

	skid := sha1.Sum(spki.SubjectPublicKey.Bytes)

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{"favorX node CA"},
			OrganizationalUnit: []string{userAndHostname},

			// The CommonName is required by iOS to show the certificate in the
			// "Certificate Trust Settings" menu.
			CommonName: "favorX " + userAndHostname,
		},
		SubjectKeyId: skid[:],

		NotAfter:  time.Now().AddDate(10, 0, 0),
		NotBefore: time.Now(),

		KeyUsage: x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	caCert, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, caKey)

	privDER, _ := x509.MarshalPKCS8PrivateKey(caKey)
	_ = os.WriteFile(filepath.Join(dir, rootKeyName), pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0400)

	_ = os.WriteFile(filepath.Join(dir, rootName), pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: caCert}), 0644)
}

func generateKey(rootCA bool) (crypto.PrivateKey, error) {
	if rootCA {
		return rsa.GenerateKey(rand.Reader, 3072)
	}
	return rsa.GenerateKey(rand.Reader, 2048)
}

func randomSerialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)
	return serialNumber
}
