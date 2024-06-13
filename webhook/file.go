package webhook

import (
	"fmt"
	"log"
	"os"

	"github.com/starlinglab/integrity-v2/config"
)

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
