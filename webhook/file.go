package webhook

import (
	"fmt"
	"os"

	"github.com/starlinglab/integrity-v2/config"
)

// Check if the output directory is set and exists
func getFileOutputDirectory() (string, error) {
	outputDirectory := config.GetConfig().Dirs.Files
	if outputDirectory == "" {
		fmt.Println("Error: Output directory not set")
		return "", fmt.Errorf("Output directory not set")
	}
	_, err := os.Stat(outputDirectory)
	if err != nil {
		fmt.Println("Error: Output directory not set")
		return "", err
	}
	return outputDirectory, nil
}
