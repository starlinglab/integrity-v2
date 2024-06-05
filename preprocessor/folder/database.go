package folder

import (
	"fmt"
	"log"
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

// initDbTableIfNotExists initializes the database tables if they do not exist
func initDbTableIfNotExists(connPool *pgxpool.Pool) error {
	return initFileStatusTableIfNotExists(connPool)
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
func setFileStatusError(connPool *pgxpool.Pool, filePath string, errorMessage string) {
	_, err := connPool.Exec(
		db.GetDatabaseContext(),
		"UPDATE file_status SET status = $1, error = $2, updated_at = $3 WHERE file_path = $4;",
		FileStatusError,
		errorMessage,
		time.Now().UTC(),
		filePath,
	)
	if err != nil {
		log.Println("error setting file status to error:", err)
	}
}
