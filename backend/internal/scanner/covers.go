package scanner

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"cryptorum/internal/covers"
	"cryptorum/internal/metadata"
)

type coverCandidate struct {
	bookID            int64
	filePath          string
	existingCoverPath string
}

// CoverProgressFunc reports cover regeneration progress.
type CoverProgressFunc func(processed, updated, failed, total int)

// RegenerateCovers rebuilds stored covers from the source book files.
func (s *Scanner) RegenerateCovers(missingOnly bool, progress CoverProgressFunc) (int, int, error) {
	return s.RegenerateCoversForLibrary(0, missingOnly, progress)
}

// RegenerateCoversForLibrary rebuilds stored covers for all libraries or one library.
func (s *Scanner) RegenerateCoversForLibrary(libraryID int64, missingOnly bool, progress CoverProgressFunc) (int, int, error) {
	candidates, err := s.coverCandidates(libraryID, missingOnly)
	if err != nil {
		return 0, 0, err
	}

	settings := covers.LoadSettings(s.db)
	updated := 0
	failed := 0
	total := len(candidates)

	for i, candidate := range candidates {
		meta, err := metadata.Extract(candidate.filePath)
		if err != nil || meta == nil || len(meta.CoverData) == 0 {
			failed++
			if progress != nil {
				progress(i+1, updated, failed, total)
			}
			continue
		}

		processed, err := covers.ProcessCover(meta.CoverData, settings)
		if err != nil || len(processed) == 0 {
			failed++
			if progress != nil {
				progress(i+1, updated, failed, total)
			}
			continue
		}

		newCoverPath, err := covers.SaveCoverBytes(s.coversPath, candidate.bookID, processed)
		if err != nil || newCoverPath == "" {
			failed++
			if progress != nil {
				progress(i+1, updated, failed, total)
			}
			continue
		}

		if err := s.updateCoverRecord(candidate.bookID, candidate.existingCoverPath, newCoverPath); err != nil {
			slog.Warn("Failed to update cover record", "bookID", candidate.bookID, "error", err)
			failed++
			if progress != nil {
				progress(i+1, updated, failed, total)
			}
			continue
		}

		updated++
		if progress != nil {
			progress(i+1, updated, failed, total)
		}
	}

	return updated, failed, nil
}

// CountCoverCandidates returns the number of book files a cover regeneration job will inspect.
func (s *Scanner) CountCoverCandidates(missingOnly bool) (int, error) {
	return s.CountCoverCandidatesForLibrary(0, missingOnly)
}

// CountCoverCandidatesForLibrary returns the number of book files a cover regeneration job will inspect.
func (s *Scanner) CountCoverCandidatesForLibrary(libraryID int64, missingOnly bool) (int, error) {
	candidates, err := s.coverCandidates(libraryID, missingOnly)
	if err != nil {
		return 0, err
	}
	return len(candidates), nil
}

func (s *Scanner) coverCandidates(libraryID int64, missingOnly bool) ([]coverCandidate, error) {
	query := `
		SELECT b.id, bf.path, COALESCE(bm.cover_path, '')
		FROM book b
		JOIN book_file bf ON b.id = bf.book_id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
	`
	args := []any{}
	if libraryID > 0 {
		query += ` WHERE b.library_id = ?`
		args = append(args, libraryID)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := []coverCandidate{}

	for rows.Next() {
		var candidate coverCandidate
		if err := rows.Scan(&candidate.bookID, &candidate.filePath, &candidate.existingCoverPath); err != nil {
			continue
		}

		if missingOnly && coverPathExists(candidate.existingCoverPath) {
			continue
		}

		candidates = append(candidates, candidate)
	}

	return candidates, rows.Err()
}

func coverPathExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (s *Scanner) updateCoverRecord(bookID int64, oldCoverPath, newCoverPath string) error {
	now := time.Now().Unix()

	if _, err := s.db.Exec(`
		INSERT INTO book_metadata (book_id, cover_path, cover_updated_on, authors, genres, locked_fields)
		VALUES (?, ?, ?, '[]', '[]', '[]')
		ON CONFLICT(book_id) DO UPDATE SET
			cover_path = excluded.cover_path,
			cover_updated_on = excluded.cover_updated_on
	`, bookID, newCoverPath, now); err != nil {
		return err
	}

	if oldCoverPath != "" && oldCoverPath != newCoverPath {
		_ = os.Remove(oldCoverPath)
	}

	return nil
}

// saveCover preserves the scanner's existing call site but routes through the shared cover helper.
func (s *Scanner) saveCover(bookID int64, data []byte) string {
	path, err := covers.SaveCoverBytes(s.coversPath, bookID, data)
	if err != nil {
		slog.Warn("Failed to save cover", "bookID", bookID, "error", err)
		return ""
	}
	return path
}

func removeCoverVariants(coversPath string, bookID int64, keepPath string) {
	extensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	for _, ext := range extensions {
		candidate := filepath.Join(coversPath, fmt.Sprintf("%d%s", bookID, ext))
		if candidate == keepPath {
			continue
		}
		_ = os.Remove(candidate)
	}
}
