package scanner

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cryptorum/internal/covers"
	"cryptorum/internal/metadata"
)

// Supported formats
var supportedFormats = map[string]bool{
	"epub": true, "pdf": true,
	"cbz": true, "cbr": true, "cb7": true,
	"mp3": true, "m4a": true, "m4b": true,
	"flac": true, "ogg": true, "wav": true,
	"mobi": true, "azw3": true,
}

// Scanner handles library scanning
type Scanner struct {
	db         *sql.DB
	dataPath   string
	coversPath string
}

// New creates a new scanner
func New(db *sql.DB, dataPath string, coversPath string) *Scanner {
	return &Scanner{
		db:         db,
		dataPath:   dataPath,
		coversPath: coversPath,
	}
}

// ScanLibrary scans a library path and imports new books
func (s *Scanner) ScanLibrary(libraryID int64, paths []string) (int, error) {
	var ownerUserID int64 = 1
	_ = s.db.QueryRow(`SELECT COALESCE(owner_user_id, 1) FROM library WHERE id = ?`, libraryID).Scan(&ownerUserID)

	imported := 0

	for _, path := range paths {
		count, err := s.scanDirectory(libraryID, path, ownerUserID)
		if err != nil {
			slog.Error("Failed to scan directory", "path", path, "error", err)
			continue
		}
		imported += count
	}

	return imported, nil
}

// RefreshMissingMetadata finds books with no title or no cover and re-extracts metadata from file.
// Returns the number of books updated.
func (s *Scanner) RefreshMissingMetadata(limit int) (int, error) {
	rows, err := s.db.Query(`
		SELECT b.id, bf.path, COALESCE(l.owner_user_id, 1)
		FROM book b
		JOIN book_file bf ON b.id = bf.book_id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE bm.book_id IS NULL OR bm.title IS NULL OR bm.title = '' OR bm.cover_path IS NULL
		LIMIT ?
	`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type entry struct {
		bookID   int64
		filePath string
		ownerID  int64
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.bookID, &e.filePath, &e.ownerID); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	rows.Close()

	count := 0
	for _, e := range entries {
		meta, err := metadata.Extract(e.filePath)
		if err != nil || meta == nil {
			continue
		}
		if err := s.saveMetadata(e.bookID, meta, e.ownerID); err != nil {
			slog.Error("Failed to save metadata", "bookID", e.bookID, "error", err)
			continue
		}
		count++
	}
	return count, nil
}

// scanDirectory recursively scans a directory for book files
func (s *Scanner) scanDirectory(libraryID int64, dirPath string, ownerUserID int64) (int, error) {
	count := 0

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(dirPath, entry.Name())

		if entry.IsDir() {
			subCount, err := s.scanDirectory(libraryID, path, ownerUserID)
			if err != nil {
				slog.Error("Failed to scan subdirectory", "path", path, "error", err)
				continue
			}
			count += subCount
		} else if isProcessableFile(entry.Name()) {
			imported, err := s.processFile(libraryID, path, ownerUserID)
			if err != nil {
				slog.Error("Failed to process file", "path", path, "error", err)
				continue
			}
			if imported {
				count++
			}
		}
	}

	return count, nil
}

// isProcessableFile checks if a file is a supported book format
func isProcessableFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return false
	}
	ext = ext[1:] // Remove the dot
	return supportedFormats[ext]
}

// processFile processes a single book file
func (s *Scanner) processFile(libraryID int64, path string, ownerUserID int64) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("failed to stat file: %w", err)
	}

	hash, err := computeFileHash(path)
	if err != nil {
		return false, fmt.Errorf("failed to compute hash: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" {
		ext = ext[1:]
	}

	// Check if path already exists before duplicate detection so rescans can repair
	// weak metadata from older extraction logic.
	var existingFileID int64
	var existingBookID int64
	var existingHash string
	err = s.db.QueryRow("SELECT id, book_id, hash FROM book_file WHERE path = ?", path).Scan(&existingFileID, &existingBookID, &existingHash)
	if err == nil {
		if existingHash != hash {
			s.db.Exec("UPDATE book_file SET hash = ?, last_modified = ? WHERE id = ?",
				hash, info.ModTime().Unix(), existingFileID)
			slog.Info("Updated file hash", "path", path)
		}
		if repairsExtractedMetadata(ext) {
			if repairErr := s.repairWeakExtractedMetadata(existingBookID, path, ownerUserID); repairErr != nil {
				slog.Debug("Skipped metadata repair", "path", path, "error", repairErr)
			}
		}
		return false, nil
	}

	// Check for existing file by hash (duplicate detection)
	err = s.db.QueryRow(`
		SELECT b.id FROM book b
		JOIN book_file bf ON b.id = bf.book_id
		WHERE bf.hash = ?
	`, hash).Scan(&existingBookID)
	if err == nil {
		slog.Debug("File already exists", "path", path, "hash", hash)
		return false, nil
	}

	now := time.Now().Unix()

	result, err := s.db.Exec(`
		INSERT INTO book (library_id, added_at, last_scanned, owner_user_id) VALUES (?, ?, ?, ?)
	`, libraryID, now, now, ownerUserID)
	if err != nil {
		return false, fmt.Errorf("failed to insert book: %w", err)
	}

	bookID, err := result.LastInsertId()
	if err != nil {
		return false, fmt.Errorf("failed to get book ID: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO book_file (book_id, path, format, size, hash, last_modified, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, bookID, path, ext, info.Size(), hash, info.ModTime().Unix(), ownerUserID)
	if err != nil {
		return false, fmt.Errorf("failed to insert book file: %w", err)
	}

	// Extract and save metadata immediately
	meta, err := metadata.Extract(path)
	if err != nil {
		slog.Warn("Failed to extract metadata", "path", path, "error", err)
	} else if meta != nil {
		if saveErr := s.saveMetadata(bookID, meta, ownerUserID); saveErr != nil {
			slog.Warn("Failed to save metadata", "path", path, "error", saveErr)
		}
	}

	slog.Info("Imported new book", "path", path, "bookID", bookID)
	return true, nil
}

func repairsExtractedMetadata(ext string) bool {
	switch ext {
	case "pdf", "mobi", "azw3", "mp3", "m4a", "m4b", "flac", "ogg", "wav":
		return true
	default:
		return false
	}
}

// saveMetadata upserts book metadata and saves the cover image to disk
func (s *Scanner) saveMetadata(bookID int64, meta *metadata.BookMetadata, ownerUserID int64) error {
	authorsJSON, _ := json.Marshal(meta.Authors)
	genresJSON, _ := json.Marshal(meta.Genres)
	var existingCoverPath string
	_ = s.db.QueryRow("SELECT COALESCE(cover_path, '') FROM book_metadata WHERE book_id = ?", bookID).Scan(&existingCoverPath)

	// Save cover image
	coverPath := ""
	coverUpdatedOn := int64(0)
	if len(meta.CoverData) > 0 {
		settings := covers.LoadSettings(s.db)
		processed, err := covers.ProcessCover(meta.CoverData, settings)
		if err == nil && len(processed) > 0 {
			if savedPath, saveErr := covers.SaveCoverBytes(s.coversPath, bookID, processed); saveErr == nil {
				coverPath = savedPath
				coverUpdatedOn = time.Now().Unix()
			}
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO book_metadata
		    (book_id, title, authors, series, series_number, publisher, pub_date,
		     description, rating, genres, isbn, asin, language, page_count, cover_path, cover_updated_on, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
		    title         = COALESCE(NULLIF(excluded.title, ''), title),
		    authors       = COALESCE(NULLIF(excluded.authors, '[]'), authors),
		    series        = COALESCE(NULLIF(excluded.series, ''), series),
		    series_number = COALESCE(NULLIF(excluded.series_number, 0), series_number),
		    publisher     = COALESCE(NULLIF(excluded.publisher, ''), publisher),
		    pub_date      = COALESCE(NULLIF(excluded.pub_date, ''), pub_date),
		    description   = COALESCE(NULLIF(excluded.description, ''), description),
		    rating        = COALESCE(NULLIF(excluded.rating, 0), rating),
		    genres        = COALESCE(NULLIF(excluded.genres, '[]'), genres),
		    isbn          = COALESCE(NULLIF(excluded.isbn, ''), isbn),
		    asin          = COALESCE(NULLIF(excluded.asin, ''), asin),
		    language      = COALESCE(NULLIF(excluded.language, ''), language),
		    page_count    = COALESCE(NULLIF(excluded.page_count, 0), page_count),
		    cover_path    = COALESCE(NULLIF(excluded.cover_path, ''), cover_path),
		    cover_updated_on = CASE
		        WHEN excluded.cover_path != '' THEN excluded.cover_updated_on
		        ELSE cover_updated_on
		    END
	`, bookID, meta.Title, string(authorsJSON), meta.Series, meta.SeriesNumber,
		meta.Publisher, meta.PubDate, meta.Description, meta.Rating,
		string(genresJSON), meta.ISBN, meta.ASIN, meta.Language, meta.PageCount, coverPath, coverUpdatedOn, ownerUserID)

	if err != nil {
		return err
	}

	if coverPath != "" && existingCoverPath != "" && existingCoverPath != coverPath {
		_ = os.Remove(existingCoverPath)
	}

	s.syncFTSFromDB(bookID)

	return nil
}

func (s *Scanner) repairWeakExtractedMetadata(bookID int64, path string, ownerUserID int64) error {
	var title, authorsRaw, publisher, pubDate, coverPath string
	var pageCount int
	err := s.db.QueryRow(`
		SELECT COALESCE(title, ''), COALESCE(authors, '[]'), COALESCE(publisher, ''),
		       COALESCE(pub_date, ''), COALESCE(page_count, 0), COALESCE(cover_path, '')
		FROM book_metadata
		WHERE book_id = ?
	`, bookID).Scan(&title, &authorsRaw, &publisher, &pubDate, &pageCount, &coverPath)
	if err != nil {
		return err
	}

	generated := metadata.ExtractFilename(path)
	oldFilenameShape := strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(generated.Title)) &&
		sameStringList(authorsRaw, generated.Authors)
	missingUsefulFields := publisher == "" || pubDate == "" || pageCount == 0 || coverPath == ""
	if !oldFilenameShape && !missingUsefulFields {
		return nil
	}

	extracted, err := metadata.Extract(path)
	if err != nil || extracted == nil || extracted.Source == "filename" {
		return err
	}

	if err := s.saveMetadata(bookID, extracted, ownerUserID); err != nil {
		return err
	}

	if oldFilenameShape && strings.TrimSpace(extracted.Title) != "" && len(extracted.Authors) > 0 {
		authorsJSON, _ := json.Marshal(extracted.Authors)
		_, err = s.db.Exec(`
			UPDATE book_metadata
			SET title = ?, authors = ?
			WHERE book_id = ?
		`, extracted.Title, string(authorsJSON), bookID)
		if err != nil {
			return err
		}
		s.syncFTS(bookID, extracted.Title, extracted.Authors, extracted.Description, extracted.Series)
	}

	return nil
}

func sameStringList(raw string, expected []string) bool {
	var existing []string
	if err := json.Unmarshal([]byte(raw), &existing); err != nil {
		return false
	}
	if len(existing) != len(expected) {
		return false
	}
	for i := range existing {
		if !strings.EqualFold(strings.TrimSpace(existing[i]), strings.TrimSpace(expected[i])) {
			return false
		}
	}
	return true
}

func (s *Scanner) syncFTSFromDB(bookID int64) {
	var title, authorsRaw, description, series string
	err := s.db.QueryRow(`
		SELECT COALESCE(title, ''), COALESCE(authors, '[]'), COALESCE(description, ''), COALESCE(series, '')
		FROM book_metadata
		WHERE book_id = ?
	`, bookID).Scan(&title, &authorsRaw, &description, &series)
	if err != nil {
		slog.Warn("Failed to read metadata for FTS sync", "bookID", bookID, "error", err)
		return
	}

	var authors []string
	if err := json.Unmarshal([]byte(authorsRaw), &authors); err != nil {
		authors = []string{}
	}
	s.syncFTS(bookID, title, authors, description, series)
}

// syncFTS updates the FTS5 index for a book
func (s *Scanner) syncFTS(bookID int64, title string, authors []string, description, series string) {
	// Delete existing entry first
	s.db.Exec("DELETE FROM book_fts WHERE rowid = (SELECT id FROM book_metadata WHERE book_id = ?)", bookID)

	// Insert new entry with normalized authors (strip JSON array wrapping for better search)
	authorsStr := strings.Join(authors, " ")
	_, err := s.db.Exec(`
		INSERT INTO book_fts(rowid, title, authors, description, series)
		SELECT id, ?, ?, ?, ? FROM book_metadata WHERE book_id = ?
	`, title, authorsStr, description, series, bookID)
	if err != nil {
		slog.Warn("Failed to sync FTS", "bookID", bookID, "error", err)
	}
}

// RebuildFTS rebuilds the entire FTS index from book_metadata
func (s *Scanner) RebuildFTS() error {
	slog.Info("Rebuilding FTS index...")

	// Clear existing FTS data
	if _, err := s.db.Exec("DELETE FROM book_fts"); err != nil {
		return fmt.Errorf("failed to clear FTS: %w", err)
	}

	// Repopulate from book_metadata
	rows, err := s.db.Query(`
		SELECT bm.id, bm.title, bm.authors, bm.description, bm.series, bm.book_id
		FROM book_metadata bm
		WHERE bm.title IS NOT NULL AND bm.title != ''
	`)
	if err != nil {
		return fmt.Errorf("failed to query metadata: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int64
		var title, authorsJSON, description, series string
		if err := rows.Scan(&id, &title, &authorsJSON, &description, &series); err != nil {
			continue
		}

		// Parse authors JSON array and join into single string
		var authors []string
		if err := json.Unmarshal([]byte(authorsJSON), &authors); err != nil {
			authors = []string{}
		}
		authorsStr := strings.Join(authors, " ")

		_, err := s.db.Exec(`
			INSERT INTO book_fts(rowid, title, authors, description, series)
			VALUES (?, ?, ?, ?, ?)
		`, id, title, authorsStr, description, series)
		if err != nil {
			slog.Warn("Failed to insert FTS entry", "id", id, "error", err)
			continue
		}
		count++
	}

	slog.Info("FTS index rebuilt", "count", count)
	return nil
}

// computeFileHash computes SHA-256 hash of a file (partial for large files)
func computeFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	const maxHashSize = 10 * 1024 * 1024
	const sampleSize = 64 * 1024

	if info.Size() > maxHashSize {
		buf := make([]byte, sampleSize)
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		hash.Write(buf[:n])

		file.Seek(info.Size()/2, io.SeekStart)
		n, err = file.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		hash.Write(buf[:n])

		file.Seek(-int64(sampleSize), io.SeekEnd)
		n, err = file.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		hash.Write(buf[:n])

		hash.Write([]byte(fmt.Sprintf("%d", info.Size())))
	} else {
		if _, err := io.Copy(hash, file); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
