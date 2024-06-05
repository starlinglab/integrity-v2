package encrypt

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/openziti/secretstream"
	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

func Run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("just pass a single CID to encrypt")
	}
	cid := args[0]

	conf := config.GetConfig()

	cidPath := filepath.Join(conf.Dirs.Files, cid)
	_, err := os.Stat(cidPath)
	if err != nil {
		return fmt.Errorf("error finding CID file: %w", err)
	}

	keyPath := filepath.Join(conf.Dirs.EncKeys, cid+".key")
	var key []byte
	_, err = os.Stat(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("Generating key as no existing one was found...")
		key = make([]byte, 32)
		_, err = rand.Read(key)
		if err != nil {
			return fmt.Errorf("error reading random data for key: %w", err)
		}
		err = os.WriteFile(keyPath, key, 0600)
		if err != nil {
			return fmt.Errorf("error saving new key: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("error inspecting key file: %w", err)
	} else {
		fmt.Printf("Using key found at %s\n", keyPath)
		key, err = os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("error reading key file: %w", err)
		}
	}

	inF, err := os.Open(cidPath)
	if err != nil {
		return fmt.Errorf("error opening CID file: %w", err)
	}
	defer inF.Close()

	tmpF, err := os.CreateTemp("", "encrypt_")
	if err != nil {
		return fmt.Errorf("error creating temp file: %w", err)
	}
	defer tmpF.Close()
	defer os.Remove(tmpF.Name())

	fmt.Println("Encrypting...")
	enc, header, err := secretstream.NewEncryptor(key)
	if err != nil {
		return fmt.Errorf("error starting encryption: %w", err)
	}

	_, err = tmpF.Write(header)
	if err != nil {
		return fmt.Errorf("error writing to temp file: %w", err)
	}

	buf := make([]byte, 32768) // 32 KiB, same as io.Copy
	var n int
	for {
		n, err = inF.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading CID file: %w", err)
		}

		cipher, err := enc.Push(buf[:n], secretstream.TagMessage)
		if err != nil {
			return fmt.Errorf("error encrypting data: %w", err)
		}
		_, err = tmpF.Write(cipher)
		if err != nil {
			return fmt.Errorf("error writing to temp file: %w", err)
		}
	}
	// EOF
	// Due to the way (*os.File).Read works, n is always 0 in this case.
	// But from my testing it's fine for the last message in the stream to be empty.
	// So we do that.
	cipher, err := enc.Push(buf[:n], secretstream.TagFinal)
	if err != nil {
		return fmt.Errorf("error encrypting data: %w", err)
	}
	_, err = tmpF.Write(cipher)
	if err != nil {
		return fmt.Errorf("error writing to temp file: %w", err)
	}
	fmt.Println("Done. Moving on to cleanup...")

	// Calc CID
	_, err = tmpF.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("error seeking in temp file: %w", err)
	}
	encCid, err := util.CalculateFileCid(tmpF)
	if err != nil {
		return fmt.Errorf("error calculating output CID: %w", err)
	}

	outPath := filepath.Join(conf.Dirs.Files, encCid)

	err = util.MoveFile(tmpF.Name(), outPath)
	if err != nil {
		return fmt.Errorf("error moving file: %w", err)
	}

	err = aa.AddRelationship(cid, "children", "encrypted", encCid)
	if err != nil {
		return fmt.Errorf("error adding encryption relationship to AuthAttr: %w", err)
	}
	err = aa.SetAttestations(encCid, false, []aa.PostKV{{Key: "encryption_type", Value: "secretstream"}})
	if err != nil {
		return fmt.Errorf("error adding encryption metadata to AuthAttr: %w", err)
	}

	fmt.Printf("Done.\nEncrypted file is stored at %s\n", outPath)
	return nil
}
