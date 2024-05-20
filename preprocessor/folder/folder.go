package preprocessor_folder

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/fsnotify/fsnotify"
	"github.com/gabriel-vasile/mimetype"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
	"lukechampine.com/blake3"
)

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

	mtype, err := mimetype.DetectReader(tee)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"sha256":           hex.EncodeToString(sha.Sum(nil)),
		"md5":              hex.EncodeToString(md.Sum(nil)),
		"blake3":           hex.EncodeToString(blake.Sum(nil)),
		"mimetype":         mtype.String(),
		"fileSize":         fileInfo.Size(),
		"fileName":         fileInfo.Name(),
		"fileLastModified": fileInfo.ModTime(),
	}, nil
}

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

func scanDirectory(subPath string) ([]string, error) {
	scanRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	if scanRoot == "" {
		scanRoot = "."
	}
	scanPath := filepath.Join(scanRoot, subPath)
	fileList := []string{}
	err := filepath.Walk(scanPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if checkShouldIncludeFile(info) {
			fileList = append(fileList, path)
			fmt.Println("Found: " + path)
			return nil
		}
		return nil
	})
	return fileList, err
}

func Run(args []string) {
	fileList, err := scanDirectory("")
	if err != nil {
		util.Die("error scanning directory: %v", err)
	}
	for _, filePath := range fileList {
		metadata, err := getFileMetadata(filePath)
		if err != nil {
			fmt.Printf("error getting metadata for file %s: %v", filePath, err)
		}
		fmt.Println(metadata)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		util.Die("error creating file watcher: %v", err)
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					file, err := os.Open(event.Name)
					if err != nil {
						// File may be moved away for fsnotify.Rename
						continue
					}
					defer file.Close()
					fileInfo, err := file.Stat()
					if err != nil {
						fmt.Println("error getting file info:", err)
						continue
					}
					if checkShouldIncludeFile(fileInfo) {
						metadata, err := getFileMetadata(event.Name)
						if err != nil {
							fmt.Println("error getting metadata for file:", err)
							continue
						}
						fmt.Println(metadata)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("error:", err)
			}
		}
	}()

	scanRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	err = watcher.Add(scanRoot)
	if err != nil {
		util.Die("error adding watch for directory: %v", err)
	}

	// Block main goroutine forever.
	<-make(chan struct{})
}
