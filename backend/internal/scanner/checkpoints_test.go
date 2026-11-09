package scanner

import (
	"cryptorum/internal/db"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPartialImportAndIncompleteInventoryNeverReconcile(t *testing.T) {
	for _, mode := range []string{"partial", "incomplete inventory", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			db := setupScannerTestDB(t)
			s := New(db.DB, t.TempDir(), t.TempDir())
			root := t.TempDir()
			gone := filepath.Join(root, "gone.txt")
			writeTestFile(t, gone, []byte("existing"))
			if _, err := s.ScanLibrary(1, []string{root}); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(gone); err != nil {
				t.Fatal(err)
			}
			next := filepath.Join(root, "next.txt")
			writeTestFile(t, next, []byte("next"))
			paths := []string{root}
			options := ScanOptions{Reconcile: true}
			var stop ScanCancelFunc
			switch mode {
			case "partial":
				paths = []string{next}
				options.Reconcile = false
			case "incomplete inventory":
				paths = append(paths, filepath.Join(root, "unmounted"))
			case "cancelled":
				stop = func() bool { return true }
			}
			_, err := s.ScanLibraryWithOptions(1, paths, nil, stop, options)
			if mode != "partial" && err == nil {
				t.Fatal("expected interrupted/incomplete scan")
			}
			var missing bool
			if err := db.QueryRow(`SELECT missing_at IS NOT NULL FROM book_file WHERE path=?`, gone).Scan(&missing); err != nil {
				t.Fatal(err)
			}
			if missing {
				t.Fatal("partial/interrupted inventory marked unrelated file missing")
			}
		})
	}
}

func TestCheckpointResumeAndChangedFiles(t *testing.T) {
	db := setupScannerTestDB(t)
	s := New(db.DB, t.TempDir(), t.TempDir())
	mustScannerExec(t, db.DB, `INSERT INTO metadata_job(id,job_type,title,status,created_at) VALUES(1,'library_scan','Scan','running',1)`)
	root := t.TempDir()
	first := filepath.Join(root, "a.txt")
	second := filepath.Join(root, "b.txt")
	writeTestFile(t, first, []byte("first"))
	writeTestFile(t, second, []byte("other"))
	stamp := time.Unix(1700000000, 100)
	if err := os.Chtimes(first, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	stopped := false
	_, err := s.ScanLibraryWithOptions(1, []string{root}, func(p ScanProgress) {
		if p.ScannedFiles == 1 {
			stopped = true
		}
	}, func() bool { return stopped }, ScanOptions{JobID: 1, Reconcile: true})
	if !errors.Is(err, ErrScanCancelled) {
		t.Fatalf("got %v", err)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM scan_file_checkpoint WHERE job_id=1`).Scan(&count)
	if count != 1 {
		t.Fatalf("checkpoints %d", count)
	}
	// A new Scanner instance simulates losing all worker state on restart.
	s = New(db.DB, t.TempDir(), t.TempDir())
	var final ScanProgress
	if _, err := s.ScanLibraryWithOptions(1, []string{root}, func(p ScanProgress) { final = p }, nil, ScanOptions{JobID: 1, Reconcile: true}); err != nil {
		t.Fatal(err)
	}
	if final.ScannedFiles != 2 || final.ImportedBooks != 2 {
		t.Fatalf("resume totals %+v", final)
	}
	db.QueryRow(`SELECT count(*) FROM book`).Scan(&count)
	if count != 2 {
		t.Fatalf("duplicate imports: %d", count)
	}
	// Same size and same Unix second, but different nanoseconds: must rehash.
	writeTestFile(t, first, []byte("newer"))
	changed := time.Unix(stamp.Unix(), 200)
	if err := os.Chtimes(first, changed, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ScanLibraryWithOptions(1, []string{root}, func(p ScanProgress) { final = p }, nil, ScanOptions{JobID: 1, Reconcile: true}); err != nil {
		t.Fatal(err)
	}
	var hash string
	db.QueryRow(`SELECT hash FROM book_file WHERE path=?`, first).Scan(&hash)
	expected, err := computeFileHash(first)
	if err != nil || hash != expected {
		t.Fatalf("changed file not rehashed: %s != %s (%v)", hash, expected, err)
	}
}

func TestImportTransactionRollsBackAndCanRetry(t *testing.T) {
	db := setupScannerTestDB(t)
	s := New(db.DB, t.TempDir(), t.TempDir())
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "book.txt"), []byte("atomic import"))
	mustScannerExec(t, db.DB, `CREATE TRIGGER fail_metadata BEFORE INSERT ON book_metadata BEGIN SELECT RAISE(ABORT,'injected metadata failure'); END`)
	if _, err := s.ScanLibrary(1, []string{root}); err == nil {
		t.Fatal("expected injected failure")
	}
	for _, table := range []string{"book", "book_file", "book_metadata"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial %s records: %d", table, count)
		}
	}
	mustScannerExec(t, db.DB, `DROP TRIGGER fail_metadata`)
	if imported, err := s.ScanLibrary(1, []string{root}); err != nil || imported != 1 {
		t.Fatalf("retry %d %v", imported, err)
	}
}

func TestFullScanReconcilesOnlyAfterCompletion(t *testing.T) {
	db := setupScannerTestDB(t)
	s := New(db.DB, t.TempDir(), t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "gone.txt")
	writeTestFile(t, path, []byte("removed"))
	if _, err := s.ScanLibrary(1, []string{root}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ScanLibrary(1, []string{root}); err != nil {
		t.Fatal(err)
	}
	var missing bool
	db.QueryRow(`SELECT missing_at IS NOT NULL FROM book_file WHERE path=?`, path).Scan(&missing)
	if !missing {
		t.Fatal("completed full scan did not reconcile missing file")
	}
}

// Exit without defers to exercise persisted checkpoints after a process dies.
func TestScanCrashRecoverySubprocess(t *testing.T) {
	if dataPath := os.Getenv("CRYPTORUM_SCAN_CRASH_TEST_DATA"); dataPath != "" {
		store, err := db.New(dataPath)
		if err != nil {
			os.Exit(90)
		}
		s := New(store.DB, dataPath, filepath.Join(dataPath, "covers"))
		_, _ = s.ScanLibraryWithOptions(1, []string{os.Getenv("CRYPTORUM_SCAN_CRASH_TEST_ROOT")}, func(p ScanProgress) {
			if p.ScannedFiles == 1 {
				os.Exit(23)
			}
		}, nil, ScanOptions{JobID: 1, Reconcile: true})
		os.Exit(91)
	}
	dataPath := t.TempDir()
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		writeTestFile(t, filepath.Join(root, name), []byte(name))
	}
	store, err := db.New(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	mustScannerExec(t, store.DB, `INSERT INTO library(id,name,owner_user_id) VALUES(1,'Crash test',1)`)
	mustScannerExec(t, store.DB, `INSERT INTO metadata_job(id,job_type,title,status,created_at) VALUES(1,'library_scan','Scan','running',1)`)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestScanCrashRecoverySubprocess$")
	cmd.Env = append(os.Environ(), "CRYPTORUM_SCAN_CRASH_TEST_DATA="+dataPath, "CRYPTORUM_SCAN_CRASH_TEST_ROOT="+root)
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 23 {
		t.Fatalf("unexpected child exit %v: %s", err, output)
	}
	store, err = db.New(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := New(store.DB, dataPath, filepath.Join(dataPath, "covers"))
	var final ScanProgress
	if _, err := s.ScanLibraryWithOptions(1, []string{root}, func(p ScanProgress) { final = p }, nil, ScanOptions{JobID: 1, Reconcile: true}); err != nil {
		t.Fatal(err)
	}
	if final.ScannedFiles != 2 || final.ImportedBooks != 2 {
		t.Fatalf("lost crash checkpoint: %+v", final)
	}
	var count int
	if err := store.QueryRow(`SELECT count(*) FROM book`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("duplicate imports after process exit: %d", count)
	}
}
