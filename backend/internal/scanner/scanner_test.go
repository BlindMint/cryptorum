package scanner

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	appdb "cryptorum/internal/db"
	"cryptorum/internal/metadata"
)

func TestMetadataWithFilenameTitleFallback(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		path     string
		expected string
	}{
		{
			name:     "empty title uses filename",
			title:    "",
			path:     "/books/Library/Useful Book.pdf",
			expected: "Useful Book",
		},
		{
			name:     "untitled title uses filename",
			title:    "Untitled",
			path:     "/books/Comics/Series 01 - The Beginning.cbz",
			expected: "The Beginning",
		},
		{
			name:     "existing useful title is preserved",
			title:    "Existing Title",
			path:     "/books/Other Name.epub",
			expected: "Existing Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := metadataWithFilenameTitleFallback(&metadata.BookMetadata{Title: tt.title}, tt.path)
			if meta.Title != tt.expected {
				t.Fatalf("Title = %q, want %q", meta.Title, tt.expected)
			}
		})
	}
}

func TestScanRelinksMovedBookAcrossLibraries(t *testing.T) {
	db := setupScannerTestDB(t)
	scanner := New(db.DB, t.TempDir(), filepath.Join(t.TempDir(), "covers"))

	oldDir := t.TempDir()
	newDir := t.TempDir()
	oldPath := filepath.Join(oldDir, "Original.txt")
	newPath := filepath.Join(newDir, "Moved.txt")
	writeTestFile(t, oldPath, []byte("same book content"))

	if imported, err := scanner.ScanLibrary(1, []string{oldDir}); err != nil || imported != 1 {
		t.Fatalf("initial scan imported=%d err=%v, want 1 nil", imported, err)
	}
	bookID := onlyBookID(t, db.DB)
	mustScannerExec(t, db.DB, `UPDATE book_metadata SET title = ?, tags = ? WHERE book_id = ?`, "User Edited Title", `["User.Tag"]`, bookID)
	mustScannerExec(t, db.DB, `INSERT INTO reading_progress (book_id, status, percent, updated_at, owner_user_id) VALUES (?, 'reading', 42, 100, 1)`, bookID)
	mustScannerExec(t, db.DB, `INSERT INTO shelf (id, name, owner_user_id) VALUES (1, 'Keep', 1)`)
	mustScannerExec(t, db.DB, `INSERT INTO book_shelf (book_id, shelf_id) VALUES (?, 1)`, bookID)

	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("rename moved book: %v", err)
	}

	statuses := scanStatuses(t, scanner, 2, []string{newDir})
	if !containsStatus(statuses, scanStatusMovedLibrary) {
		t.Fatalf("statuses = %#v, want %q", statuses, scanStatusMovedLibrary)
	}

	assertSingleRelinkedBook(t, db.DB, bookID, 2, newPath, "User Edited Title")
	var status string
	if err := db.QueryRow(`SELECT status FROM reading_progress WHERE book_id = ? AND owner_user_id = 1`, bookID).Scan(&status); err != nil {
		t.Fatalf("fetch reading progress: %v", err)
	}
	if status != "reading" {
		t.Fatalf("reading status = %q, want reading", status)
	}
	var shelfCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM book_shelf WHERE book_id = ? AND shelf_id = 1`, bookID).Scan(&shelfCount); err != nil {
		t.Fatalf("fetch shelf count: %v", err)
	}
	if shelfCount != 1 {
		t.Fatalf("shelf links = %d, want 1", shelfCount)
	}
}

func TestScanRelinksRenamedBookWithinLibrary(t *testing.T) {
	db := setupScannerTestDB(t)
	scanner := New(db.DB, t.TempDir(), filepath.Join(t.TempDir(), "covers"))

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "Old.txt")
	newPath := filepath.Join(dir, "New.txt")
	writeTestFile(t, oldPath, []byte("renamed book content"))

	if imported, err := scanner.ScanLibrary(1, []string{dir}); err != nil || imported != 1 {
		t.Fatalf("initial scan imported=%d err=%v, want 1 nil", imported, err)
	}
	bookID := onlyBookID(t, db.DB)
	mustScannerExec(t, db.DB, `UPDATE book_metadata SET title = ? WHERE book_id = ?`, "Preserved Rename", bookID)
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("rename book: %v", err)
	}

	statuses := scanStatuses(t, scanner, 1, []string{dir})
	if !containsStatus(statuses, scanStatusRelinked) {
		t.Fatalf("statuses = %#v, want %q", statuses, scanStatusRelinked)
	}
	assertSingleRelinkedBook(t, db.DB, bookID, 1, newPath, "Preserved Rename")
}

func TestScanRelinksLegacySampledHash(t *testing.T) {
	db := setupScannerTestDB(t)
	scanner := New(db.DB, t.TempDir(), filepath.Join(t.TempDir(), "covers"))

	newDir := t.TempDir()
	newPath := filepath.Join(newDir, "Large.pdf")
	writeLargeTestFile(t, newPath, legacySampleMaxHashSize+1024)
	hashes, err := computeFileHashes(newPath)
	if err != nil {
		t.Fatalf("compute hashes: %v", err)
	}
	oldPath := filepath.Join(t.TempDir(), "Old Large.pdf")

	mustScannerExec(t, db.DB, `INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id) VALUES (10, 1, 100, 100, 1)`)
	mustScannerExec(t, db.DB, `
		INSERT INTO book_file (book_id, path, format, size, hash, hash_algorithm, last_modified, owner_user_id)
		VALUES (10, ?, 'pdf', ?, ?, ?, 100, 1)
	`, oldPath, legacySampleMaxHashSize+1024, hashes.Legacy, legacySampledHashAlgorithm)
	mustScannerExec(t, db.DB, `INSERT INTO book_metadata (book_id, title, authors, genres, tags, owner_user_id) VALUES (10, 'Legacy Metadata', '[]', '[]', '[]', 1)`)

	statuses := scanStatuses(t, scanner, 2, []string{newDir})
	if !containsStatus(statuses, scanStatusMovedLibrary) {
		t.Fatalf("statuses = %#v, want %q", statuses, scanStatusMovedLibrary)
	}

	var storedHash string
	var algorithm string
	var path string
	if err := db.QueryRow(`SELECT hash, hash_algorithm, path FROM book_file WHERE book_id = 10`).Scan(&storedHash, &algorithm, &path); err != nil {
		t.Fatalf("fetch upgraded hash: %v", err)
	}
	if storedHash != hashes.Full {
		t.Fatalf("stored hash = %q, want full hash %q", storedHash, hashes.Full)
	}
	if algorithm != fullFileHashAlgorithm {
		t.Fatalf("hash algorithm = %q, want %q", algorithm, fullFileHashAlgorithm)
	}
	if path != newPath {
		t.Fatalf("path = %q, want %q", path, newPath)
	}
}

func TestScanImportsActiveCrossLibraryDuplicate(t *testing.T) {
	db := setupScannerTestDB(t)
	scanner := New(db.DB, t.TempDir(), filepath.Join(t.TempDir(), "covers"))

	firstDir := t.TempDir()
	secondDir := t.TempDir()
	writeTestFile(t, filepath.Join(firstDir, "Book.txt"), []byte("duplicate content"))
	writeTestFile(t, filepath.Join(secondDir, "Book Copy.txt"), []byte("duplicate content"))

	if imported, err := scanner.ScanLibrary(1, []string{firstDir}); err != nil || imported != 1 {
		t.Fatalf("first scan imported=%d err=%v, want 1 nil", imported, err)
	}
	statuses := scanStatuses(t, scanner, 2, []string{secondDir})
	if !containsStatus(statuses, scanStatusDuplicate) {
		t.Fatalf("statuses = %#v, want %q", statuses, scanStatusDuplicate)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM book`).Scan(&count); err != nil {
		t.Fatalf("count books: %v", err)
	}
	if count != 2 {
		t.Fatalf("book count = %d, want duplicate import count 2", count)
	}
}

func TestScanChangedSamePathPreservesUserMetadata(t *testing.T) {
	db := setupScannerTestDB(t)
	scanner := New(db.DB, t.TempDir(), filepath.Join(t.TempDir(), "covers"))

	dir := t.TempDir()
	path := filepath.Join(dir, "Mutable.txt")
	writeTestFile(t, path, []byte("before"))

	if imported, err := scanner.ScanLibrary(1, []string{dir}); err != nil || imported != 1 {
		t.Fatalf("initial scan imported=%d err=%v, want 1 nil", imported, err)
	}
	bookID := onlyBookID(t, db.DB)
	mustScannerExec(t, db.DB, `UPDATE book_metadata SET title = ? WHERE book_id = ?`, "User Title", bookID)

	writeTestFile(t, path, []byte("after"))
	statuses := scanStatuses(t, scanner, 1, []string{dir})
	if !containsStatus(statuses, scanStatusChanged) {
		t.Fatalf("statuses = %#v, want %q", statuses, scanStatusChanged)
	}
	assertSingleRelinkedBook(t, db.DB, bookID, 1, path, "User Title")
}

func setupScannerTestDB(t *testing.T) *appdb.DB {
	t.Helper()
	db, err := appdb.New(t.TempDir())
	if err != nil {
		t.Fatalf("create scanner test db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	mustScannerExec(t, db.DB, `
		INSERT INTO library (id, name, owner_user_id)
		VALUES (1, 'One', 1), (2, 'Two', 1)
	`)
	return db
}

func scanStatuses(t *testing.T, scanner *Scanner, libraryID int64, paths []string) []string {
	t.Helper()
	statuses := []string{}
	imported, err := scanner.ScanLibraryWithProgress(libraryID, paths, func(progress ScanProgress) {
		if progress.CurrentStatus != "" {
			statuses = append(statuses, progress.CurrentStatus)
		}
	})
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if imported > 1 {
		t.Fatalf("scan imported %d books, want at most 1", imported)
	}
	return statuses
}

func containsStatus(statuses []string, want string) bool {
	for _, status := range statuses {
		if status == want {
			return true
		}
	}
	return false
}

func onlyBookID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var bookID int64
	if err := db.QueryRow(`SELECT id FROM book`).Scan(&bookID); err != nil {
		t.Fatalf("fetch only book id: %v", err)
	}
	return bookID
}

func assertSingleRelinkedBook(t *testing.T, db *sql.DB, bookID int64, libraryID int64, path string, title string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM book`).Scan(&count); err != nil {
		t.Fatalf("count books: %v", err)
	}
	if count != 1 {
		t.Fatalf("book count = %d, want 1", count)
	}
	var gotLibraryID int64
	var gotPath string
	var missingAt sql.NullInt64
	var gotTitle string
	if err := db.QueryRow(`
		SELECT b.library_id, bf.path, bf.missing_at, bm.title
		FROM book b
		JOIN book_file bf ON bf.book_id = b.id
		JOIN book_metadata bm ON bm.book_id = b.id
		WHERE b.id = ?
	`, bookID).Scan(&gotLibraryID, &gotPath, &missingAt, &gotTitle); err != nil {
		t.Fatalf("fetch relinked book: %v", err)
	}
	if gotLibraryID != libraryID {
		t.Fatalf("library_id = %d, want %d", gotLibraryID, libraryID)
	}
	if gotPath != path {
		t.Fatalf("path = %q, want %q", gotPath, path)
	}
	if missingAt.Valid {
		t.Fatalf("missing_at = %d, want NULL", missingAt.Int64)
	}
	if gotTitle != title {
		t.Fatalf("title = %q, want %q", gotTitle, title)
	}
}

func mustScannerExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec query: %v", err)
	}
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func writeLargeTestFile(t *testing.T, path string, size int64) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create large test file: %v", err)
	}
	defer file.Close()

	chunk := make([]byte, 1024*1024)
	for i := range chunk {
		chunk[i] = byte(i % 251)
	}
	remaining := size
	for remaining > 0 {
		toWrite := int64(len(chunk))
		if remaining < toWrite {
			toWrite = remaining
		}
		if _, err := file.Write(chunk[:toWrite]); err != nil {
			t.Fatalf("write large test file: %v", err)
		}
		remaining -= toWrite
	}
}

func TestComputeFileHashUsesFullSHA256ForLargeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.pdf")
	writeLargeTestFile(t, path, legacySampleMaxHashSize+1024)

	got, err := computeFileHash(path)
	if err != nil {
		t.Fatalf("compute file hash: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read large file: %v", err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("hash = %q, want full SHA-256 %q", got, want)
	}
}
