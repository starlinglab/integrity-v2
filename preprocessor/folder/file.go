package preprocessor_folder

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/webhook"
	"lukechampine.com/blake3"
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

	sha := sha256.New()
	md := md5.New()
	blake := blake3.New(32, nil)

	tee := io.TeeReader(file, sha)
	tee = io.TeeReader(tee, md)
	tee = io.TeeReader(tee, blake)

	bytes, err := io.ReadAll(tee)
	if err != nil {
		return nil, err
	}
	mediaType := http.DetectContentType(bytes)

	return map[string]any{
		"sha256":        hex.EncodeToString(sha.Sum(nil)),
		"md5":           hex.EncodeToString(md.Sum(nil)),
		"blake3":        hex.EncodeToString(blake.Sum(nil)),
		"media_type":    mediaType,
		"file_size":     fileInfo.Size(),
		"file_name":     fileInfo.Name(),
		"last_modified": fileInfo.ModTime().Format(time.RFC3339),
		"time_created":  fileInfo.ModTime().Format(time.RFC3339),
	}, nil
}

// handleNewFile posts a new file and its metadata to the webhook server
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

// checkShouldIncludeFile reports whether the file should be included in the processing
func checkShouldIncludeFile(info fs.FileInfo) bool {
	whiteListExtension := config.GetConfig().FolderPreprocessor.FileExtensions
	var ignoreFileNamePrefix byte = '.'
	ignoreFileSuffix := ".partial"
	fileName := info.Name()
	if fileName[0] == ignoreFileNamePrefix {
		return false
	}
	fileExt := filepath.Ext(fileName)
	if fileExt == ignoreFileSuffix {
		return false
	}
	if slices.Contains(whiteListExtension, fileExt) {
		return true
	}
	return false
}
