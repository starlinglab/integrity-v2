package webhook

import (
	"io"
	"os"
	"path/filepath"
)

var outputDirectory = os.Getenv("FILE_OUTPUT_PATH")

func CopyOutputToFilePath(src io.Reader, originalFileName string, cid string) error {
	ext := filepath.Ext(originalFileName)
	path := filepath.Join(outputDirectory, cid)
	if ext != "" {
		path += ext
	}
	fd, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(fd, src)
	return err
}
