package webhook

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
)

func CopyOutputToFilePath(src io.Reader, originalFileName string, cid string) error {
	outputDirectory := config.GetConfig().Dirs.Files
	if outputDirectory == "" {
		return fmt.Errorf("output directory not set")
	}
	_, err := os.Stat(outputDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("output directory %s does not exist", outputDirectory)
		}
		return err
	}
	path := filepath.Join(outputDirectory, cid)
	fd, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = io.Copy(fd, src)
	return err
}
