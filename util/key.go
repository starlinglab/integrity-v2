package util

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
)

const (
	secretboxKeySize = 32
)

// GenerateEncKey generates a new encryption key for the given CID and attribute
// and stores it in a file. If the key already exists, it is read from the file.
func GenerateEncKey(cid, attr string) (encKeyPath string, encKeyBytes []byte, isNew bool, err error) {
	conf := config.GetConfig()
	encKeyPath = filepath.Join(conf.Dirs.EncKeys, fmt.Sprintf("%s_%s.key", cid, attr))
	f, err := os.OpenFile(encKeyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			encKeyBytes, err := os.ReadFile(encKeyPath)
			if err != nil {
				return "", nil, false, fmt.Errorf("error reading key file: %w", err)
			}
			if len(encKeyBytes) != secretboxKeySize {
				return "", nil, false, fmt.Errorf("key file is not the correct size")
			}
			return encKeyPath, encKeyBytes, false, nil
		}
		return "", nil, false, fmt.Errorf("error creating key file: %w", err)
	}
	defer f.Close()
	encKeyBytes = make([]byte, secretboxKeySize)
	_, err = rand.Read(encKeyBytes)
	if err != nil {
		return "", nil, false, fmt.Errorf("error generating random bytes: %w", err)
	}

	_, err = f.Write(encKeyBytes)
	if err != nil {
		// Cleanup
		f.Close()
		os.Remove(encKeyPath)
		return "", nil, false, fmt.Errorf("error writing key file: %w", err)
	}
	return encKeyPath, encKeyBytes, true, nil
}
