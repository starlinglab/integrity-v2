package folder

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/webhook"
)

// File status constants
var (
	FileStatusFound     = "Found"
	FileStatusUploading = "Uploading"
	FileStatusSuccess   = "Success"
	FileStatusError     = "Error"
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

// handleNewFile takes a discovered file, update file status on database,
// posts the new file and its metadata to the webhook server,
// and returns the CID of the file according to the server.
func handleNewFile(pgPool *pgxpool.Pool, filePath string) (string, error) {
	result, err := queryIfFileExists(pgPool, filePath)
	if err != nil {
		return "", fmt.Errorf("error checking if file exists in database: %v", err)
	}
	status, errorMessage, cid := "", "", ""
	if result != nil {
		if result.Status != nil {
			status = *result.Status
		}
		if result.ErrorMessage != nil {
			errorMessage = *result.ErrorMessage
		}
		if result.Cid != nil {
			cid = *result.Cid
		}
	}
	switch status {
	case FileStatusFound:
		return "", fmt.Errorf("file %s is already found", filePath)
	case FileStatusUploading:
		return "", fmt.Errorf("file %s is already uploading", filePath)
	case FileStatusSuccess:
		return cid, nil
	case FileStatusError:
		return "", fmt.Errorf("file %s has error: %s", filePath, errorMessage)
	default:
		err = setFileStatusFound(pgPool, filePath)
		if err != nil {
			return "", fmt.Errorf("error setting file status to found: %v", err)
		}
	}
	metadata, err := getFileMetadata(filePath)
	if err != nil {
		e := setFileStatusError(pgPool, filePath, err.Error())
		if e != nil {
			fmt.Println("error setting file status to error:", e)
		}
		return "", fmt.Errorf("error getting metadata for file %s: %v", filePath, err)
	}
	err = setFileStatusUploading(pgPool, filePath, metadata["sha256"].(string))
	if err != nil {
		return "", fmt.Errorf("error setting file status to uploading: %v", err)
	}
	resp, err := webhook.PostFileToWebHook(filePath, metadata, webhook.PostGenericWebhookOpt{})
	if err != nil {
		e := setFileStatusError(pgPool, filePath, err.Error())
		if e != nil {
			fmt.Println("error setting file status to error:", e)
		}
		return "", fmt.Errorf("error posting metadata for file %s: %v", filePath, err)
	}
	err = setFileStatusDone(pgPool, filePath, cid)
	if err != nil {
		return "", fmt.Errorf("error setting file status to done: %v", err)
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
