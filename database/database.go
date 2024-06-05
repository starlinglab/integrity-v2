package database

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pgPool *pgxpool.Pool
	pgOnce sync.Once
)

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// GetDatabaseContext returns a new context for database operations
func GetDatabaseContext() context.Context {
	return context.Background()
}

// GetDatabaseConnectionPool returns a thread safe connection pool singleton
func GetDatabaseConnectionPool(config DatabaseConfig) (*pgxpool.Pool, error) {
	var pgErr error = nil
	pgOnce.Do(func() {
		connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
			config.User,
			config.Password,
			config.Host,
			config.Port,
			config.Database,
		)
		db, err := pgxpool.New(GetDatabaseContext(), connString)
		pgPool = db
		pgErr = err
	})
	return pgPool, pgErr
}

// CloseDatabaseConnectionPool closes the database connection pool
func CloseDatabaseConnectionPool() {
	if pgPool != nil {
		pgPool.Close()
	}
}
