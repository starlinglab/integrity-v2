package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	car "github.com/photon-storage/go-ipfs-car"
	"github.com/starlinglab/integrity-v2/config"
)

// Fatal kills the program if the provided err is not nil, logging it as well.
func Fatal(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// GetCID returns the CIDv1 string for the bytes it reads.
func GetCID(r io.Reader) (string, error) {
	ipfsArgs := []string{
		"add",
		// "many of these settings are the default, but for the purposes of being clear in case
		// the default ever change, we want to specify them explicitly."
		// -- https://github.com/starlinglab/authenticated-attributes/issues/1
		"--only-hash=true",
		"--wrap-with-directory=false",
		"--cid-version=1",
		"--hash=sha2-256",
		"--pin=true",
		"--raw-leaves=true",
		"--chunker=size-262144",
		"--nocopy=false",
		"--fscache=false",
		"--inline=false",
		"--inline-limit=32",
		"--quieter",
		"-",
	}
	ipfs := config.GetConfig().Bins.Ipfs
	cmd := exec.Command(ipfs, ipfsArgs...)
	cmd.Stdin = r
	cid, err := cmd.Output()
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("ipfs not found at configured path, may not be installed: %s", ipfs)
	}
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(cid)), nil
}

// MoveFile moves the provided file, even if source and dest are part of different file systems.
//
// It is not atomic. os.Rename should be used in favour of this function if it can be guaranteed
// that source and dest are on the same filesystem, or atomicity is required.
func MoveFile(sourcePath, destPath string) error {
	// Adapted from https://stackoverflow.com/a/50741908

	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %v", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("couldn't open dest file: %v", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return fmt.Errorf("couldn't copy to dest from source: %v", err)
	}

	inputFile.Close() // for Windows, close before trying to remove: https://stackoverflow.com/a/64943554/246801

	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("couldn't remove source file: %v", err)
	}
	return nil
}

// GetCAR returns a CARv1 file created from the provided reader.
// Currently this uses the default IPFS kubo settings under the hood, and so the
// CIDv1 represented by the CAR file should exactly match the CIDv1 from GetCid
// or IPFS kubo every time.
//
// I think this function will load the whole file into memory, so watch out for that.
func GetCAR(r io.Reader) (*car.CarV1, error) {
	b := car.NewBuilder()
	return b.Buildv1(context.Background(), r, car.ImportOpts.CIDv1())
}
