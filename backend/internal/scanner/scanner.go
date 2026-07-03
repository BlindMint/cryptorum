package scanner

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cryptorum/internal/coverprefs"
	"cryptorum/internal/covers"
	"cryptorum/internal/metadata"
)

var ErrScanCancelled = errors.New("scan cancelled")

const (
	fullFileHashAlgorithm      = "sha256-full-v1"
	legacySampledHashAlgorithm = "sha256-sampled-v1"
	legacySampleMaxHashSize    = 10 * 1024 * 1024
	legacySampleSize           = 64 * 1024
)

// Supported formats
var supportedFormats = map[string]bool{
	"epub": true, "pdf": true,
	"cbz": true, "cbr": true, "cb7": true,
	"mp3": true, "m4a": true, "m4b": true,
	"flac": true, "ogg": true, "wav": true,
	"mobi": true, "azw3": true,
	"fb2": true, "rtf": true, "txt": true,
}

// Scanner handles library scanning
type Scanner struct {
	db         *sql.DB
	dataPath   string
	coversPath string
}

type ScanProgress struct {
	TotalFiles     int
	ScannedFiles   int
	ImportedBooks  int
	FailedFiles    int
	CurrentPath    string
	CurrentStatus  string
	CurrentError   string
	UnchangedFiles int
	MissingFiles   int
	ChangedFiles   int
	RelinkedFiles  int
	DuplicateFiles int
	Phase          string
}

type ScanProgressFunc func(progress ScanProgress)
type ScanCancelFunc func() bool

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
	return s.ScanLibraryWithProgress(libraryID, paths, nil)
}

// ScanLibraryWithProgress scans a library path and reports per-file progress.
func (s *Scanner) ScanLibraryWithProgress(libraryID int64, paths []string, onProgress ScanProgressFunc) (int, error) {
	return s.ScanLibraryWithProgressAndCancel(libraryID, paths, onProgress, nil)
}

func (s *Scanner) ScanLibraryWithProgressAndCancel(libraryID int64, paths []string, onProgress ScanProgressFunc, shouldCancel ScanCancelFunc) (int, error) {
	var ownerUserID int64 = 1
	_ = s.db.QueryRow(`SELECT COALESCE(owner_user_id, 1) FROM library WHERE id = ?`, libraryID).Scan(&ownerUserID)

	progress := ScanProgress{Phase: "inventory"}
	files, err := collectProcessableFiles(paths, shouldCancel)
	if errors.Is(err, ErrScanCancelled) {
		progress.Phase = "cancelled"
		if onProgress != nil {
			onProgress(progress)
		}
		return 0, ErrScanCancelled
	}
	if err != nil {
		slog.Warn("Library inventory completed with errors", "libraryID", libraryID, "error", err)
	}
	progress.TotalFiles = len(files)
	if onProgress != nil {
		onProgress(progress)
	}

	imported := 0
	scanStartedAt := time.Now().Unix()
	seenPaths := make(map[string]struct{}, len(files))
	for _, file := range files {
		seenPaths[file.Path] = struct{}{}
	}

	existing, err := s.loadLibraryFileInventory(libraryID)
	if err != nil {
		return 0, err
	}

	missing, err := s.markMissingFiles(libraryID, seenPaths, scanStartedAt)
	if err != nil {
		slog.Warn("Failed to pre-mark missing files", "libraryID", libraryID, "error", err)
	} else {
		progress.MissingFiles = missing
	}

	progress.Phase = "processing"
	if onProgress != nil {
		onProgress(progress)
	}
	for _, file := range files {
		if shouldCancel != nil && shouldCancel() {
			progress.Phase = "cancelled"
			if onProgress != nil {
				onProgress(progress)
			}
			return imported, ErrScanCancelled
		}
		if record, ok := existing[file.Path]; ok &&
			record.Size == file.Size &&
			record.LastModified == file.ModTimeUnix &&
			record.MissingAt == 0 &&
			record.HashAlgorithm == fullFileHashAlgorithm {
			if shouldUseFilenameTitle(record.Title) {
				if err := s.saveFilenameFallbackTitle(record.BookID, file.Path, ownerUserID); err != nil {
					slog.Debug("Skipped filename title fallback", "path", file.Path, "error", err)
				}
			}
			progress.ScannedFiles++
			progress.UnchangedFiles++
			progress.CurrentPath = file.Path
			progress.CurrentStatus = "unchanged"
			progress.CurrentError = ""
			if onProgress != nil {
				onProgress(progress)
			}
			continue
		}

		result, err := s.processFileWithInfo(libraryID, file, ownerUserID, scanStartedAt)
		progress.ScannedFiles++
		progress.CurrentPath = file.Path
		if err != nil {
			progress.FailedFiles++
			progress.CurrentStatus = "failed"
			progress.CurrentError = err.Error()
			slog.Error("Failed to process file", "path", file.Path, "error", err)
			if onProgress != nil {
				onProgress(progress)
			}
			continue
		}
		if result.Imported {
			imported++
			progress.ImportedBooks++
		}
		switch result.Status {
		case scanStatusChanged:
			progress.ChangedFiles++
		case scanStatusRelinked, scanStatusMovedLibrary, scanStatusRestored:
			progress.RelinkedFiles++
		case scanStatusDuplicate:
			progress.DuplicateFiles++
		}
		progress.CurrentStatus = result.Status
		progress.CurrentError = ""
		if onProgress != nil {
			onProgress(progress)
		}
		time.Sleep(5 * time.Millisecond)
	}

	progress.Phase = "complete"
	if onProgress != nil {
		onProgress(progress)
	}

	return imported, nil
}

type fileInventoryItem struct {
	Path        string
	Format      string
	Size        int64
	ModTimeUnix int64
}

type processFileResult struct {
	Imported bool
	Status   string
}

const (
	scanStatusImported     = "imported"
	scanStatusUpdated      = "updated"
	scanStatusChanged      = "changed"
	scanStatusRestored     = "restored"
	scanStatusRelinked     = "relinked"
	scanStatusMovedLibrary = "moved_library"
	scanStatusDuplicate    = "duplicate"
)

func (s *Scanner) extractMetadata(bookID, libraryID int64, path string) (*metadata.BookMetadata, error) {
	return metadata.ExtractWithOptions(path, metadata.ExtractOptions{
		ComicSpreadFallbackSide: s.resolveComicSpreadFallback(bookID, libraryID),
	})
}

func (s *Scanner) resolveComicSpreadFallback(bookID, libraryID int64) string {
	settings := covers.LoadSettings(s.db)
	bookValue := coverprefs.ComicSpreadFallbackInherit
	libraryValue := coverprefs.ComicSpreadFallbackInherit

	if bookID > 0 {
		_ = s.db.QueryRow(`
			SELECT COALESCE(bm.comic_spread_fallback, ?), COALESCE(l.comic_spread_fallback, ?)
			FROM book b
			JOIN library l ON b.library_id = l.id
			LEFT JOIN book_metadata bm ON b.id = bm.book_id
			WHERE b.id = ?
		`, coverprefs.ComicSpreadFallbackInherit, coverprefs.ComicSpreadFallbackInherit, bookID).Scan(&bookValue, &libraryValue)
	} else if libraryID > 0 {
		_ = s.db.QueryRow(`
			SELECT COALESCE(comic_spread_fallback, ?)
			FROM library
			WHERE id = ?
		`, coverprefs.ComicSpreadFallbackInherit, libraryID).Scan(&libraryValue)
	}

	return coverprefs.ResolveComicSpreadFallback(bookValue, libraryValue, settings.ComicSpreadFallback)
}

type existingFileRecord struct {
	ID            int64
	BookID        int64
	Path          string
	Size          int64
	Hash          string
	HashAlgorithm string
	LastModified  int64
	MissingAt     int64
	Title         string
}

func collectProcessableFiles(paths []string, shouldCancel ScanCancelFunc) ([]fileInventoryItem, error) {
	files := []fileInventoryItem{}
	var firstErr error
	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if shouldCancel != nil && shouldCancel() {
				return ErrScanCancelled
			}
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return nil
			}
			if entry.IsDir() || !isProcessableFile(entry.Name()) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != "" {
				ext = ext[1:]
			}
			files = append(files, fileInventoryItem{
				Path:        path,
				Format:      ext,
				Size:        info.Size(),
				ModTimeUnix: info.ModTime().Unix(),
			})
			return nil
		})
		if errors.Is(err, ErrScanCancelled) {
			return files, ErrScanCancelled
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return files, firstErr
}

func (s *Scanner) loadLibraryFileInventory(libraryID int64) (map[string]existingFileRecord, error) {
	rows, err := s.db.Query(`
		SELECT bf.id, bf.book_id, bf.path, bf.size, bf.hash, COALESCE(bf.hash_algorithm, ''),
		       bf.last_modified, COALESCE(bf.missing_at, 0),
		       COALESCE(bm.title, '')
		FROM book_file bf
		JOIN book b ON b.id = bf.book_id
		LEFT JOIN book_metadata bm ON bm.book_id = b.id
		WHERE b.library_id = ?
	`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := map[string]existingFileRecord{}
	for rows.Next() {
		var record existingFileRecord
		if err := rows.Scan(
			&record.ID,
			&record.BookID,
			&record.Path,
			&record.Size,
			&record.Hash,
			&record.HashAlgorithm,
			&record.LastModified,
			&record.MissingAt,
			&record.Title,
		); err != nil {
			continue
		}
		record.HashAlgorithm = normalizeHashAlgorithm(record.HashAlgorithm, record.Size)
		records[record.Path] = record
	}
	return records, rows.Err()
}

func (s *Scanner) markMissingFiles(libraryID int64, seenPaths map[string]struct{}, scanStartedAt int64) (int, error) {
	rows, err := s.db.Query(`
		SELECT bf.id, bf.path
		FROM book_file bf
		JOIN book b ON b.id = bf.book_id
		WHERE b.library_id = ? AND bf.missing_at IS NULL
	`, libraryID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	missingIDs := []int64{}
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			continue
		}
		if _, ok := seenPaths[path]; !ok {
			missingIDs = append(missingIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range missingIDs {
		if _, err := s.db.Exec(`UPDATE book_file SET missing_at = ? WHERE id = ?`, scanStartedAt, id); err != nil {
			return len(missingIDs), err
		}
	}
	return len(missingIDs), nil
}

// RefreshMissingMetadata finds books with no title or no cover and re-extracts metadata from file.
// Returns the number of books updated.
func (s *Scanner) RefreshMissingMetadata(limit int) (int, error) {
	rows, err := s.db.Query(`
		SELECT b.id, b.library_id, bf.path, COALESCE(l.owner_user_id, 1)
		FROM book b
		JOIN book_file bf ON b.id = bf.book_id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE bm.book_id IS NULL OR bm.title IS NULL OR bm.title = '' OR LOWER(TRIM(bm.title)) = 'untitled' OR bm.cover_path IS NULL
		LIMIT ?
	`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type entry struct {
		bookID    int64
		libraryID int64
		filePath  string
		ownerID   int64
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.bookID, &e.libraryID, &e.filePath, &e.ownerID); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	rows.Close()

	count := 0
	for _, e := range entries {
		meta, err := s.extractMetadata(e.bookID, e.libraryID, e.filePath)
		if err != nil {
			slog.Debug("Using filename fallback for missing metadata refresh", "path", e.filePath, "error", err)
		}
		meta = metadataWithFilenameTitleFallback(meta, e.filePath)
		if err := s.saveMetadata(e.bookID, meta, e.ownerID); err != nil {
			slog.Error("Failed to save metadata", "bookID", e.bookID, "error", err)
			continue
		}
		count++
	}
	return count, nil
}

// scanDirectory recursively scans a directory for book files
func (s *Scanner) scanDirectory(
	libraryID int64,
	dirPath string,
	ownerUserID int64,
	progress *ScanProgress,
	onProgress ScanProgressFunc,
) (int, error) {
	count := 0

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(dirPath, entry.Name())

		if entry.IsDir() {
			subCount, err := s.scanDirectory(libraryID, path, ownerUserID, progress, onProgress)
			if err != nil {
				slog.Error("Failed to scan subdirectory", "path", path, "error", err)
				continue
			}
			count += subCount
		} else if isProcessableFile(entry.Name()) {
			imported, err := s.processFile(libraryID, path, ownerUserID)
			progress.ScannedFiles++
			progress.CurrentPath = path
			if err != nil {
				progress.FailedFiles++
				slog.Error("Failed to process file", "path", path, "error", err)
				if onProgress != nil {
					onProgress(*progress)
				}
				continue
			}
			if imported {
				count++
				progress.ImportedBooks++
			}
			if onProgress != nil {
				onProgress(*progress)
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
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" {
		ext = ext[1:]
	}
	result, err := s.processFileWithInfo(libraryID, fileInventoryItem{
		Path:        path,
		Format:      ext,
		Size:        info.Size(),
		ModTimeUnix: info.ModTime().Unix(),
	}, ownerUserID, time.Now().Unix())
	return result.Imported, err
}

func (s *Scanner) processFileWithInfo(
	libraryID int64,
	file fileInventoryItem,
	ownerUserID int64,
	scanSeenAt int64,
) (processFileResult, error) {
	hashes, err := computeFileHashes(file.Path)
	if err != nil {
		return processFileResult{}, fmt.Errorf("failed to compute hash: %w", err)
	}

	// Check if path already exists before duplicate detection so rescans can repair
	// weak metadata from older extraction logic.
	var existingFileID int64
	var existingBookID int64
	var existingLibraryID int64
	var existingHash string
	var existingHashAlgorithm string
	var existingMissingAt int64
	err = s.db.QueryRow(`
		SELECT bf.id, bf.book_id, b.library_id, bf.hash, COALESCE(bf.hash_algorithm, ''),
		       COALESCE(bf.missing_at, 0)
		FROM book_file bf
		JOIN book b ON b.id = bf.book_id
		WHERE bf.path = ?
	`, file.Path).Scan(&existingFileID, &existingBookID, &existingLibraryID, &existingHash, &existingHashAlgorithm, &existingMissingAt)
	if err == nil {
		existingHashAlgorithm = normalizeHashAlgorithm(existingHashAlgorithm, file.Size)
		status := scanStatusUpdated
		if existingMissingAt > 0 {
			status = scanStatusRestored
		}
		if existingLibraryID != libraryID {
			status = scanStatusMovedLibrary
		}
		if existingHashAlgorithm == fullFileHashAlgorithm && existingHash != hashes.Full && existingMissingAt == 0 {
			status = scanStatusChanged
		}
		if err := s.updateKnownBookFile(existingBookID, existingFileID, libraryID, file, hashes, ownerUserID, scanSeenAt); err != nil {
			return processFileResult{}, err
		}
		if existingHash != hashes.Full {
			slog.Info("Updated file hash", "path", file.Path)
		}
		if repairsExtractedMetadata(file.Format) {
			if repairErr := s.repairWeakExtractedMetadata(existingBookID, file.Path, ownerUserID); repairErr != nil {
				slog.Debug("Skipped metadata repair", "path", file.Path, "error", repairErr)
			}
		}
		if repairErr := s.saveFilenameFallbackTitleIfWeak(existingBookID, file.Path, ownerUserID); repairErr != nil {
			slog.Debug("Skipped filename title fallback", "path", file.Path, "error", repairErr)
		}
		return processFileResult{Status: status}, nil
	}

	match, hasRelinkableMatch, hasActiveDuplicate, hasActiveDuplicateInLibrary, err := s.findRelinkableChecksumMatch(libraryID, file, hashes, scanSeenAt)
	if err != nil {
		return processFileResult{}, err
	}
	if hasRelinkableMatch {
		if err := s.relinkBookFile(match, libraryID, file, hashes, ownerUserID, scanSeenAt); err != nil {
			return processFileResult{}, err
		}
		status := scanStatusRelinked
		if match.LibraryID != libraryID {
			status = scanStatusMovedLibrary
		}
		slog.Info("Relinked moved book file", "path", file.Path, "bookID", match.BookID, "fromLibraryID", match.LibraryID, "toLibraryID", libraryID)
		return processFileResult{Status: status}, nil
	}
	if hasActiveDuplicateInLibrary {
		slog.Debug("File already exists in this library", "path", file.Path, "hash", hashes.Full)
		return processFileResult{Status: scanStatusDuplicate}, nil
	}
	if hasActiveDuplicate {
		slog.Debug("Importing duplicate content into a different library", "path", file.Path, "hash", hashes.Full)
	}

	now := time.Now().Unix()

	result, err := s.db.Exec(`
		INSERT INTO book (library_id, added_at, last_scanned, owner_user_id) VALUES (?, ?, ?, ?)
	`, libraryID, now, now, ownerUserID)
	if err != nil {
		return processFileResult{}, fmt.Errorf("failed to insert book: %w", err)
	}

	bookID, err := result.LastInsertId()
	if err != nil {
		return processFileResult{}, fmt.Errorf("failed to get book ID: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO book_file (book_id, path, format, size, hash, hash_algorithm, last_modified, owner_user_id, scan_seen_at, missing_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, bookID, file.Path, file.Format, file.Size, hashes.Full, fullFileHashAlgorithm, file.ModTimeUnix, ownerUserID, scanSeenAt)
	if err != nil {
		return processFileResult{}, fmt.Errorf("failed to insert book file: %w", err)
	}

	// Extract and save metadata immediately
	meta, err := s.extractMetadata(bookID, libraryID, file.Path)
	if err != nil {
		slog.Warn("Failed to extract metadata", "path", file.Path, "error", err)
	}
	meta = metadataWithFilenameTitleFallback(meta, file.Path)
	if saveErr := s.saveMetadata(bookID, meta, ownerUserID); saveErr != nil {
		slog.Warn("Failed to save metadata", "path", file.Path, "error", saveErr)
	}

	slog.Info("Imported new book", "path", file.Path, "bookID", bookID)
	status := scanStatusImported
	if hasActiveDuplicate {
		status = scanStatusDuplicate
	}
	return processFileResult{Imported: true, Status: status}, nil
}

type computedFileHashes struct {
	Full   string
	Legacy string
}

type checksumMatch struct {
	FileID        int64
	BookID        int64
	LibraryID     int64
	Path          string
	MissingAt     int64
	HashAlgorithm string
}

func normalizeHashAlgorithm(algorithm string, size int64) string {
	algorithm = strings.TrimSpace(algorithm)
	if algorithm != "" {
		return algorithm
	}
	if size > legacySampleMaxHashSize {
		return legacySampledHashAlgorithm
	}
	return fullFileHashAlgorithm
}

func (s *Scanner) updateKnownBookFile(
	bookID int64,
	fileID int64,
	libraryID int64,
	file fileInventoryItem,
	hashes computedFileHashes,
	ownerUserID int64,
	scanSeenAt int64,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		UPDATE book
		SET library_id = ?, last_scanned = ?, owner_user_id = ?
		WHERE id = ?
	`, libraryID, scanSeenAt, ownerUserID, bookID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE book_file
		SET path = ?, format = ?, size = ?, hash = ?, hash_algorithm = ?, last_modified = ?,
		    scan_seen_at = ?, missing_at = NULL, owner_user_id = ?
		WHERE id = ?
	`, file.Path, file.Format, file.Size, hashes.Full, fullFileHashAlgorithm, file.ModTimeUnix, scanSeenAt, ownerUserID, fileID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE book_metadata
		SET owner_user_id = ?
		WHERE book_id = ?
	`, ownerUserID, bookID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Scanner) findRelinkableChecksumMatch(
	libraryID int64,
	file fileInventoryItem,
	hashes computedFileHashes,
	missingAt int64,
) (checksumMatch, bool, bool, bool, error) {
	rows, err := s.db.Query(`
		SELECT bf.id, bf.book_id, b.library_id, bf.path, COALESCE(bf.missing_at, 0),
		       COALESCE(bf.hash_algorithm, '')
		FROM book_file bf
		JOIN book b ON b.id = bf.book_id
		WHERE bf.size = ? AND bf.hash IN (?, ?)
	`, file.Size, hashes.Full, hashes.Legacy)
	if err != nil {
		return checksumMatch{}, false, false, false, err
	}
	defer rows.Close()

	relinkable := []checksumMatch{}
	hasActiveDuplicate := false
	hasActiveDuplicateInLibrary := false
	for rows.Next() {
		var match checksumMatch
		if err := rows.Scan(&match.FileID, &match.BookID, &match.LibraryID, &match.Path, &match.MissingAt, &match.HashAlgorithm); err != nil {
			continue
		}
		if match.Path == file.Path {
			continue
		}
		match.HashAlgorithm = normalizeHashAlgorithm(match.HashAlgorithm, file.Size)
		if match.MissingAt > 0 {
			relinkable = append(relinkable, match)
			continue
		}
		if pathLikelyExists(match.Path) {
			hasActiveDuplicate = true
			if match.LibraryID == libraryID {
				hasActiveDuplicateInLibrary = true
			}
			continue
		}
		if _, err := s.db.Exec(`UPDATE book_file SET missing_at = ? WHERE id = ? AND missing_at IS NULL`, missingAt, match.FileID); err != nil {
			return checksumMatch{}, false, false, false, err
		}
		match.MissingAt = missingAt
		relinkable = append(relinkable, match)
	}
	if err := rows.Err(); err != nil {
		return checksumMatch{}, false, false, false, err
	}
	if len(relinkable) == 0 {
		return checksumMatch{}, false, hasActiveDuplicate, hasActiveDuplicateInLibrary, nil
	}

	sort.SliceStable(relinkable, func(i, j int) bool {
		leftSameLibrary := relinkable[i].LibraryID == libraryID
		rightSameLibrary := relinkable[j].LibraryID == libraryID
		if leftSameLibrary != rightSameLibrary {
			return leftSameLibrary
		}
		leftFull := relinkable[i].HashAlgorithm == fullFileHashAlgorithm
		rightFull := relinkable[j].HashAlgorithm == fullFileHashAlgorithm
		if leftFull != rightFull {
			return leftFull
		}
		return relinkable[i].MissingAt > relinkable[j].MissingAt
	})

	return relinkable[0], true, hasActiveDuplicate, hasActiveDuplicateInLibrary, nil
}

func pathLikelyExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func (s *Scanner) relinkBookFile(
	match checksumMatch,
	libraryID int64,
	file fileInventoryItem,
	hashes computedFileHashes,
	ownerUserID int64,
	scanSeenAt int64,
) error {
	return s.updateKnownBookFile(match.BookID, match.FileID, libraryID, file, hashes, ownerUserID, scanSeenAt)
}

func repairsExtractedMetadata(ext string) bool {
	switch ext {
	case "pdf", "cbz", "cbr", "cb7", "mobi", "azw3", "mp3", "m4a", "m4b", "flac", "ogg", "wav":
		return true
	default:
		return false
	}
}

func metadataWithFilenameTitleFallback(meta *metadata.BookMetadata, path string) *metadata.BookMetadata {
	if meta == nil {
		meta = metadata.ExtractFilename(path)
	}
	if meta.Authors == nil {
		meta.Authors = []string{}
	}
	if meta.Genres == nil {
		meta.Genres = []string{}
	}
	if shouldUseFilenameTitle(meta.Title) {
		meta.Title = filenameFallbackTitle(path)
	}
	return meta
}

func shouldUseFilenameTitle(title string) bool {
	title = strings.TrimSpace(title)
	return title == "" || strings.EqualFold(title, "Untitled")
}

func filenameFallbackTitle(path string) string {
	generated := metadata.ExtractFilename(path)
	title := strings.TrimSpace(generated.Title)
	if title != "" {
		return title
	}
	return strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

func (s *Scanner) saveFilenameFallbackTitleIfWeak(bookID int64, path string, ownerUserID int64) error {
	var title string
	err := s.db.QueryRow("SELECT COALESCE(title, '') FROM book_metadata WHERE book_id = ?", bookID).Scan(&title)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && !shouldUseFilenameTitle(title) {
		return nil
	}
	return s.saveFilenameFallbackTitle(bookID, path, ownerUserID)
}

func (s *Scanner) saveFilenameFallbackTitle(bookID int64, path string, ownerUserID int64) error {
	title := filenameFallbackTitle(path)
	if title == "" {
		return nil
	}
	return s.saveMetadata(bookID, &metadata.BookMetadata{
		Title:   title,
		Authors: []string{},
		Genres:  []string{},
		Source:  "filename",
	}, ownerUserID)
}

// saveMetadata upserts book metadata and saves the cover image to disk
func (s *Scanner) saveMetadata(bookID int64, meta *metadata.BookMetadata, ownerUserID int64) error {
	authorsJSON, _ := json.Marshal(meta.Authors)
	emptyGenresJSON, _ := json.Marshal([]string{})
	tagsJSON, _ := json.Marshal(meta.Genres)
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
		    (book_id, title, authors, series, series_number, series_number_display, publisher, pub_date,
		     description, rating, genres, tags, isbn, asin, language, page_count, cover_path, cover_updated_on, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
		    title         = COALESCE(NULLIF(excluded.title, ''), title),
		    authors       = COALESCE(NULLIF(excluded.authors, '[]'), authors),
		    series        = COALESCE(NULLIF(excluded.series, ''), series),
		    series_number = COALESCE(NULLIF(excluded.series_number, 0), series_number),
		    series_number_display = COALESCE(NULLIF(excluded.series_number_display, ''), series_number_display),
		    publisher     = COALESCE(NULLIF(excluded.publisher, ''), publisher),
		    pub_date      = COALESCE(NULLIF(excluded.pub_date, ''), pub_date),
		    description   = COALESCE(NULLIF(excluded.description, ''), description),
		    rating        = COALESCE(NULLIF(excluded.rating, 0), rating),
		    tags          = COALESCE(NULLIF(excluded.tags, '[]'), tags),
		    isbn          = COALESCE(NULLIF(excluded.isbn, ''), isbn),
		    asin          = COALESCE(NULLIF(excluded.asin, ''), asin),
		    language      = COALESCE(NULLIF(excluded.language, ''), language),
		    page_count    = COALESCE(NULLIF(excluded.page_count, 0), page_count),
		    cover_path    = COALESCE(NULLIF(excluded.cover_path, ''), cover_path),
		    cover_updated_on = CASE
		        WHEN excluded.cover_path != '' THEN excluded.cover_updated_on
		        ELSE cover_updated_on
		    END
	`, bookID, meta.Title, string(authorsJSON), meta.Series, meta.SeriesNumber, meta.SeriesNumberDisplay,
		meta.Publisher, meta.PubDate, meta.Description, meta.Rating,
		string(emptyGenresJSON), string(tagsJSON), meta.ISBN, meta.ASIN, meta.Language, meta.PageCount, coverPath, coverUpdatedOn, ownerUserID)

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
	var libraryID int64
	err := s.db.QueryRow(`
		SELECT COALESCE(title, ''), COALESCE(authors, '[]'), COALESCE(publisher, ''),
		       COALESCE(pub_date, ''), COALESCE(page_count, 0), COALESCE(cover_path, ''),
		       b.library_id
		FROM book b
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.id = ?
	`, bookID).Scan(&title, &authorsRaw, &publisher, &pubDate, &pageCount, &coverPath, &libraryID)
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

	extracted, err := s.extractMetadata(bookID, libraryID, path)
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

// computeFileHash computes a full-file SHA-256 hash.
func computeFileHash(path string) (string, error) {
	hashes, err := computeFileHashes(path)
	if err != nil {
		return "", err
	}
	return hashes.Full, nil
}

// computeFileHashes computes the current full-file SHA-256 hash and the legacy
// sampled fingerprint used by older scans for files over 10 MiB.
func computeFileHashes(path string) (computedFileHashes, error) {
	file, err := os.Open(path)
	if err != nil {
		return computedFileHashes{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return computedFileHashes{}, err
	}

	fullHash := sha256.New()
	if info.Size() <= legacySampleMaxHashSize {
		if _, err := io.Copy(fullHash, file); err != nil {
			return computedFileHashes{}, err
		}
		sum := hex.EncodeToString(fullHash.Sum(nil))
		return computedFileHashes{Full: sum, Legacy: sum}, nil
	}

	firstSample := make([]byte, 0, legacySampleSize)
	middleSample := make([]byte, 0, legacySampleSize)
	lastSample := make([]byte, 0, legacySampleSize)
	middleStart := info.Size() / 2
	lastStart := info.Size() - legacySampleSize

	buf := make([]byte, 1024*1024)
	offset := int64(0)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			fullHash.Write(chunk)
			appendSampleRange(&firstSample, chunk, offset, 0, legacySampleSize)
			appendSampleRange(&middleSample, chunk, offset, middleStart, middleStart+legacySampleSize)
			appendSampleRange(&lastSample, chunk, offset, lastStart, info.Size())
			offset += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return computedFileHashes{}, readErr
		}
	}

	legacyHash := sha256.New()
	legacyHash.Write(firstSample)
	legacyHash.Write(middleSample)
	legacyHash.Write(lastSample)
	legacyHash.Write([]byte(fmt.Sprintf("%d", info.Size())))

	return computedFileHashes{
		Full:   hex.EncodeToString(fullHash.Sum(nil)),
		Legacy: hex.EncodeToString(legacyHash.Sum(nil)),
	}, nil
}

func appendSampleRange(dst *[]byte, chunk []byte, chunkStart int64, sampleStart int64, sampleEnd int64) {
	chunkEnd := chunkStart + int64(len(chunk))
	if chunkEnd <= sampleStart || chunkStart >= sampleEnd {
		return
	}
	from := maxInt64(sampleStart, chunkStart) - chunkStart
	to := minInt64(sampleEnd, chunkEnd) - chunkStart
	*dst = append(*dst, chunk[from:to]...)
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
