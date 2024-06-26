package encrypt

import (
	"crypto/rand"
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
	if len(args) != 1 ||
		(len(args) > 0 && (args[0] == "--help" || args[0] == "help" || args[0] == "-h")) {
		return fmt.Errorf("just pass a single CID to encrypt")
	}
	cid := args[0]

	conf := config.GetConfig()

	cidPath := filepath.Join(conf.Dirs.Files, cid)
	_, err := os.Stat(cidPath)
	if err != nil {
		return fmt.Errorf("error finding CID file: %w", err)
	}

	key := make([]byte, 32)
	_, err = rand.Read(key)
	if err != nil {
		return fmt.Errorf("error reading random data for key: %w", err)
	}

	inF, err := os.Open(cidPath)
	if err != nil {
		return fmt.Errorf("error opening CID file: %w", err)
	}
	defer inF.Close()

	fi, err := inF.Stat()
	if err != nil {
		return fmt.Errorf("error statting CID file: %w", err)
	}
	inFileSize := fi.Size()

	tmpF, err := os.CreateTemp(util.TempDir(), "encrypt_")
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
	var bytesRead int64
	var n int
	for {
		n, err = inF.Read(buf)
		if err == io.EOF {
			return fmt.Errorf("assertion error: unexpected end of file")
		}
		if err != nil {
			return fmt.Errorf("error reading CID file: %w", err)
		}

		bytesRead += int64(n)
		if bytesRead == inFileSize {
			// Whole file has been read, the next .Read would return (0, io.EOF)
			break
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
	// The last message in the stream, the last chunk of the file
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
	_, err = tmpF.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("error seeking in temp file: %w", err)
	}
	encCid, err := util.CalculateFileCid(tmpF)
	if err != nil {
		return fmt.Errorf("error calculating output CID: %w", err)
	}

	// Write key and output file

	keyPath := filepath.Join(conf.Dirs.EncKeys, encCid+".key")
	err = os.WriteFile(keyPath, key, 0600)
	if err != nil {
		return fmt.Errorf("error saving key: %w", err)
	}
	fmt.Printf("Saved encryption key to %s\n", keyPath)

	outPath := filepath.Join(conf.Dirs.Files, encCid)
	err = util.MoveFile(tmpF.Name(), outPath)
	if err != nil {
		return fmt.Errorf("error moving file: %w", err)
	}
	fmt.Printf("Saved encrypted file to %s\n", outPath)

	// Log to AA
	err = aa.AddRelationship(cid, "children", "encrypted", encCid)
	if err != nil {
		return fmt.Errorf("error adding encryption relationship to AuthAttr: %w", err)
	}
	err = aa.SetAttestations(encCid, false, []aa.PostKV{{Key: "encryption_type", Value: "secretstream"}})
	if err != nil {
		return fmt.Errorf("error adding encryption metadata to AuthAttr: %w", err)
	}

	fmt.Println("Done.")
	return nil
}
