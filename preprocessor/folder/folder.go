package folder

import (
	"log"
	"os"
	"path/filepath"

	"github.com/rjeczalik/notify"
	"github.com/starlinglab/integrity-v2/config"
)

// scanSyncDirectory scans a path under the sync directory and returns a list of files
func scanSyncDirectory(subPath string) ([]string, error) {
	scanRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	if scanRoot == "" {
		scanRoot = "."
	}
	scanPath := filepath.Join(scanRoot, subPath)
	fileList := []string{}
	err := filepath.WalkDir(scanPath, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if shouldIncludeFile(info.Name()) {
			fileList = append(fileList, path)
			log.Println("Found: " + path)
			return nil
		}
		return nil
	})
	return fileList, err
}

func Run(args []string) error {
	// Scan whole sync directory
	fileList, err := scanSyncDirectory("")
	if err != nil {
		return err
	}
	for _, filePath := range fileList {
		cid, err := handleNewFile(filePath)
		if err != nil {
			log.Println(err)
		} else {
			log.Printf("File %s uploaded to webhook with CID %s\n", filePath, cid)
		}
	}

	// Init directory watcher
	c := make(chan notify.EventInfo, 1)
	scanRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	err = notify.Watch(scanRoot+"/...", c, notify.Create, notify.Rename)
	if err != nil {
		return err
	}
	defer notify.Stop(c)

	for {
		ei := <-c
		event := ei.Event()
		if event == notify.Rename || event == notify.Create {
			filePath := ei.Path()
			file, err := os.Open(filePath)
			if err != nil {
				// File may be moved away for notify.Rename
				continue
			}
			fileInfo, err := file.Stat()
			if err != nil {
				log.Println("error getting file info:", err)
				continue
			}
			if shouldIncludeFile(fileInfo.Name()) {
				cid, err := handleNewFile(filePath)
				if err != nil {
					log.Println(err)
				} else {
					log.Printf("File %s uploaded to webhook with CID %s\n", filePath, cid)
				}
			}
			file.Close()
		}
	}
}
