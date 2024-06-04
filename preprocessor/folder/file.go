package folder

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/webhook"
)

// getFileMetadata calculates and returns a map of attributes for a file
func getFileMetadata(filePath string) (map[string]any, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil {
		return nil, err
	}
	mediaType := http.DetectContentType(buffer[:n])
	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"media_type":    mediaType,
		"file_name":     fileInfo.Name(),
		"last_modified": fileInfo.ModTime().UTC().Format(time.RFC3339),
		"time_created":  fileInfo.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// handleNewFile posts a new file and its metadata to the webhook server,
// and returns the CID of the file according to the server.
func handleNewFile(filePath string) (string, error) {
	metadata, err := getFileMetadata(filePath)
	if err != nil {
		return "", fmt.Errorf("error getting metadata for file %s: %v", filePath, err)
	}
	resp, err := webhook.PostFileToWebHook(filePath, metadata, webhook.PostGenericWebhookOpt{})
	if err != nil {
		return "", fmt.Errorf("error posting metadata for file %s: %v", filePath, err)
	}
	return resp.Cid, nil
}

// shouldIncludeFile reports whether the file should be included in the processing
func shouldIncludeFile(fileName string) bool {
	whiteListExtension := config.GetConfig().FolderPreprocessor.FileExtensions
	if fileName[0] == '.' {
		return false
	}
	fileExt := filepath.Ext(fileName)
	if fileExt == ".partial" {
		return false
	}
	if slices.Contains(whiteListExtension, fileExt) {
		return true
	}
	return false
}
