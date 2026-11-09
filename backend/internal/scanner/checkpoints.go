package scanner

import (
	"errors"
	"fmt"
	"os"
)

// JobID zero is for synchronous callers/tests. File-only imports never reconcile.
type ScanOptions struct {
	JobID     int64
	Reconcile bool
}

func validateScanRoots(roots []string) error {
	if len(roots) == 0 {
		return errors.New("no scan paths configured")
	}
	for _, root := range roots {
		f, err := os.Open(root)
		if err != nil {
			return fmt.Errorf("library storage unavailable at %s: %w", root, err)
		}
		_, err = f.Stat()
		f.Close()
		if err != nil {
			return fmt.Errorf("library storage unavailable at %s: %w", root, err)
		}
	}
	return nil
}

type checkpointRecord struct {
	Status           string
	Size, ModifiedNS int64
}

func (s *Scanner) loadCheckpoints(jobID int64) (map[string]checkpointRecord, error) {
	result := map[string]checkpointRecord{}
	if jobID == 0 {
		return result, nil
	}
	rows, err := s.db.Query(`SELECT path,status,size,modified_ns FROM scan_file_checkpoint WHERE job_id=?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var value checkpointRecord
		if err := rows.Scan(&path, &value.Status, &value.Size, &value.ModifiedNS); err != nil {
			return nil, err
		}
		result[path] = value
	}
	return result, rows.Err()
}

func (s *Scanner) saveCheckpoint(jobID int64, file fileInventoryItem, status, message string) error {
	if jobID == 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO scan_file_checkpoint(job_id,path,size,modified_ns,status,error) VALUES(?,?,?,?,?,?) ON CONFLICT(job_id,path) DO UPDATE SET size=excluded.size,modified_ns=excluded.modified_ns,status=excluded.status,error=excluded.error`, jobID, file.Path, file.Size, file.ModTimeNS, status, message)
	return err
}

func countScanStatus(progress *ScanProgress, status string) {
	switch status {
	case "unchanged":
		progress.UnchangedFiles++
	case scanStatusChanged:
		progress.ChangedFiles++
	case scanStatusRelinked, scanStatusMovedLibrary, scanStatusRestored:
		progress.RelinkedFiles++
	case scanStatusDuplicate:
		progress.DuplicateFiles++
	}
}

func unchangedSinceInventory(file fileInventoryItem) error {
	info, err := os.Stat(file.Path)
	if err != nil {
		return err
	}
	if info.Size() != file.Size || (file.ModTimeNS != 0 && info.ModTime().UnixNano() != file.ModTimeNS) {
		return fmt.Errorf("file changed during scanning; retry after the file has finished syncing: %s", file.Path)
	}
	return nil
}
