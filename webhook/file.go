package webhook

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
	"lukechampine.com/blake3"
)

func getFileAttributesAndWriteToDest(source io.Reader, destFile *os.File) (cid string, fileAttributes map[string]any, err error) {
	pr, pw := io.Pipe()
	cidChan := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		cid, err := util.CalculateFileCid(pr)
		cidChan <- cid
		errChan <- err
	}()

	sha := sha256.New()
	md := md5.New()
	blake := blake3.New(32, nil)

	fileWriter := io.MultiWriter(destFile, pw, sha, md, blake)

	_, err = io.Copy(fileWriter, source)
	if err != nil {
		return "", nil, err
	}
	err = pw.Close()
	if err != nil {
		return "", nil, err
	}
	cid = <-cidChan
	err = <-errChan
	if err != nil {
		return "", nil, err
	}
	fileState, err := destFile.Stat()
	if err != nil {
		return "", nil, err
	}
	fileAttributes = map[string]any{
		"sha256":    hex.EncodeToString(sha.Sum(nil)),
		"md5":       hex.EncodeToString(md.Sum(nil)),
		"blake3":    hex.EncodeToString(blake.Sum(nil)),
		"file_size": fileState.Size(),
	}
	return cid, fileAttributes, nil
}

// Check if the output directory is set and exists
func getFileOutputDirectory() (string, error) {
	outputDirectory := config.GetConfig().Dirs.Files
	if outputDirectory == "" {
		log.Println("error: output directory not set")
		return "", fmt.Errorf("output directory not set")
	}
	_, err := os.Stat(outputDirectory)
	if err != nil {
		log.Println("error: output directory not set")
		return "", err
	}
	return outputDirectory, nil
}
