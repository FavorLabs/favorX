package p2pkey

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	crypto2 "github.com/libp2p/go-libp2p/core/crypto"
)

// Service is the file-based keystore.Service implementation.
//
// Keys are stored in directory where each private key is stored in a file,
// which is encrypted with symmetric key using some password.
type Service struct {
	dir string
}

// New creates new file-based keystore.Service implementation.
func New(dir string) *Service {
	return &Service{dir: dir}
}

func (s *Service) Exists(name string) (bool, error) {
	filename := s.keyFilename(name)

	data, err := os.ReadFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read private key: %w", err)
	}
	if len(data) == 0 {
		return false, nil
	}

	return true, nil
}

func (s *Service) Key(name, password string) (sk crypto2.PrivKey, created bool, err error) {
	filename := s.keyFilename(name)

	data, err := os.ReadFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("read private key: %w", err)
	}
	if len(data) > 0 {
		sk, err = decryptKey(data, password)
		if err != nil {
			goto CREATE
		}
		return sk, false, nil
	}
CREATE:
	sk, err = s.generate(name, password)
	if err != nil {
		return nil, false, err
	}
	return sk, true, nil
}

func (s *Service) keyFilename(name string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s.key", name))
}

func (s *Service) write(name string, data []byte) (err error) {
	filename := s.keyFilename(name)
	if err = os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filename, data, 0600); err != nil {
		return err
	}
	return nil
}

func (s *Service) generate(name, password string) (crypto2.PrivKey, error) {
	sk, _, err := crypto2.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	d, err := encryptKey(sk, password)
	if err != nil {
		return nil, err
	}

	err = s.write(name, d)
	if err != nil {
		return nil, err
	}
	return sk, nil
}
