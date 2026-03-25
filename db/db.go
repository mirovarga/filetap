package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mirovarga/filetap/source"

	_ "modernc.org/sqlite"
)

// DB provides file storage and querying backed by SQLite.
type DB struct {
	db *sql.DB
}

const schemaSQL = `
CREATE TABLE files (
    hash       TEXT NOT NULL,
    source_id  TEXT NOT NULL DEFAULT 'default',
    path       TEXT NOT NULL,
    name       TEXT NOT NULL,
    baseName   TEXT NOT NULL,
    ext        TEXT NOT NULL,
    size       INTEGER NOT NULL,
    modifiedAt DATETIME NOT NULL,
    mime       TEXT NOT NULL,
    PRIMARY KEY (hash, source_id)
);

CREATE TABLE file_dirs (
    file_hash  TEXT NOT NULL,
    source_id  TEXT NOT NULL,
    dir        TEXT NOT NULL,
    position   INTEGER NOT NULL,
    FOREIGN KEY (file_hash, source_id) REFERENCES files(hash, source_id) ON DELETE CASCADE
);

CREATE INDEX idx_files_source_id ON files(source_id);
CREATE INDEX idx_files_path ON files(path);
CREATE INDEX idx_files_name ON files(name);
CREATE INDEX idx_files_baseName ON files(baseName);
CREATE INDEX idx_files_ext ON files(ext);
CREATE INDEX idx_files_size ON files(size);
CREATE INDEX idx_files_modifiedAt ON files(modifiedAt);
CREATE INDEX idx_files_mime ON files(mime);
CREATE INDEX idx_file_dirs_lookup ON file_dirs(file_hash, source_id);
CREATE INDEX idx_file_dirs_dir ON file_dirs(dir);
`

// New creates a DB backed by SQLite at the given path.
func New(dbPath string) (*DB, error) {
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	if dbPath != ":memory:" {
		if _, err := database.Exec("PRAGMA journal_mode=WAL"); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("setting WAL mode: %w", err)
		}
	}

	if _, err := database.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	if _, err := database.Exec(schemaSQL); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &DB{db: database}, nil
}

// NewInMemory creates a DB backed by an in-memory SQLite database.
func NewInMemory() (*DB, error) {
	return New(":memory:")
}

// Insert stores the given files under the specified source ID in a single transaction.
func (d *DB) Insert(ctx context.Context, sourceID string, files []*source.FileInfo) error {
	transaction, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer transaction.Rollback()

	fileStatement, err := transaction.PrepareContext(ctx, `
		INSERT OR IGNORE INTO files (hash, source_id, path, name, baseName, ext, size, modifiedAt, mime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing file insert: %w", err)
	}
	defer fileStatement.Close()

	dirStatement, err := transaction.PrepareContext(ctx, `
		INSERT OR IGNORE INTO file_dirs (file_hash, source_id, dir, position)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing dir insert: %w", err)
	}
	defer dirStatement.Close()

	for _, file := range files {
		modifiedAtStr := ""
		if !file.ModifiedAt.IsZero() {
			modifiedAtStr = file.ModifiedAt.UTC().Format(time.RFC3339)
		}

		if _, err := fileStatement.ExecContext(ctx,
			file.Hash,
			sourceID,
			file.Path,
			file.Name,
			file.BaseName,
			file.Ext,
			file.Size,
			modifiedAtStr,
			file.Mime,
		); err != nil {
			return fmt.Errorf("inserting '%s': %w", file.Path, err)
		}

		for position, dir := range file.Dirs {
			if _, err := dirStatement.ExecContext(ctx,
				file.Hash,
				sourceID,
				dir,
				position,
			); err != nil {
				return fmt.Errorf("inserting dir for '%s': %w", file.Path, err)
			}
		}
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// FindByHash returns the file with the given hash, or false if not found.
func (d *DB) FindByHash(ctx context.Context, hash string) (*source.FileInfo, bool, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT f.hash, f.path, f.name, f.baseName, f.ext, f.size, f.modifiedAt, f.mime, fd.dir
		FROM files f
		LEFT JOIN file_dirs fd ON fd.file_hash = f.hash AND fd.source_id = f.source_id
		WHERE f.hash = ?
		ORDER BY fd.position`, hash)
	if err != nil {
		return nil, false, fmt.Errorf("querying file by hash '%s': %w", hash, err)
	}
	defer rows.Close()

	var file *source.FileInfo
	for rows.Next() {
		scanned, dir, err := scanFileWithDir(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scanning file by hash '%s': %w", hash, err)
		}
		if file == nil {
			file = scanned
			file.Dirs = make([]string, 0)
		}
		if dir.Valid {
			file.Dirs = append(file.Dirs, dir.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterating rows for hash '%s': %w", hash, err)
	}
	if file == nil {
		return nil, false, nil
	}
	return file, true, nil
}

// Find executes the query and returns matching files along with the total count.
func (d *DB) Find(ctx context.Context, query *FileQuery) ([]*source.FileInfo, int, error) {
	countSQL, countArgs, err := query.BuildCount()
	if err != nil {
		return nil, 0, fmt.Errorf("building count query: %w", err)
	}
	var total int
	if err := d.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting: %w", err)
	}

	querySQL, queryArgs, err := query.BuildWithDirs()
	if err != nil {
		return nil, 0, fmt.Errorf("building query: %w", err)
	}
	rows, err := d.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying: %w", err)
	}
	defer rows.Close()

	files := make([]*source.FileInfo, 0)
	var currentFile *source.FileInfo
	for rows.Next() {
		scanned, dir, err := scanFileWithDir(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning row: %w", err)
		}
		if currentFile == nil || currentFile.Hash != scanned.Hash {
			currentFile = scanned
			currentFile.Dirs = make([]string, 0)
			files = append(files, currentFile)
		}
		if dir.Valid {
			currentFile.Dirs = append(currentFile.Dirs, dir.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating rows: %w", err)
	}

	return files, total, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

func scanFileWithDir(rows *sql.Rows) (*source.FileInfo, sql.NullString, error) {
	var file source.FileInfo
	var modifiedAt string
	var dir sql.NullString

	if err := rows.Scan(
		&file.Hash,
		&file.Path,
		&file.Name,
		&file.BaseName,
		&file.Ext,
		&file.Size,
		&modifiedAt,
		&file.Mime,
		&dir,
	); err != nil {
		return nil, sql.NullString{}, err
	}

	if modifiedAt != "" {
		parsedTime, err := time.Parse(time.RFC3339, modifiedAt)
		if err != nil {
			return nil, sql.NullString{}, fmt.Errorf("parsing modifiedAt: %w", err)
		}
		file.ModifiedAt = parsedTime
	}

	return &file, dir, nil
}
