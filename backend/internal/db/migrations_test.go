package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestMigration18MergesGenresIntoTags(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cryptorum.db")
	conn, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.UpTo(conn, "migrations", 17); err != nil {
		t.Fatalf("migrate to 17: %v", err)
	}

	if _, err := conn.Exec(`INSERT INTO library (id, name) VALUES (1, 'Main')`); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO book (id, library_id, added_at, last_scanned) VALUES (1, 1, 100, 100)`); err != nil {
		t.Fatalf("insert book: %v", err)
	}
	if _, err := conn.Exec(`
		INSERT INTO book_metadata (book_id, title, genres, tags)
		VALUES (1, 'Tagged', '["Military","alpha","Space.Opera"]', '["zeta","Alpha","Personal"]')
	`); err != nil {
		t.Fatalf("insert metadata: %v", err)
	}

	if err := goose.Up(conn, "migrations"); err != nil {
		t.Fatalf("migrate to latest: %v", err)
	}

	var rawGenres string
	var rawTags string
	if err := conn.QueryRow(`SELECT COALESCE(genres, '[]'), COALESCE(tags, '[]') FROM book_metadata WHERE book_id = 1`).Scan(&rawGenres, &rawTags); err != nil {
		t.Fatalf("fetch migrated metadata: %v", err)
	}

	var genres []string
	if err := json.Unmarshal([]byte(rawGenres), &genres); err != nil {
		t.Fatalf("decode genres: %v", err)
	}
	if len(genres) != 0 {
		t.Fatalf("genres = %#v, want empty legacy column", genres)
	}

	var tags []string
	if err := json.Unmarshal([]byte(rawTags), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	want := []string{"Alpha", "Military", "Personal", "Space.Opera", "zeta"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %#v, want %#v", tags, want)
	}
}
