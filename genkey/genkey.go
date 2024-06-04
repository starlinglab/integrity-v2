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
		(len(args) == 1 && args[0] != "aa-enc" && args[0] != "aa-sig" && args[0] != "file") {
		return fmt.Errorf(`Valid invocations:
$ genkey aa-enc
$ genkey aa-sig
$ genkey file`)
	}

	conf := config.GetConfig()

	if args[0] == "aa-sig" {
		fmt.Println(`Currently not implemented. Instead run: openssl genpkey -algorithm ED25519`)
		return nil
	}

	fmt.Print("CID: ")
	var cid string
	_, err := fmt.Scan(&cid)
	if err != nil {
		return err
	}

	var attr string
	if args[0] == "aa-enc" {
		fmt.Print("Attribute: ")
		_, err = fmt.Scan(&attr)
		if err != nil {
			return err
		}
	}

	var path string
	if args[0] == "aa-enc" {
		path = filepath.Join(conf.Dirs.EncKeys, fmt.Sprintf("%s_%s.key", cid, attr))
	} else if args[0] == "file" {
		path = filepath.Join(conf.Dirs.EncKeys, fmt.Sprintf("%s.key", cid))
	}

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
