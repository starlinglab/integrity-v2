package folder

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	syncRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	syncRoot = filepath.Clean(syncRoot)
	assetOrigin := filepath.Clean(strings.TrimPrefix(filePath, syncRoot))

	return map[string]any{
		"media_type":    mediaType,
		"file_name":     fileInfo.Name(),
		"last_modified": fileInfo.ModTime().UTC().Format(time.RFC3339),
		"time_created":  fileInfo.ModTime().UTC().Format(time.RFC3339),
		"asset_origin":  assetOrigin,
	}, nil
}

// handleNewFile takes a discovered file, update file status on database,
// posts the new file and its metadata to the webhook server,
// and returns the CID of the file according to the server.
func handleNewFile(pgPool *pgxpool.Pool, filePath string, project *ProjectQueryResult) (string, error) {
	if len(project.FileExtensions) > 0 {
		fileExt := filepath.Ext(filePath)
		if !slices.Contains(project.FileExtensions, fileExt) {
			return "", nil
		}
	}
	log.Println("found: " + filePath)
	result, err := queryAndSetFoundFileStatus(pgPool, filePath)
	if err != nil {
		return "", fmt.Errorf("error checking if file exists in database: %v", err)
	}

	status, errorMessage, cid := "", "", ""
	if result != nil {
		status = result.Status
		errorMessage = result.ErrorMessage
		cid = result.Cid
	}

	switch status {
	case FileStatusUploading:
		log.Println("retrying uploading file:", filePath)
	case FileStatusSuccess:
		return cid, nil
	case FileStatusError:
		return "", fmt.Errorf("file %s has error: %s", filePath, errorMessage)
	case FileStatusFound:
	default:
		// proceed to upload
	}

	metadata, err := getFileMetadata(filePath)
	if err != nil {
		if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
			log.Println("error setting file status to error:", err)
		}
		return "", fmt.Errorf("error getting metadata for file %s: %v", filePath, err)
	}

	if project != nil {
		metadata["project_id"] = project.ProjectId
		metadata["project_path"] = filepath.Clean(project.ProjectPath)
		if project.AuthorType != "" || project.AuthorName != "" || project.AuthorIdentifier != "" {
			author := map[string]string{}
			if project.AuthorType != "" {
				author["@type"] = project.AuthorType
			}
			if project.AuthorName != "" {
				author["name"] = project.AuthorName
			}
			if project.AuthorIdentifier != "" {
				author["identifier"] = project.AuthorIdentifier
			}
			metadata["author"] = author
		}
	}

	err = setFileStatusUploading(pgPool, filePath)
	if err != nil {
		return "", fmt.Errorf("error setting file status to uploading: %v", err)
	}
	resp, err := webhook.PostFileToWebHook(filePath, metadata, webhook.PostGenericWebhookOpt{})
	if err != nil {
		if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
			log.Println("error setting file status to error:", err)
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
	if fileName[0] == '.' {
		return false
	}
	fileExt := filepath.Ext(fileName)
	return fileExt != ".partial"
}
