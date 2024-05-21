package folder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rjeczalik/notify"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/database"
)

// scanSyncDirectory scans a path under the sync directory and returns a list of files
func scanSyncDirectory(subPath string) ([]string, error) {
	scanRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	if scanRoot == "" {
		return nil, fmt.Errorf("sync folder root not set")
	}
	scanPath := filepath.Join(scanRoot, subPath)
	fileList := []string{}
	err := filepath.WalkDir(scanPath, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if shouldIncludeFile(info.Name()) {
			fileList = append(fileList, path)
			log.Println("found: " + path)
			return nil
		}
		return nil
	})
	return fileList, err
}

func Run(args []string) error {
	pgPool, err := database.GetDatabaseConnectionPool()
	if err != nil {
		return err
	}
	defer database.CloseDatabaseConnectionPool()
	err = initDbTableIfNotExists(pgPool)
	if err != nil {
		return err
	}

	// Scan whole sync directory
	fileList, err := scanSyncDirectory("")
	if err != nil {
		return err
	}
	for _, filePath := range fileList {
		cid, err := handleNewFile(pgPool, filePath)
		if err != nil {
			log.Println(err)
		} else {
			log.Printf("file %s uploaded to webhook with CID %s\n", filePath, cid)
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
			go func() {
				filePath := ei.Path()
				file, err := os.Open(filePath)
				if err != nil {
					// File may be moved away for notify.Rename
					return
				}
				fileInfo, err := file.Stat()
				if err != nil {
					log.Println("error getting file info:", err)
					return
				}
				if shouldIncludeFile(fileInfo.Name()) {
					cid, err := handleNewFile(pgPool, filePath)
					if err != nil {
						log.Println(err)
					} else {
						log.Printf("file %s uploaded to webhook with CID %s\n", filePath, cid)
					}
				}
				file.Close()
			}()
		}
	}
}
