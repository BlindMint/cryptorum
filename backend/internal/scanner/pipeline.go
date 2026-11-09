package scanner

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cryptorum/internal/covers"
	"cryptorum/internal/metadata"
)

const DefaultPreparationWorkers = 2
const MaxPreparationWorkers = 4

type preparedFile struct {
	file          fileInventoryItem
	hashes        computedFileHashes
	meta          *metadata.BookMetadata
	cover         []byte
	metadataReady bool
	fastStatus    string
	checkpointed  bool
	err           error
	timings       ScanTimings
}

// Durations are cumulative stage work (parallel work may exceed elapsed time).
type ScanTimings struct {
	Workers      int     `json:"preparation_workers"`
	InventoryMS  float64 `json:"inventory_ms"`
	HashMS       float64 `json:"hash_ms"`
	MetadataMS   float64 `json:"metadata_ms"`
	CoverMS      float64 `json:"cover_ms"`
	CommitMS     float64 `json:"commit_ms"`
	CheckpointMS float64 `json:"checkpoint_ms"`
	ElapsedMS    float64 `json:"elapsed_ms"`
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}

// At most workers files are dispatched, including completed results awaiting
// the serial writer. Results commit in inventory order for deterministic relinking.
// Stop requests discard uncommitted preparations; completed commits are retained.
func runPreparationPipeline(parent context.Context, workers int, files []fileInventoryItem, prepare func(context.Context, fileInventoryItem) preparedFile, consume func(preparedFile) error, shouldStop ScanCancelFunc) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	type work struct {
		index int
		file  fileInventoryItem
	}
	type result struct {
		index    int
		prepared preparedFile
	}
	jobs := make(chan work, workers)
	results := make(chan result, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-jobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}
					value := prepare(ctx, item.file)
					select {
					case results <- result{item.index, value}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	defer func() { cancel(); close(jobs); group.Wait() }()
	next, committed := 0, 0
	pending := make(map[int]preparedFile, workers)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	stopped := func() bool { return ctx.Err() != nil || (shouldStop != nil && shouldStop()) }
	for committed < len(files) {
		if stopped() {
			return ErrScanCancelled
		}
		for next < len(files) && next-committed < workers {
			if stopped() {
				return ErrScanCancelled
			}
			jobs <- work{next, files[next]}
			next++
		}
		if value, ok := pending[committed]; ok {
			if stopped() {
				return ErrScanCancelled
			}
			if err := consume(value); err != nil {
				return err
			}
			delete(pending, committed)
			committed++
			continue
		}
		select {
		case item := <-results:
			pending[item.index] = item.prepared
		case <-ticker.C:
		case <-ctx.Done():
			return ErrScanCancelled
		}
	}
	return nil
}

func (s *Scanner) prepareFile(ctx context.Context, libraryID int64, file fileInventoryItem) preparedFile {
	value := preparedFile{file: file}
	start := time.Now()
	value.hashes, value.err = computeFileHashesContext(ctx, file.Path)
	value.timings.HashMS = elapsedMS(start)
	if value.err != nil {
		return value
	}
	if value.err = unchangedSinceInventory(file); value.err != nil {
		return value
	}
	// This is only a read-side optimization. The writer always repeats matching
	// after earlier files have committed and before making any catalog changes.
	var exists bool
	value.err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM book_file WHERE path=?)`, file.Path).Scan(&exists)
	if value.err != nil || exists {
		return value
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.library_id,bf.path,COALESCE(bf.missing_at,0) FROM book_file bf JOIN book b ON b.id=bf.book_id WHERE bf.size=? AND bf.hash IN (?,?)`, file.Size, value.hashes.Full, value.hashes.Legacy)
	if err != nil {
		value.err = err
		return value
	}
	skipMetadata := false
	for rows.Next() {
		var id, missing int64
		var path string
		if err = rows.Scan(&id, &path, &missing); err != nil {
			break
		}
		if id == libraryID || missing > 0 || !pathLikelyExists(path) {
			skipMetadata = true
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		value.err = err
		return value
	}
	if skipMetadata {
		return value
	}
	start = time.Now()
	meta, err := s.extractMetadataContext(ctx, 0, libraryID, file.Path)
	value.timings.MetadataMS = elapsedMS(start)
	if ctx.Err() != nil {
		value.err = ctx.Err()
		return value
	}
	if err != nil {
		slog.Warn("Metadata extraction failed; using filename fallback", "path", file.Path, "error", err)
	}
	value.meta = metadataWithFilenameTitleFallback(meta, file.Path)
	value.metadataReady = true
	start = time.Now()
	value.cover = prepareCover(value.meta, s.db)
	value.meta.CoverData = nil
	value.timings.CoverMS = elapsedMS(start)
	value.err = unchangedSinceInventory(file)
	return value
}

func heavyExtraction(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cbz", ".cbr", ".cb7", ".pdf", ".mobi", ".azw3", ".mp3", ".m4a", ".m4b", ".flac", ".ogg", ".wav":
		return true
	default:
		return false
	}
}

func (s *Scanner) extractMetadataContext(ctx context.Context, bookID, libraryID int64, path string) (*metadata.BookMetadata, error) {
	return s.withExtractionSlot(ctx, path, func() (*metadata.BookMetadata, error) {
		return metadata.ExtractWithOptions(path, metadata.ExtractOptions{ComicSpreadFallbackSide: s.resolveComicSpreadFallback(bookID, libraryID)})
	})
}

func (s *Scanner) withExtractionSlot(ctx context.Context, path string, extract func() (*metadata.BookMetadata, error)) (*metadata.BookMetadata, error) {
	if heavyExtraction(path) {
		select {
		case s.extractionSlot <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		defer func() { <-s.extractionSlot }()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return extract()
}

// Used outside write transactions, including the serial metadata repair path.
func prepareCover(meta *metadata.BookMetadata, db *sql.DB) []byte {
	if meta == nil || len(meta.CoverData) == 0 {
		return nil
	}
	processed, err := covers.ProcessCover(meta.CoverData, covers.LoadSettings(db))
	if err != nil {
		return nil
	}
	return processed
}
