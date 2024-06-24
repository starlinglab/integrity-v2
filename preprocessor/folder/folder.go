package folder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rjeczalik/notify"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/database"
)

type Projects struct {
	mu   sync.RWMutex
	list []*ProjectQueryResult
}

// findProjectWithFilePath finds the project
// in which ProjectPath is the parent directory of the given file path
func findProjectWithFilePath(filePath string, projects []*ProjectQueryResult) *ProjectQueryResult {
	syncRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	for _, project := range projects {
		projectPath := project.ProjectPath
		projectPath = filepath.Join(syncRoot, projectPath) + "/"
		if strings.HasPrefix(filePath, projectPath) {
			return project
		}
	}
	return nil
}

// scanSyncDirectory scans a path under the sync directory and returns a list of files
func scanSyncDirectory(subPath string) (fileList []string, err error) {
	scanRoot := config.GetConfig().FolderPreprocessor.SyncFolderRoot
	if scanRoot == "" {
		return nil, fmt.Errorf("sync folder root not set")
	}
	scanPath := filepath.Join(scanRoot, subPath)
	log.Println("scanning: " + scanPath)
	err = filepath.WalkDir(scanPath, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if shouldIncludeFile(info.Name()) {
			fileList = append(fileList, path)
			return nil
		}
		return nil
	})
	return fileList, err
}

// Scan the sync directory and watch for file changes
func Run(args []string) error {
	dbConfig := config.GetConfig().FolderDatabase
	pgPool, err := database.GetDatabaseConnectionPool(database.DatabaseConfig(dbConfig))
	if err != nil {
		return err
	}
	defer database.CloseDatabaseConnectionPool()
	err = initDbTableIfNotExists(pgPool)
	if err != nil {
		return err
	}
	var projects Projects
	list, err := queryAllProjects(pgPool)
	if err != nil {
		return err
	}
	projects.list = list

	for _, project := range projects.list {
		projectPath := project.ProjectPath
		fileList, err := scanSyncDirectory(projectPath)
		if err != nil {
			log.Println(err)
		}
		for _, filePath := range fileList {
			cid, err := handleNewFile(pgPool, filePath, project)
			if err != nil {
				log.Println(err)
			} else if cid != "" {
				log.Printf("file %s uploaded to webhook with CID %s\n", filePath, cid)
			}
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
				if shouldIncludeFile(filepath.Base(filePath)) {
					fileInfo, err := os.Stat(filePath)
					if err != nil {
						log.Println("error getting file info:", err)
						return
					}
					if fileInfo.IsDir() {
						return
					}

					projects.mu.RLock()
					project := findProjectWithFilePath(filePath, projects.list)
					projects.mu.RUnlock()
					if project == nil {
						list, err = queryAllProjects(pgPool)
						if err != nil {
							log.Println(err)
						} else {
							projects.mu.Lock()
							projects.list = list
							projects.mu.Unlock()
							projects.mu.RLock()
							project = findProjectWithFilePath(filePath, projects.list)
							projects.mu.RUnlock()
							if project == nil {
								log.Printf("no project found for file %s\n", filePath)
								return
							}
						}
					}
					cid, err := handleNewFile(pgPool, filePath, project)
					if err != nil {
						log.Println(err)
					} else if cid != "" {
						log.Printf("file %s uploaded to webhook with CID %s\n", filePath, cid)
					}
				}
			}()
		}
	}
}
