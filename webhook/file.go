package webhook

import (
	"io"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
)

func CopyOutputToFilePath(src io.Reader, originalFileName string, cid string) error {
	outputDirectory := config.GetConfig().Dirs.Files
	if outputDirectory == "" {
		outputDirectory = "./output"
	}
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
