package preprocessor_folder

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

func scanSyncDirectory(subPath string) ([]string, error) {
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
	// Scan whole sync directory
	fileList, err := scanSyncDirectory("")
	if err != nil {
		util.Die("error scanning directory: %v", err)
	}
	for _, filePath := range fileList {
		cid, err := handleNewFile(filePath)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Printf("File %s uploaded to webhook with CID %s\n", filePath, cid)
		}
	}

	// Init directory watcher
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
					filePath := event.Name
					file, err := os.Open(filePath)
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
						cid, err := handleNewFile(filePath)
						if err != nil {
							fmt.Println(err)
						} else {
							fmt.Printf("File %s uploaded to webhook with CID %s\n", filePath, cid)
						}
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
