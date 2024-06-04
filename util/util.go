package util

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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
// CIDv1 represented by the CAR file should exactly match the CIDv1 from CalculateFileCid
// or IPFS kubo every time.
//
// This function loads the whole file into memory. Watch out!
// https://github.com/starlinglab/integrity-v2/issues/17
func GetCAR(r io.Reader) (*car.CarV1, error) {
	b := car.NewBuilder()
	return b.Buildv1(context.Background(), r, car.ImportOpts.CIDv1())
}

// CidPath takes a CID and returns the absolute path on the filesystem for where
// that CID is stored.
func CidPath(cid string) (string, error) {
	fileDir := config.GetConfig().Dirs.Files
	c2paDir := config.GetConfig().Dirs.C2PA
	path := filepath.Join(fileDir, cid)
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		path = filepath.Join(c2paDir, cid)
		_, err = os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("file not found in files or c2pa dirs: %s", cid)
		}
		if err != nil {
			return "", err
		}
	}
	if err != nil {
		return "", err
	}
	return path, nil
}

// GuessMediaType guesses the media type of a file based on its contents.
// The 'media_type' should be preferred over this method.
func GuessMediaType(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("error opening CID file: %w", err)
	}
	defer f.Close()
	header := make([]byte, 512)
	_, err = io.ReadFull(f, header)
	if err != nil {
		return "", fmt.Errorf("error reading CID file: %w", err)
	}
	return http.DetectContentType(header), nil
}
