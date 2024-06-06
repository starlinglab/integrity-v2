package genkey

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
)

const (
	secretboxKeySize = 32
)

func Run(args []string) error {
	if len(args) != 1 ||
		(len(args) == 1 && args[0] != "aa-enc" && args[0] != "aa-sig") {
		return fmt.Errorf(`Valid invocations:
$ genkey aa-enc
$ genkey aa-sig`)
	}

	conf := config.GetConfig()

	if args[0] == "aa-sig" {
		fmt.Println(`Currently not implemented. Instead run: openssl genpkey -algorithm ED25519`)
		return nil
	}
	// "aa-enc"

	fmt.Print("CID: ")
	var cid string
	_, err := fmt.Scan(&cid)
	if err != nil {
		return err
	}

	var attr string
	fmt.Print("Attribute: ")
	_, err = fmt.Scan(&attr)
	if err != nil {
		return err
	}

	path := filepath.Join(conf.Dirs.EncKeys, fmt.Sprintf("%s_%s.key", cid, attr))

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("key already exists!: %w", err)
		}
		return fmt.Errorf("error creating key file: %w", err)
	}
	defer f.Close()

	_, err = io.CopyN(f, rand.Reader, secretboxKeySize)
	if err != nil {
		// Cleanup
		f.Close()
		os.Remove(path)
		return fmt.Errorf("failed to write key: %w", err)
	}

	fmt.Printf("Generated key was stored at %s\n", path)

	return nil
}
