package folder

import (
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/starlinglab/integrity-v2/database"
)

// initFileStatusTableIfNotExists creates the file_status table if it does not exist
func initFileStatusTableIfNotExists(connPool *pgxpool.Pool) error {
	_, err := connPool.Exec(
		db.GetDatabaseContext(),
		fileStatusTableSchema,
	)
	return err
}

// initFileStatusTableIfNotExists creates the project_metadata table if it does not exist
func initProjectDataTableIfNotExists(connPool *pgxpool.Pool) error {
	_, err := connPool.Exec(
		db.GetDatabaseContext(),
		PROJECT_METADATA_TABLE,
	)
	if err != nil {
		return err
	}
	return nil
}

// initDbTableIfNotExists initializes the database tables if they do not exist
func initDbTableIfNotExists(connPool *pgxpool.Pool) error {
	err := initFileStatusTableIfNotExists(connPool)
	if err != nil {
		return err
	}
	err = initProjectDataTableIfNotExists(connPool)
	return err
}

// ProjectQueryResult represents the result of a project metadata query
type ProjectQueryResult struct {
	ProjectId        string
	ProjectPath      string
	AuthorType       string
	AuthorName       string
	AuthorIdentifier string
	FileExtensions   []string
}

// queryAllProjects queries all project metadata from the database
func queryAllProjects(connPool *pgxpool.Pool) ([]*ProjectQueryResult, error) {
	var result []*ProjectQueryResult
	rows, err := connPool.Query(
		db.GetDatabaseContext(),
		"SELECT project_id, project_path, author_type, author_name, author_identifier, file_extensions FROM project_metadata;",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row ProjectQueryResult
		var fileExtensionsString string
		err := rows.Scan(&row.ProjectId, &row.ProjectPath, &row.AuthorType, &row.AuthorName, &row.AuthorIdentifier, &fileExtensionsString)
		if err != nil {
			return nil, err
		}
		if fileExtensionsString != "" {
			row.FileExtensions = strings.Split(fileExtensionsString, ",")
		}
		result = append(result, &row)
	}
	return result, nil
}

// FileQueryResult represents the result of a file query
type FileQueryResult struct {
	Status       string
	Cid          string
	ErrorMessage string
}

func queryAndSetFoundFileStatus(connPool *pgxpool.Pool, filePath string) (*FileQueryResult, error) {
	var status string
	err := connPool.QueryRow(
		db.GetDatabaseContext(),
		`WITH new_file_status AS (
			INSERT INTO file_status (file_path, status, created_at, updated_at) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING
			RETURNING *
		) SELECT COALESCE(
				(SELECT status FROM new_file_status),
				(SELECT status FROM file_status WHERE file_path = $1)
		);`,
		filePath,
		FileStatusFound,
		time.Now().UTC(),
		time.Now().UTC(),
	).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("file status not found")
		}
		return nil, err
	}
	result := FileQueryResult{
		Status: status,
	}
	if status != FileStatusFound {
		err := connPool.QueryRow(
			db.GetDatabaseContext(),
			"SELECT status, cid, error FROM file_status WHERE file_path = $1;",
			filePath,
		).Scan(&result.Status, &result.Cid, &result.ErrorMessage)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("file status not found")
			}
			return nil, err
		}
	}
	return &result, nil
}

// setFileStatusUploading sets the status of a file to uploading
func setFileStatusUploading(connPool *pgxpool.Pool, filePath string) error {
	_, err := connPool.Exec(
		db.GetDatabaseContext(),
		"UPDATE file_status SET status = $1, updated_at = $2 WHERE file_path = $3;",
		FileStatusUploading,
		time.Now().UTC(),
		filePath,
	)
	return err
}

// setFileStatusDone sets the status of a file to done with cid
func setFileStatusDone(connPool *pgxpool.Pool, filePath string, cid string) error {
	_, err := connPool.Exec(
		db.GetDatabaseContext(),
		"UPDATE file_status SET status = $1, cid = $2, updated_at = $3 WHERE file_path = $4;",
		FileStatusSuccess,
		cid,
		time.Now().UTC(),
		filePath,
	)
	return err
}

// setFileStatusError sets the status of a file to error with the error message
func setFileStatusError(connPool *pgxpool.Pool, filePath string, errorMessage string) error {
	_, err := connPool.Exec(
		db.GetDatabaseContext(),
		"UPDATE file_status SET status = $1, error = $2, updated_at = $3 WHERE file_path = $4;",
		FileStatusError,
		errorMessage,
		time.Now().UTC(),
		filePath,
	)
	return err
}
