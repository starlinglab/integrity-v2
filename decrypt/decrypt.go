package decrypt

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/openziti/secretstream"
)

var (
	inPath  string
	keyPath string
	outPath string
)

func Run(args []string) error {
	fs := flag.NewFlagSet("decrypt", flag.ContinueOnError)
	fs.StringVar(&inPath, "i", "", "path to encrypted input file")
	fs.StringVar(&keyPath, "k", "", "path to decryption key file (32 bytes)")
	fs.StringVar(&outPath, "o", "", "path where decrypted file should be written")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	if inPath == "" || keyPath == "" || outPath == "" {
		return fmt.Errorf("all flags must be specified, see --help")
	}

	fi, err := os.Stat(keyPath)
	if err != nil {
		return err
	}
	if fi.Size() != 32 {
		return fmt.Errorf("expected key file to be 32 bytes, instead was %d", fi.Size())
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	inF, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("error opening input file: %w", err)
	}
	defer inF.Close()

	fi, err = inF.Stat()
	if err != nil {
		return fmt.Errorf("error statting CID file: %w", err)
	}
	inFileSize := fi.Size()

	outF, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("error opening output file: %w", err)
	}
	defer outF.Close()

	header := make([]byte, secretstream.StreamHeaderBytes)
	_, err = io.ReadFull(inF, header)
	if err != nil {
		return fmt.Errorf("error reading from input file: %w", err)
	}

	fmt.Println("Decrypting...")
	dec, err := secretstream.NewDecryptor(key, header)
	if err != nil {
		return fmt.Errorf("error starting decryption: %w", err)
	}

	// File decryption logic is more complex than encryption
	// Need to make sure final tag in stream matches when the file bytes end
	// See example:
	// https://doc.libsodium.org/secret-key_cryptography/secretstream#file-encryption-example-code

	// Input buffer is 32 KiB (like io.Copy) plus the overhead added by the encryption
	// process. This keeps things aligned with encrypt.go, so each read in this loop
	// corresponds to the write made in the encryption loop.
	buf := make([]byte, 32768+secretstream.StreamABytes)
	var bytesRead int64 = secretstream.StreamHeaderBytes // Already read header
	var n int
	for {
		n, err = inF.Read(buf)
		if err == io.EOF {
			return fmt.Errorf("assertion error: unexpected end of file")
		}
		if err != nil {
			os.Remove(outPath)
			return fmt.Errorf("error reading input file: %w", err)
		}

		bytesRead += int64(n)
		if bytesRead == inFileSize {
			// Whole file has been read, the next .Read would return (0, io.EOF)
			break
		}

		plain, tag, err := dec.Pull(buf[:n])
		if err != nil {
			os.Remove(outPath)
			return fmt.Errorf("error decrypting data: %w", err)
		}
		if tag == secretstream.TagFinal {
			// Got "end of stream" but file hasn't reached EOF
			os.Remove(outPath)
			return fmt.Errorf("encrypted stream ended before file did")
		}

		_, err = outF.Write(plain)
		if err != nil {
			os.Remove(outPath)
			return fmt.Errorf("error writing to output file: %w", err)
		}
	}
	// Decrypt last message
	// If it isn't tagged as final then the stream is truncated and shouldn't be accepted.
	plain, tag, err := dec.Pull(buf[:n])
	if err != nil {
		os.Remove(outPath)
		return fmt.Errorf("error decrypting data: %w", err)
	}
	if tag != secretstream.TagFinal {
		os.Remove(outPath)
		return fmt.Errorf("file ended early, encrypted stream truncated")
	}

	_, err = outF.Write(plain)
	if err != nil {
		os.Remove(outPath)
		return fmt.Errorf("error writing to output file: %w", err)
	}

	fmt.Println("Done.")
	return nil
}
