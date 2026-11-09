package scanner

import (
	"context"
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
	"sync"
	"time"

	"cryptorum/internal/audiometa"
	"cryptorum/internal/coverprefs"
	"cryptorum/internal/covers"
	"cryptorum/internal/filenameinfo"
	"cryptorum/internal/metadata"
	"cryptorum/internal/metaprotection"
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
	db                 *sql.DB
	dataPath           string
	coversPath         string
	preparationWorkers int
	extractionSlot     chan struct{}
	scanMu             sync.Mutex
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
	Timings        ScanTimings
}

type ScanProgressFunc func(progress ScanProgress)
type ScanCancelFunc func() bool

// New creates a new scanner
func New(db *sql.DB, dataPath string, coversPath string) *Scanner {
	return NewWithWorkers(db, dataPath, coversPath, DefaultPreparationWorkers)
}

func NewWithWorkers(db *sql.DB, dataPath, coversPath string, workers int) *Scanner {
	if workers < 1 {
		workers = DefaultPreparationWorkers
	}
	if workers > MaxPreparationWorkers {
		workers = MaxPreparationWorkers
	}
	return &Scanner{db: db, dataPath: dataPath, coversPath: coversPath, preparationWorkers: workers, extractionSlot: make(chan struct{}, 1)}
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
	return s.ScanLibraryWithOptions(libraryID, paths, onProgress, shouldCancel, ScanOptions{Reconcile: true})
}

func (s *Scanner) ScanLibraryWithOptions(libraryID int64, paths []string, onProgress ScanProgressFunc, shouldCancel ScanCancelFunc, options ScanOptions) (int, error) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	started := time.Now()

	if shouldCancel != nil && shouldCancel() {
		return 0, ErrScanCancelled
	}
	var ownerUserID int64 = 1
	mediaScope := "mixed"
	_ = s.db.QueryRow(`SELECT COALESCE(owner_user_id, 1), COALESCE(media_scope, 'mixed') FROM library WHERE id = ?`, libraryID).Scan(&ownerUserID, &mediaScope)

	if err := validateScanRoots(paths); err != nil {
		return 0, err
	}
	progress := ScanProgress{Phase: "inventory", Timings: ScanTimings{Workers: s.preparationWorkers}}
	defer func() {
		slog.Info("Scan stage timings", "library_id", libraryID, "preparation_workers", s.preparationWorkers, "inventory_ms", progress.Timings.InventoryMS, "hash_ms", progress.Timings.HashMS, "metadata_ms", progress.Timings.MetadataMS, "cover_ms", progress.Timings.CoverMS, "commit_ms", progress.Timings.CommitMS, "checkpoint_ms", progress.Timings.CheckpointMS, "elapsed_ms", elapsedMS(started))
	}()
	emit := func() {
		progress.Timings.ElapsedMS = elapsedMS(started)
		if onProgress != nil {
			onProgress(progress)
		}
	}
	files, err := collectProcessableFiles(paths, shouldCancel)
	if errors.Is(err, ErrScanCancelled) {
		progress.Phase = "cancelled"
		emit()
		return 0, ErrScanCancelled
	}
	if err != nil {
		return 0, fmt.Errorf("library inventory incomplete; check mounted folders and permissions: %w", err)
	}
	files = filterFilesForMediaScope(files, mediaScope)
	progress.TotalFiles = len(files)
	emit()

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

	checkpoints, err := s.loadCheckpoints(options.JobID)
	if err != nil {
		return imported, err
	}
	progress.Timings.InventoryMS = elapsedMS(started)
	progress.Phase = "processing"
	emit()
	prepare := func(ctx context.Context, file fileInventoryItem) preparedFile {
		if record, ok := checkpoints[file.Path]; ok && record.Status != "failed" && record.Size == file.Size && record.ModifiedNS == file.ModTimeNS {
			if known, ok := existing[file.Path]; ok && known.IndexingComplete && known.HasMetadata && known.MissingAt == 0 && known.Size == file.Size && known.LastModified == file.ModTimeUnix {
				return preparedFile{file: file, fastStatus: record.Status, checkpointed: true}
			}
		}
		_, checkpointExists := checkpoints[file.Path]
		if record, ok := existing[file.Path]; ok && !checkpointExists && record.Size == file.Size && record.LastModified == file.ModTimeUnix && record.MissingAt == 0 && record.HashAlgorithm == fullFileHashAlgorithm && record.IndexingComplete && record.HasMetadata {
			return preparedFile{file: file, fastStatus: "unchanged"}
		}
		return s.prepareFile(ctx, libraryID, file)
	}
	consume := func(value preparedFile) error {
		file := value.file
		progress.Timings.HashMS += value.timings.HashMS
		progress.Timings.MetadataMS += value.timings.MetadataMS
		progress.Timings.CoverMS += value.timings.CoverMS
		status := value.fastStatus
		result := processFileResult{}
		fileErr := value.err
		if fileErr == nil && status != "" {
			fileErr = unchangedSinceInventory(file)
		}
		if fileErr != nil {
			value.checkpointed = false
		}

		commitStart := time.Now()
		if fileErr == nil && status == "" {
			result, fileErr = s.processPreparedFile(libraryID, value, ownerUserID, scanStartedAt)
			status = result.Status
		} else if fileErr == nil && status == "unchanged" && !value.checkpointed {
			record := existing[file.Path]
			if shouldUseFilenameMetadata(record.Title, record.AuthorsRaw) {
				if err := s.saveFilenameFallbackMetadataIfWeak(record.BookID, file.Path, ownerUserID); err != nil {
					slog.Debug("Skipped filename metadata fallback", "path", file.Path, "error", err)
				}
			}
		}
		progress.Timings.CommitMS += elapsedMS(commitStart)
		progress.ScannedFiles++
		progress.CurrentPath = file.Path
		progress.CurrentError = ""
		if fileErr != nil {
			status = "failed"
			progress.FailedFiles++
			progress.CurrentError = fileErr.Error()
			slog.Error("Failed to process file", "path", file.Path, "error", fileErr)
		} else {
			if result.Imported || (value.checkpointed && status == scanStatusImported) {
				imported++
				progress.ImportedBooks++
			}
			countScanStatus(&progress, status)
		}
		if !value.checkpointed {
			checkpointStart := time.Now()
			checkpointErr := s.saveCheckpoint(options.JobID, file, status, progress.CurrentError)
			progress.Timings.CheckpointMS += elapsedMS(checkpointStart)
			if checkpointErr != nil {
				return checkpointErr
			}
		}
		progress.CurrentStatus = status
		emit()
		return nil
	}
	err = runPreparationPipeline(context.Background(), s.preparationWorkers, files, prepare, consume, shouldCancel)
	if err != nil {
		if errors.Is(err, ErrScanCancelled) {
			progress.Phase = "cancelled"
			emit()
		}
		return imported, err
	}
	if shouldCancel != nil && shouldCancel() {
		progress.Phase = "cancelled"
		emit()
		return imported, ErrScanCancelled
	}

	if progress.FailedFiles > 0 {
		return imported, fmt.Errorf("%d files could not be imported; resume to retry failed files", progress.FailedFiles)
	}
	if options.Reconcile {
		// Revalidate roots before reconciling: a mount may have disappeared mid-scan.
		if err := validateScanRoots(paths); err != nil {
			return imported, err
		}
		progress.MissingFiles, err = s.markMissingFiles(libraryID, seenPaths, scanStartedAt, paths)
		if err != nil {
			return imported, err
		}
	}
	progress.Phase = "complete"
	emit()

	return imported, nil
}

func filterFilesForMediaScope(files []fileInventoryItem, mediaScope string) []fileInventoryItem {
	if mediaScope != "audio" && mediaScope != "books" {
		return files
	}
	filtered := make([]fileInventoryItem, 0, len(files))
	for _, file := range files {
		isAudio := audiometa.Supported(file.Format)
		if (mediaScope == "audio" && isAudio) || (mediaScope == "books" && !isAudio) {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

type fileInventoryItem struct {
	Path        string
	Format      string
	Size        int64
	ModTimeUnix int64
	ModTimeNS   int64
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
	return s.extractMetadataContext(context.Background(), bookID, libraryID, path)
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
	ID               int64
	BookID           int64
	Path             string
	Size             int64
	Hash             string
	HashAlgorithm    string
	LastModified     int64
	MissingAt        int64
	Title            string
	AuthorsRaw       string
	IndexingComplete bool
	HasMetadata      bool
}

func collectProcessableFiles(paths []string, shouldCancel ScanCancelFunc) ([]fileInventoryItem, error) {
	files := []fileInventoryItem{}
	seen := map[string]bool{}
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
			if !info.Mode().IsRegular() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != "" {
				ext = ext[1:]
			}
			path = filepath.Clean(path)
			if seen[path] {
				return nil
			}
			seen[path] = true
			files = append(files, fileInventoryItem{
				Path:        path,
				Format:      ext,
				Size:        info.Size(),
				ModTimeUnix: info.ModTime().Unix(),
				ModTimeNS:   info.ModTime().UnixNano(),
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
		       COALESCE(bm.title, ''), COALESCE(bm.authors, '[]'), bf.indexing_complete, bm.book_id IS NOT NULL
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
			&record.AuthorsRaw,
			&record.IndexingComplete,
			&record.HasMetadata,
		); err != nil {
			continue
		}
		record.HashAlgorithm = normalizeHashAlgorithm(record.HashAlgorithm, record.Size)
		records[record.Path] = record
	}
	return records, rows.Err()
}

func (s *Scanner) markMissingFiles(libraryID int64, seenPaths map[string]struct{}, scanStartedAt int64, roots []string) (int, error) {
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
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				missingIDs = append(missingIDs, id)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, id := range missingIDs {
		if _, err := tx.Exec(`UPDATE book_file SET missing_at=? WHERE id=? AND missing_at IS NULL`, scanStartedAt, id); err != nil {
			return 0, err
		}
	}
	if err := validateScanRoots(roots); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
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
		WHERE bm.book_id IS NULL
		   OR (
		       COALESCE(l.metadata_protection_enabled, 0) = 0
		       AND (
		           (
		               (
		                   bm.title IS NULL OR bm.title = ''
		                   OR LOWER(TRIM(bm.title)) IN ('untitled', 'unknown', 'book', 'document', 'test', 'upload')
		                   OR LOWER(TRIM(bm.title)) LIKE '%.pdf'
		                   OR LOWER(TRIM(bm.title)) LIKE '%.epub'
		                   OR LOWER(TRIM(bm.title)) LIKE '%.doc'
		                   OR LOWER(TRIM(bm.title)) LIKE '%.docx'
		               )
		               AND NOT EXISTS (
		                   SELECT 1 FROM json_each(COALESCE(bm.locked_fields, '[]'))
		                   WHERE value = 'title'
		               )
		           )
		           OR (
		               bm.cover_path IS NULL
		               AND COALESCE(bm.cover_source, '') <> 'custom'
		               AND NOT EXISTS (
		                   SELECT 1 FROM json_each(COALESCE(bm.locked_fields, '[]'))
		                   WHERE value = 'cover'
		               )
		           )
		       )
		   )
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
		if err := s.saveMetadataWithSource(e.bookID, meta, e.ownerID, "metadata_refresh"); err != nil {
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
		ModTimeNS:   info.ModTime().UnixNano(),
	}, ownerUserID, time.Now().Unix())
	return result.Imported, err
}

func (s *Scanner) processFileWithInfo(
	libraryID int64,
	file fileInventoryItem,
	ownerUserID int64,
	scanSeenAt int64,
) (processFileResult, error) {
	value := s.prepareFile(context.Background(), libraryID, file)
	if value.err != nil {
		return processFileResult{}, value.err
	}
	return s.processPreparedFile(libraryID, value, ownerUserID, scanSeenAt)
}

func (s *Scanner) processPreparedFile(libraryID int64, value preparedFile, ownerUserID, scanSeenAt int64) (processFileResult, error) {
	file, hashes := value.file, value.hashes
	if err := unchangedSinceInventory(file); err != nil {
		return processFileResult{}, err
	}
	var err error
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
		var hasMetadata bool
		if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM book_metadata WHERE book_id = ?)`, existingBookID).Scan(&hasMetadata); err != nil {
			return processFileResult{}, err
		}
		if !hasMetadata {
			meta, _ := s.extractMetadata(existingBookID, libraryID, file.Path)
			if err := s.saveMetadata(existingBookID, metadataWithFilenameTitleFallback(meta, file.Path), ownerUserID); err != nil {
				return processFileResult{}, err
			}
		}
		if repairsExtractedMetadata(file.Format) {
			if repairErr := s.repairWeakExtractedMetadata(existingBookID, file.Path, ownerUserID); repairErr != nil {
				slog.Debug("Skipped metadata repair", "path", file.Path, "error", repairErr)
			}
		}
		if repairErr := s.saveFilenameFallbackMetadataIfWeak(existingBookID, file.Path, ownerUserID); repairErr != nil {
			slog.Debug("Skipped filename metadata fallback", "path", file.Path, "error", repairErr)
		}
		if audioErr := audiometa.SyncFile(s.db, ownerUserID, existingBookID, existingFileID, file.Path, file.Format, hashes.Full); audioErr != nil {
			return processFileResult{}, fmt.Errorf("audio indexing: %w", audioErr)
		}
		s.syncFTSFromDB(existingBookID)
		if err := unchangedSinceInventory(file); err != nil {
			return processFileResult{}, err
		}
		if _, err := s.db.Exec(`UPDATE book_file SET indexing_complete = 1 WHERE id = ?`, existingFileID); err != nil {
			return processFileResult{}, err
		}

		return processFileResult{Status: status}, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return processFileResult{}, err
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
		if err := audiometa.SyncFile(s.db, ownerUserID, match.BookID, match.FileID, file.Path, file.Format, hashes.Full); err != nil {
			return processFileResult{}, err
		}
		s.syncFTSFromDB(match.BookID)
		if err := unchangedSinceInventory(file); err != nil {
			return processFileResult{}, err
		}
		if _, err := s.db.Exec(`UPDATE book_file SET indexing_complete=1 WHERE id=?`, match.FileID); err != nil {
			return processFileResult{}, err
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

	meta := value.meta
	preparedCover := value.cover
	if !value.metadataReady {
		var extractErr error
		meta, extractErr = s.extractMetadata(0, libraryID, file.Path)
		if extractErr != nil {
			slog.Warn("Metadata extraction failed; using filename fallback", "path", file.Path, "error", extractErr)
		}
		meta = metadataWithFilenameTitleFallback(meta, file.Path)
		preparedCover = prepareCover(meta, s.db)
	}

	if err := unchangedSinceInventory(file); err != nil {
		return processFileResult{}, err
	}
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return processFileResult{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		INSERT INTO book (library_id, added_at, last_scanned, owner_user_id) VALUES (?, ?, ?, ?)
	`, libraryID, now, now, ownerUserID)
	if err != nil {
		return processFileResult{}, fmt.Errorf("failed to insert book: %w", err)
	}

	bookID, err := result.LastInsertId()
	if err != nil {
		return processFileResult{}, fmt.Errorf("failed to get book ID: %w", err)
	}

	fileResult, err := tx.Exec(`
		INSERT INTO book_file (book_id, path, format, size, hash, hash_algorithm, last_modified, owner_user_id, scan_seen_at, missing_at, indexing_complete)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, 0)
	`, bookID, file.Path, file.Format, file.Size, hashes.Full, fullFileHashAlgorithm, file.ModTimeUnix, ownerUserID, scanSeenAt)
	if err != nil {
		return processFileResult{}, fmt.Errorf("failed to insert book file: %w", err)
	}

	if err := s.saveMetadataPrepared(tx, bookID, meta, ownerUserID, "scanner", preparedCover, true); err != nil {
		return processFileResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return processFileResult{}, err
	}
	s.syncFTSFromDB(bookID)

	fileID, fileIDErr := fileResult.LastInsertId()
	if fileIDErr == nil {
		if audioErr := audiometa.SyncFile(s.db, ownerUserID, bookID, fileID, file.Path, file.Format, hashes.Full); audioErr != nil {
			return processFileResult{}, fmt.Errorf("audio indexing: %w", audioErr)
		}
	}
	if err := unchangedSinceInventory(file); err != nil {
		return processFileResult{}, err
	}
	if _, err := s.db.Exec(`UPDATE book_file SET indexing_complete = 1 WHERE id = ?`, fileID); err != nil {
		return processFileResult{}, err
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
		    scan_seen_at = ?, missing_at = NULL, owner_user_id = ?, indexing_complete = 0
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
	analysis := filenameinfo.Analyze(path)
	candidateTitle := analysis.Title
	candidateAuthors := analysis.Authors
	legacy := metadata.ExtractFilename(path)
	if legacy.Series != "" {
		candidateTitle = legacy.Title
		candidateAuthors = legacy.Authors
	}
	if meta == nil {
		meta = &metadata.BookMetadata{Source: "filename"}
	}
	if meta.Authors == nil {
		meta.Authors = []string{}
	}
	if meta.Genres == nil {
		meta.Genres = []string{}
	}
	authorsWeak := filenameinfo.SuspiciousAuthors(meta.Authors, true) ||
		(len(meta.Authors) == 1 && strings.EqualFold(strings.TrimSpace(meta.Authors[0]), strings.TrimSpace(meta.Title)))
	titleIncludesAuthor := authorsWeak && analysis.Pattern != "filename title" &&
		strings.EqualFold(strings.TrimSpace(meta.Title), filenameinfo.Stem(path))
	if analysis.Confidence != "ambiguous" {
		if (filenameinfo.SuspiciousTitle(meta.Title) || titleIncludesAuthor) && candidateTitle != "" {
			meta.Title = candidateTitle
		}
		if authorsWeak && len(candidateAuthors) > 0 {
			meta.Authors = append([]string{}, candidateAuthors...)
		}
	} else if meta.Source == "filename" {
		// Keep the usable title, but do not automatically store an ambiguous author list.
		meta.Title = analysis.Title
		meta.Authors = []string{}
	}
	return meta
}

func shouldUseFilenameMetadata(title, authorsRaw string) bool {
	var authors []string
	valid := json.Unmarshal([]byte(authorsRaw), &authors) == nil
	return filenameinfo.SuspiciousTitle(title) || filenameinfo.SuspiciousAuthors(authors, valid) ||
		(len(authors) == 1 && strings.EqualFold(strings.TrimSpace(authors[0]), strings.TrimSpace(title)))
}

func (s *Scanner) saveFilenameFallbackMetadataIfWeak(bookID int64, path string, ownerUserID int64) error {
	var title, authorsRaw, lockedRaw string
	err := s.db.QueryRow(`
		SELECT COALESCE(title, ''), COALESCE(authors, '[]'), COALESCE(locked_fields, '[]')
		FROM book_metadata WHERE book_id = ?
	`, bookID).Scan(&title, &authorsRaw, &lockedRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	var authors []string
	validAuthors := json.Unmarshal([]byte(authorsRaw), &authors) == nil
	authorsWeak := filenameinfo.SuspiciousAuthors(authors, validAuthors) ||
		(len(authors) == 1 && strings.EqualFold(strings.TrimSpace(authors[0]), strings.TrimSpace(title)))
	titleWeak := filenameinfo.SuspiciousTitle(title)
	analysis := filenameinfo.Analyze(path)
	if analysis.Confidence == "ambiguous" {
		return nil
	}
	titleIncludesAuthor := authorsWeak && analysis.Pattern != "filename title" &&
		strings.EqualFold(strings.TrimSpace(title), filenameinfo.Stem(path))
	locked := metaprotection.ParseLocked(lockedRaw)
	titleEdited, authorsEdited, err := s.filenameFieldsEditedByUser(bookID)
	if err != nil {
		return err
	}
	incoming := &metadata.BookMetadata{Authors: []string{}, Genres: []string{}, Source: "filename"}
	if (titleWeak || titleIncludesAuthor) && !locked[metaprotection.FieldTitle] && !titleEdited {
		incoming.Title = analysis.Title
	}
	if authorsWeak && len(analysis.Authors) > 0 && !locked[metaprotection.FieldAuthors] && !authorsEdited {
		incoming.Authors = append([]string{}, analysis.Authors...)
	}
	if incoming.Title == "" && len(incoming.Authors) == 0 {
		return nil
	}
	return s.saveMetadataWithSource(bookID, incoming, ownerUserID, "filename_fallback")
}

// saveMetadata upserts book metadata and saves the cover image to disk
func (s *Scanner) saveMetadata(bookID int64, meta *metadata.BookMetadata, ownerUserID int64) error {
	return s.saveMetadataWithSource(bookID, meta, ownerUserID, "scanner")
}

func (s *Scanner) saveMetadataWithSource(bookID int64, meta *metadata.BookMetadata, ownerUserID int64, source string) error {
	return s.saveMetadataInTransaction(nil, bookID, meta, ownerUserID, source)
}

func (s *Scanner) saveMetadataInTransaction(tx *sql.Tx, bookID int64, meta *metadata.BookMetadata, ownerUserID int64, source string) error {
	return s.saveMetadataPrepared(tx, bookID, meta, ownerUserID, source, nil, false)
}

func (s *Scanner) saveMetadataPrepared(tx *sql.Tx, bookID int64, meta *metadata.BookMetadata, ownerUserID int64, source string, processedCover []byte, coverReady bool) error {
	if meta == nil {
		return nil
	}

	if !coverReady {
		processedCover = prepareCover(meta, s.db)
	}
	authorsJSON, _ := json.Marshal(meta.Authors)
	tagsJSON, _ := json.Marshal(meta.Genres)
	ownTransaction := tx == nil
	var err error
	if ownTransaction {
		tx, err = s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	current, exists, err := metaprotection.LoadSnapshot(tx, bookID)
	if err != nil {
		return err
	}
	previous := current
	locked := metaprotection.ParseLocked(current.LockedFields)
	libraryProtected := false
	if exists {
		libraryProtected, err = metaprotection.LibraryProtectionEnabled(tx, bookID)
		if err != nil {
			return err
		}
		if libraryProtected {
			for _, field := range metaprotection.AllFields {
				locked[field] = true
			}
		}
	}
	changed := []string{}

	applyString := func(field, incoming string, target *string) {
		if incoming == "" || locked[field] || *target == incoming {
			return
		}
		*target = incoming
		changed = append(changed, field)
	}
	applyFloat := func(field string, incoming float64, target *float64) {
		if incoming == 0 || locked[field] || *target == incoming {
			return
		}
		*target = incoming
		changed = append(changed, field)
	}
	applyInt := func(field string, incoming int, target *int64) {
		if incoming == 0 || locked[field] || *target == int64(incoming) {
			return
		}
		*target = int64(incoming)
		changed = append(changed, field)
	}

	applyString(metaprotection.FieldTitle, strings.TrimSpace(meta.Title), &current.Title)
	if len(meta.Authors) > 0 {
		applyString(metaprotection.FieldAuthors, string(authorsJSON), &current.Authors)
	}
	applyString(metaprotection.FieldSeries, strings.TrimSpace(meta.Series), &current.Series)
	if meta.SeriesNumber != 0 && !locked[metaprotection.FieldSeriesNumber] &&
		(current.SeriesNumber != meta.SeriesNumber || current.SeriesNumberDisplay != meta.SeriesNumberDisplay) {
		current.SeriesNumber = meta.SeriesNumber
		current.SeriesNumberDisplay = meta.SeriesNumberDisplay
		changed = append(changed, metaprotection.FieldSeriesNumber)
	}
	applyString(metaprotection.FieldPublisher, strings.TrimSpace(meta.Publisher), &current.Publisher)
	applyString(metaprotection.FieldPubDate, strings.TrimSpace(meta.PubDate), &current.PubDate)
	applyString(metaprotection.FieldDescription, strings.TrimSpace(meta.Description), &current.Description)
	applyFloat(metaprotection.FieldRating, meta.Rating, &current.Rating)
	if len(meta.Genres) > 0 {
		applyString(metaprotection.FieldTags, string(tagsJSON), &current.Tags)
	}
	applyString(metaprotection.FieldISBN, strings.TrimSpace(meta.ISBN), &current.ISBN)
	applyString(metaprotection.FieldASIN, strings.TrimSpace(meta.ASIN), &current.ASIN)
	applyString(metaprotection.FieldLanguage, strings.TrimSpace(meta.Language), &current.Language)
	applyInt(metaprotection.FieldPageCount, meta.PageCount, &current.PageCount)

	newCoverPath := ""
	if len(processedCover) > 0 && !locked[metaprotection.FieldCover] && current.CoverSource != "custom" {
		if savedPath, saveErr := covers.SaveCoverBytes(s.coversPath, bookID, processedCover); saveErr == nil {
			newCoverPath = savedPath
			if current.CoverPath != savedPath || current.CoverSource != "" {
				current.CoverPath = savedPath
				current.CoverSource = ""
				current.CoverUpdatedOn = time.Now().Unix()
				changed = append(changed, metaprotection.FieldCover)
			}
		}
	}

	var extractedFromHash string
	_ = tx.QueryRow(`
		SELECT COALESCE(hash, '')
		FROM book_file
		WHERE book_id = ?
		ORDER BY CASE WHEN missing_at IS NULL THEN 0 ELSE 1 END, id
		LIMIT 1
	`, bookID).Scan(&extractedFromHash)
	now := time.Now().Unix()
	if !exists {
		current.BookID = bookID
		current.Genres = "[]"
		current.LockedFields = "[]"
		current.OwnerUserID = ownerUserID
	}
	if !exists || !libraryProtected {
		current.ExtractedFromHash = extractedFromHash
	}
	current.OwnerUserID = ownerUserID
	if len(changed) > 0 || !exists {
		current.MetadataUpdatedAt = now
	}

	if exists && len(changed) > 0 {
		if err := metaprotection.RecordRevision(tx, previous, changed, source, 0); err != nil {
			if newCoverPath != "" && newCoverPath != previous.CoverPath {
				_ = os.Remove(newCoverPath)
			}
			return err
		}
	}

	_, err = tx.Exec(`
		INSERT INTO book_metadata
		    (book_id, title, authors, series, series_number, series_number_display, publisher, pub_date,
		     description, rating, genres, tags, isbn, asin, language, page_count, cover_path, cover_source,
		     cover_updated_on, locked_fields, extracted_from_hash, metadata_updated_at, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
		    title = excluded.title,
		    authors = excluded.authors,
		    series = excluded.series,
		    series_number = excluded.series_number,
		    series_number_display = excluded.series_number_display,
		    publisher = excluded.publisher,
		    pub_date = excluded.pub_date,
		    description = excluded.description,
		    rating = excluded.rating,
		    genres = excluded.genres,
		    tags = excluded.tags,
		    isbn = excluded.isbn,
		    asin = excluded.asin,
		    language = excluded.language,
		    page_count = excluded.page_count,
		    cover_path = excluded.cover_path,
		    cover_source = excluded.cover_source,
		    cover_updated_on = excluded.cover_updated_on,
		    locked_fields = excluded.locked_fields,
		    extracted_from_hash = excluded.extracted_from_hash,
		    metadata_updated_at = excluded.metadata_updated_at,
		    owner_user_id = excluded.owner_user_id
	`, bookID, current.Title, current.Authors, current.Series, current.SeriesNumber, current.SeriesNumberDisplay,
		current.Publisher, current.PubDate, current.Description, current.Rating, current.Genres, current.Tags,
		current.ISBN, current.ASIN, current.Language, current.PageCount, current.CoverPath, current.CoverSource,
		current.CoverUpdatedOn, current.LockedFields, current.ExtractedFromHash, current.MetadataUpdatedAt, current.OwnerUserID)
	if err != nil {
		if newCoverPath != "" && newCoverPath != previous.CoverPath {
			_ = os.Remove(newCoverPath)
		}
		return err
	}

	if !ownTransaction {
		return nil
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if newCoverPath != "" && previous.CoverPath != "" && previous.CoverPath != newCoverPath {
		_ = os.Remove(previous.CoverPath)
	}

	s.syncFTSFromDB(bookID)

	return nil
}

func (s *Scanner) repairWeakExtractedMetadata(bookID int64, path string, ownerUserID int64) error {
	var title, authorsRaw, publisher, pubDate, coverPath, coverSource, lockedFieldsRaw string
	var pageCount int
	var libraryID int64
	err := s.db.QueryRow(`
		SELECT COALESCE(title, ''), COALESCE(authors, '[]'), COALESCE(publisher, ''),
		       COALESCE(pub_date, ''), COALESCE(page_count, 0), COALESCE(cover_path, ''),
		       COALESCE(cover_source, ''), COALESCE(locked_fields, '[]'), b.library_id
		FROM book b
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.id = ?
	`, bookID).Scan(
		&title,
		&authorsRaw,
		&publisher,
		&pubDate,
		&pageCount,
		&coverPath,
		&coverSource,
		&lockedFieldsRaw,
		&libraryID,
	)
	if err != nil {
		return err
	}

	libraryProtected, err := metaprotection.LibraryProtectionEnabled(s.db, bookID)
	if err != nil {
		return err
	}
	if libraryProtected {
		return nil
	}

	locked := metaprotection.ParseLocked(lockedFieldsRaw)
	generated := metadata.ExtractFilename(path)
	oldFilenameShape := strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(generated.Title)) &&
		sameStringList(authorsRaw, generated.Authors) &&
		(!locked[metaprotection.FieldTitle] || !locked[metaprotection.FieldAuthors])
	missingUsefulFields :=
		(publisher == "" && !locked[metaprotection.FieldPublisher]) ||
			(pubDate == "" && !locked[metaprotection.FieldPubDate]) ||
			(pageCount == 0 && !locked[metaprotection.FieldPageCount]) ||
			(coverPath == "" && coverSource != "custom" && !locked[metaprotection.FieldCover])
	if !oldFilenameShape && !missingUsefulFields {
		return nil
	}

	extracted, err := s.extractMetadata(bookID, libraryID, path)
	if err != nil || extracted == nil || extracted.Source == "filename" {
		return err
	}

	extracted = metadataWithFilenameTitleFallback(extracted, path)
	if err := s.saveMetadataWithSource(bookID, extracted, ownerUserID, "scan_repair"); err != nil {
		return err
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

var userFilenameEditSources = map[string]bool{
	"manual_edit":      true,
	"bulk_edit":        true,
	"provider_apply":   true,
	"revision_restore": true,
}

// RepairSwappedFilenameMetadata rewrites title/author pairs that still match the
// historical "Author - Title" filename parse. User-edited values, explicit field
// locks, and metadata that no longer matches that parse are left unchanged.
func (s *Scanner) RepairSwappedFilenameMetadata() (int, error) {
	rows, err := s.db.Query(`
		SELECT b.id, COALESCE(b.owner_user_id, 1), bf.path,
		       COALESCE(bm.title, ''), COALESCE(bm.authors, '[]'),
		       COALESCE(bm.locked_fields, '[]')
		FROM book b
		JOIN book_metadata bm ON bm.book_id = b.id
		JOIN book_file bf ON bf.id = (
			SELECT bf2.id
			FROM book_file bf2
			WHERE bf2.book_id = b.id AND bf2.missing_at IS NULL
			ORDER BY bf2.id
			LIMIT 1
		)
		WHERE bf.path LIKE '% - %'
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type entry struct {
		bookID      int64
		ownerUserID int64
		path        string
		title       string
		authorsRaw  string
		lockedRaw   string
	}
	var entries []entry
	for rows.Next() {
		var item entry
		if err := rows.Scan(&item.bookID, &item.ownerUserID, &item.path, &item.title, &item.authorsRaw, &item.lockedRaw); err != nil {
			continue
		}
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	repaired := 0
	for _, item := range entries {
		changed, err := s.repairSwappedFilenameMetadata(item.bookID, item.ownerUserID, item.path, item.title, item.authorsRaw, item.lockedRaw)
		if err != nil {
			slog.Warn("Failed to repair swapped filename metadata", "bookID", item.bookID, "path", item.path, "error", err)
			continue
		}
		if changed {
			repaired++
		}
	}
	return repaired, nil
}

func (s *Scanner) repairSwappedFilenameMetadata(
	bookID, ownerUserID int64,
	path, title, authorsRaw, lockedRaw string,
) (bool, error) {
	var authors []string
	if err := json.Unmarshal([]byte(authorsRaw), &authors); err != nil {
		authors = []string{}
	}

	newTitle, newAuthors, updateTitle, updateAuthors := metadata.FilenameOrderCorrection(path, title, authors)
	if !updateTitle && !updateAuthors {
		return false, nil
	}

	locked := metaprotection.ParseLocked(lockedRaw)
	if locked[metaprotection.FieldTitle] {
		updateTitle = false
	}
	if locked[metaprotection.FieldAuthors] {
		updateAuthors = false
	}

	titleEdited, authorsEdited, err := s.filenameFieldsEditedByUser(bookID)
	if err != nil {
		return false, err
	}
	if titleEdited {
		updateTitle = false
	}
	if authorsEdited {
		updateAuthors = false
	}
	if !updateTitle && !updateAuthors {
		return false, nil
	}

	if err := s.applyFilenameOrderCorrection(bookID, ownerUserID, newTitle, newAuthors, updateTitle, updateAuthors); err != nil {
		return false, err
	}
	slog.Info("Repaired swapped filename metadata", "bookID", bookID, "path", path, "title", updateTitle, "authors", updateAuthors)
	return true, nil
}

func (s *Scanner) filenameFieldsEditedByUser(bookID int64) (bool, bool, error) {
	rows, err := s.db.Query(`
		SELECT change_source, changed_fields
		FROM book_metadata_revision
		WHERE book_id = ?
	`, bookID)
	if err != nil {
		return false, false, err
	}
	defer rows.Close()

	titleEdited := false
	authorsEdited := false
	for rows.Next() {
		var source, fieldsRaw string
		if err := rows.Scan(&source, &fieldsRaw); err != nil {
			continue
		}
		if !userFilenameEditSources[strings.TrimSpace(source)] {
			continue
		}
		var fields []string
		if err := json.Unmarshal([]byte(fieldsRaw), &fields); err != nil {
			continue
		}
		for _, field := range metaprotection.NormalizeFields(fields) {
			if field == metaprotection.FieldTitle {
				titleEdited = true
			}
			if field == metaprotection.FieldAuthors {
				authorsEdited = true
			}
		}
	}
	return titleEdited, authorsEdited, rows.Err()
}

func (s *Scanner) applyFilenameOrderCorrection(
	bookID, ownerUserID int64,
	title string,
	authors []string,
	updateTitle, updateAuthors bool,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	current, exists, err := metaprotection.LoadSnapshot(tx, bookID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	previous := current
	changed := []string{}
	if updateTitle {
		current.Title = title
		changed = append(changed, metaprotection.FieldTitle)
	}
	if updateAuthors {
		authorsJSON, err := json.Marshal(authors)
		if err != nil {
			return err
		}
		current.Authors = string(authorsJSON)
		changed = append(changed, metaprotection.FieldAuthors)
	}
	if len(changed) == 0 {
		return nil
	}

	if err := metaprotection.RecordRevision(tx, previous, changed, "filename_order_repair", 0); err != nil {
		return err
	}

	now := time.Now().Unix()
	if _, err := tx.Exec(`
		UPDATE book_metadata
		SET title = ?, authors = ?, metadata_updated_at = ?, owner_user_id = ?
		WHERE book_id = ?
	`, current.Title, current.Authors, now, ownerUserID, bookID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.syncFTSFromDB(bookID)
	return nil
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
	return computeFileHashesContext(context.Background(), path)
}

func computeFileHashesContext(ctx context.Context, path string) (computedFileHashes, error) {
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
		if _, err := io.Copy(fullHash, &contextReader{ctx: ctx, reader: file}); err != nil {
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
		if err := ctx.Err(); err != nil {
			return computedFileHashes{}, err
		}
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

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
