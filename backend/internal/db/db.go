package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// DB wraps the database connection
type DB struct {
	*sql.DB
	dataPath string
}

// New creates a new database connection and runs migrations
func New(dataPath string) (*DB, error) {
	dbPath := filepath.Join(dataPath, "cryptorum.db")

	// Open SQLite database with WAL mode
	conn, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_timeout=60")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// SQLite with WAL supports concurrent readers. Allow a small pool so one
	// slow query or background write does not serialize every request.
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(4)

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	db := &DB{
		DB:       conn,
		dataPath: dataPath,
	}

	// Run migrations
	if err := db.runMigrations(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	slog.Info("Database initialized", "path", dbPath)
	return db, nil
}

// runMigrations runs Goose migrations from embedded files
func (db *DB) runMigrations() error {
	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("failed to set dialect: %w", err)
	}

	if err := goose.Up(db.DB, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	slog.Info("Database migrations completed")
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.DB != nil {
		return db.DB.Close()
	}
	return nil
}

// Backup creates a backup of the database
func (db *DB) Backup(backupPath string) error {
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		return err
	}

	escapedPath := strings.ReplaceAll(backupPath, "'", "''")
	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapedPath))
	return err
}
