package folder

// schema for file_status table
var fileStatusTableSchema = `CREATE TABLE IF NOT EXISTS file_status (
		id BIGSERIAL PRIMARY KEY,
		file_path TEXT UNIQUE NOT NULL,
		status TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		cid TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_file_status_file_path ON file_status (file_path);
	CREATE INDEX IF NOT EXISTS idx_file_status_status ON file_status (status);
`
