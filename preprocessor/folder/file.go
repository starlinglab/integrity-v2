package folder

import (
	"archive/zip"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starlinglab/integrity-v2/config"
	proofmode "github.com/starlinglab/integrity-v2/preprocessor/proofmode"
	wacz "github.com/starlinglab/integrity-v2/preprocessor/wacz"
	"github.com/starlinglab/integrity-v2/webhook"
)

// File status constants
var (
	FileStatusFound     = "Found"
	FileStatusUploading = "Uploading"
	FileStatusSuccess   = "Success"
	FileStatusError     = "Error"
)

func getAssetOriginRoot(filePath string) string {
	syncRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	syncRoot = filepath.Clean(syncRoot)
	assetOriginRoot := filepath.Clean(strings.TrimPrefix(filePath, syncRoot))
	return assetOriginRoot
}

// getProofModeFileMetadatas reads a proofmode file and returns a list of metadata
func getProofModeFileMetadatas(filePath string) ([]map[string]any, error) {
	assets, err := proofmode.ReadAndVerifyMetadata(filePath)
	if err != nil {
		return nil, err
	}
	metadatas := []map[string]any{}
	for _, asset := range assets {
		fileName := filepath.Base(asset.Metadata.FilePath)
		assetOriginRoot := getAssetOriginRoot(filePath)
		assetOrigin := filepath.Join(assetOriginRoot, asset.Metadata.FilePath)

		metadata := map[string]any{
			"file_name":         fileName,
			"last_modified":     asset.Metadata.FileModified,
			"time_created":      asset.Metadata.FileCreated,
			"asset_origin_id":   assetOrigin,
			"asset_origin_type": []string{"proofmode"},
			"media_type":        asset.MediaType,
			"private": map[string]any{ // "private" fields are encrypted
				"proofmode": map[string]any{
					"metadata":  string(asset.MetadataBytes),
					"meta_sig":  string(asset.MetadataSignature),
					"media_sig": string(asset.AssetSignature),
					"pubkey":    string(asset.PubKey),
					"ots":       asset.Ots,
					"gst":       string(asset.Gst),
				},
			},
		}
		metadatas = append(metadatas, metadata)
	}
	return metadatas, nil
}

func getWaczFileMetadata(filePath string) (map[string]any, error) {
	metadata, err := wacz.GetVerifiedMetadata(filePath)
	if err != nil {
		return nil, err
	}
	metadata["asset_origin_id"] = getAssetOriginRoot(filePath)
	return metadata, nil
}

// getFileMetadata calculates and returns a map of attributes for a file
func getFileMetadata(filePath string, mediaType string) (map[string]any, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"media_type":        mediaType,
		"asset_origin_id":   getAssetOriginRoot(filePath),
		"asset_origin_type": []string{"folder"},
		"file_name":         fileInfo.Name(),
		"last_modified":     fileInfo.ModTime().UTC().Format(time.RFC3339),
		"time_created":      fileInfo.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// checkFileType checks if the file is a zip based special file type
// that we will handle differently
func checkFileType(filePath string) (string, string, error) {
	fileType := "generic" // default is generic
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil {
		return "", "", err
	}
	mediaType := http.DetectContentType(buffer[:n])
	if mediaType == "application/zip" {
		isProofMode := proofmode.CheckIsProofModeFile(filePath)
		if isProofMode {
			fileType = "proofmode"
		}
		isWacz := wacz.CheckIsWaczFile(filePath)
		if isWacz {
			fileType = "wacz"
		}
	}
	return fileType, mediaType, nil
}

// handleNewFile takes a discovered file, update file status on database,
// posts the new file and its metadata to the webhook server,
// and returns the CID of the file according to the server.
func handleNewFile(pgPool *pgxpool.Pool, filePath string, project *ProjectQueryResult) (string, error) {
	if project == nil {
		return "", fmt.Errorf("project not found for file %s", filePath)
	}

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

	fileType, mediaType, err := checkFileType(filePath)
	if err != nil {
		if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
			log.Println("error setting file status to error:", err)
		}
		return "", fmt.Errorf("error checking file type for %s: %v", filePath, err)
	}

	metadatas := []map[string]any{}
	switch fileType {
	case "proofmode":
		metadatas, err = getProofModeFileMetadatas(filePath)
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error getting proofmode file metadatas: %v", err)
		}
	case "wacz":
		fileMetadata, err := getFileMetadata(filePath, mediaType)
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error getting file metadata: %v", err)
		}
		waczMetadata, err := getWaczFileMetadata(filePath)
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error getting wacz file metadatas: %v", err)
		}
		metadata := map[string]any{}
		for k, v := range fileMetadata {
			metadata[k] = v
		}
		for k, v := range waczMetadata {
			metadata[k] = v
		}
		metadatas = append(metadatas, metadata)
	case "generic":
		metadata, err := getFileMetadata(filePath, mediaType)
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error getting file metadata: %v", err)
		}
		metadatas = append(metadatas, metadata)
	}

	err = setFileStatusUploading(pgPool, filePath)
	if err != nil {
		return "", fmt.Errorf("error setting file status to uploading: %v", err)
	}

	for _, metadata := range metadatas {
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

	switch fileType {
	case "proofmode":
		zipListing, err := zip.OpenReader(filePath)
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error opening zip file %s: %v", filePath, err)
		}
		defer zipListing.Close()
		fileMap, _, err := proofmode.GetMapOfZipFiles(zipListing)
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error getting files from zip: %v", err)
		}
		for _, metadata := range metadatas {
			fileName := metadata["file_name"].(string)
			if zipFile, ok := fileMap[fileName]; ok {
				file, err := zipFile.Open()
				if err != nil {
					if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
						log.Println("error setting file status to error:", err)
					}
					return "", fmt.Errorf("error opening file %s in zip: %v", fileName, err)
				}
				defer file.Close()
				resp, err := webhook.PostFileToWebHook(file, metadata, webhook.PostGenericWebhookOpt{Format: "cbor"})
				if err != nil {
					if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
						log.Println("error setting file status to error:", err)
					}
					return "", fmt.Errorf("error posting metadata for file %s: %v", filePath, err)
				}
				cid = resp.Cid
			} else {
				if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
					log.Println("error setting file status to error:", err)
				}
				return "", fmt.Errorf("file %s not found in zip", fileName)
			}
		}
	case "wacz":
	case "generic":
		file, err := os.Open(filePath)
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error opening file %s: %v", filePath, err)
		}
		defer file.Close()
		resp, err := webhook.PostFileToWebHook(file, metadatas[0], webhook.PostGenericWebhookOpt{})
		if err != nil {
			if err := setFileStatusError(pgPool, filePath, err.Error()); err != nil {
				log.Println("error setting file status to error:", err)
			}
			return "", fmt.Errorf("error posting metadata for file %s: %v", filePath, err)
		}
		cid = resp.Cid
	}

	err = setFileStatusDone(pgPool, filePath, cid)
	if err != nil {
		return "", fmt.Errorf("error setting file status to done: %v", err)
	}
	return cid, nil
}

// shouldIncludeFile reports whether the file should be included in the processing
func shouldIncludeFile(fileName string) bool {
	if fileName[0] == '.' {
		return false
	}
	fileExt := filepath.Ext(fileName)
	return fileExt != ".partial"
}
