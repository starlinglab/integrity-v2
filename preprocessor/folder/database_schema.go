package folder

// schema for file_status table
const fileStatusTableSchema = `CREATE TABLE IF NOT EXISTS file_status (
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

// schema for project_metadata table
const projectMetadataTable = `CREATE TABLE IF NOT EXISTS project_metadata (
	id BIGSERIAL PRIMARY KEY,
	project_id TEXT UNIQUE NOT NULL,
	project_path TEXT UNIQUE NOT NULL,
	author_type TEXT NOT NULL DEFAULT '',
	author_name TEXT NOT NULL DEFAULT '',
	author_identifier TEXT NOT NULL DEFAULT '',
	file_extensions TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_project_metadata_project_id ON project_metadata (project_id);
`

const allowedKeysTable = `CREATE TABLE IF NOT EXISTS allowed_keys (
	id BIGSERIAL PRIMARY KEY,
	project_id TEXT NOT NULL,
	key_type TEXT NOT NULL,
	key TEXT NOT NULL,
	name TEXT NOT NULL
);`
