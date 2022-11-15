package subkey

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FavorLabs/favorX/pkg/crypto"
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

func (s *Service) Key(name, password string) (keypair crypto.Signer, created bool, err error) {
	filename := s.keyFilename(name)

	data, err := os.ReadFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("read private key: %w", err)
	}
	if len(data) == 0 {
		mnemonic, _ := crypto.NewBIP39Mnemonic()
		keypair, err = crypto.NewKeypairFromMnemonic(mnemonic)
		if err != nil {
			return nil, false, fmt.Errorf("generate sr25519 key: %w", err)
		}

		d, err := encryptKey(keypair, password)
		if err != nil {
			return nil, false, err
		}

		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return nil, false, err
		}
		if err := os.WriteFile(filename, d, 0600); err != nil {
			return nil, false, err
		}
		return keypair, true, nil
	}

	keypair, err = decryptKey(data, password)
	if err != nil {
		keypair, err = decryptKeyPair(data, password)
		if err != nil {
			return nil, false, err
		}
	}
	return keypair, false, nil
}

func (s *Service) ExportKey(name, password string) ([]byte, error) {
	pk, err := s.read(name, password)
	if err != nil {
		return nil, err
	}
	return encryptKeyPair(pk, password)
}

func (s *Service) ImportKey(name, password, passwordNow string, keyJson []byte) (err error) {
	if passwordNow == "" {
		passwordNow = password
	}
	_, err = s.read(name, password)
	if err != nil {
		return err
	}
	bakFile, err := s.bak(name)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.restore(name, bakFile)
		}
	}()

	pk, err := decryptKeyPair(keyJson, passwordNow)
	if err != nil {
		return err
	}
	d, err := encryptKeyPair(pk, passwordNow)
	if err != nil {
		return err
	}

	return s.write(name, d)
}

func (s *Service) ImportPrivateKey(name, password, passwordNow string, mnemonicOrPrivateData string) (err error) {
	if passwordNow == "" {
		passwordNow = password
	}
	_, err = s.read(name, password)
	if err != nil {
		return err
	}

	var keypair crypto.Signer
	if strings.Contains(mnemonicOrPrivateData, " ") {
		keypair, err = crypto.NewKeypairFromMnemonic(mnemonicOrPrivateData)
	} else {
		keypair, err = crypto.NewKeypairFromSeedHex(mnemonicOrPrivateData)

	}
	if err != nil {
		return err
	}

	d, err := encryptKey(keypair, passwordNow)
	if err != nil {
		return err
	}

	bakFile, err := s.bak(name)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.restore(name, bakFile)
		}
	}()
	return s.write(name, d)
}

func (s *Service) keyFilename(name string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s.key", name))
}

func (s *Service) read(name, password string) (kp crypto.Signer, err error) {
	filename := s.keyFilename(name)

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read private key failed")
	}
	kp, err = decryptKey(data, password)
	if err != nil {
		return decryptKeyPair(data, password)
	}
	return
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

func (s *Service) bak(name string) (bakFile string, err error) {
	filename := s.keyFilename(name)
	bakFile = filename + fmt.Sprintf(".bak.%d", time.Now().Unix())
	err = os.Rename(filename, bakFile)
	return
}

func (s *Service) restore(name, bakFile string) error {
	filename := s.keyFilename(name)
	return os.Rename(bakFile, filename)
}
