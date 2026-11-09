package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cryptorum/internal/metadata"
)

func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for pipeline")
	}
}

func TestPreparationPipelineBoundsAndOrderedWriter(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			files := make([]fileInventoryItem, 8)
			for i := range files {
				files[i].Path = fmt.Sprint(i)
			}
			started := make(chan struct{}, 8)
			releaseFirst := make(chan struct{})
			writerEntered := make(chan struct{})
			releaseWriter := make(chan struct{})
			var active, peak, prepared atomic.Int32
			var order []string
			prepare := func(ctx context.Context, file fileInventoryItem) preparedFile {
				n := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
				}
				prepared.Add(1)
				started <- struct{}{}
				if file.Path == "0" {
					select {
					case <-releaseFirst:
					case <-ctx.Done():
					}
				}
				return preparedFile{file: file}
			}
			consume := func(value preparedFile) error {
				if value.file.Path == "0" {
					close(writerEntered)
					select {
					case <-releaseWriter:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				order = append(order, value.file.Path)
				return nil
			}
			done := make(chan error, 1)
			go func() { done <- runPreparationPipeline(ctx, workers, files, prepare, consume, nil) }()
			for i := 0; i < workers; i++ {
				waitSignal(t, started)
			}
			if got := prepared.Load(); got != int32(workers) {
				t.Fatalf("unbounded preparation: %d", got)
			}
			close(releaseFirst)
			waitSignal(t, writerEntered)
			if got := prepared.Load(); got != int32(workers) {
				t.Fatalf("prepared results ran ahead of blocked writer: %d", got)
			}
			close(releaseWriter)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("pipeline failed to finish")
			}
			if peak.Load() > int32(workers) {
				t.Fatalf("active preparation %d > %d", peak.Load(), workers)
			}
			for i, path := range order {
				if path != fmt.Sprint(i) {
					t.Fatalf("unordered commit %v", order)
				}
			}
			if len(order) != len(files) {
				t.Fatal(order)
			}
		})
	}
}

func TestPreparationPipelineStopDiscardsUncommittedWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	var stopped atomic.Bool
	var consumed, active atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- runPreparationPipeline(ctx, 2, []fileInventoryItem{{Path: "first"}, {Path: "second"}, {Path: "undispatched"}}, func(ctx context.Context, file fileInventoryItem) preparedFile {
			active.Add(1)
			defer active.Add(-1)
			started <- struct{}{}
			if file.Path == "first" {
				<-ctx.Done()
			}
			return preparedFile{file: file}
		}, func(preparedFile) error { consumed.Add(1); return nil }, stopped.Load)
	}()
	waitSignal(t, started)
	waitSignal(t, started)
	stopped.Store(true)
	select {
	case err := <-done:
		if !errors.Is(err, ErrScanCancelled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not drain preparation workers")
	}
	if active.Load() != 0 || consumed.Load() != 0 {
		t.Fatalf("uncommitted work escaped stop: active=%d commits=%d", active.Load(), consumed.Load())
	}
}

func TestPreparationPipelineWriterFailureStopsWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sentinel := errors.New("checkpoint failed")
	var active atomic.Int32
	err := runPreparationPipeline(ctx, 2, []fileInventoryItem{{Path: "first"}, {Path: "second"}, {Path: "third"}}, func(ctx context.Context, file fileInventoryItem) preparedFile {
		active.Add(1)
		defer active.Add(-1)
		if file.Path != "first" {
			<-ctx.Done()
		}
		return preparedFile{file: file}
	}, func(preparedFile) error { return sentinel }, nil)
	if !errors.Is(err, sentinel) || active.Load() != 0 {
		t.Fatalf("writer error leaked workers: %v %d", err, active.Load())
	}
}

func TestHeavyExtractionLimitAndCancellation(t *testing.T) {
	s := New(nil, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		_, err := s.withExtractionSlot(ctx, "first.cbr", func() (*metadata.BookMetadata, error) {
			close(firstEntered)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
			}
			return nil, nil
		})
		done <- err
	}()
	waitSignal(t, firstEntered)
	waiting, stopWaiting := context.WithCancel(ctx)
	go func() {
		_, err := s.withExtractionSlot(waiting, "second.cbz", func() (*metadata.BookMetadata, error) { close(secondEntered); return nil, nil })
		done <- err
	}()
	stopWaiting()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting extractor ran: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waiting extraction ignored cancellation")
	}
	select {
	case <-secondEntered:
		t.Fatal("two heavy extractions overlapped")
	default:
	}
	close(releaseFirst)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPreparedDuplicatesRecheckedByWriter(t *testing.T) {
	db := setupScannerTestDB(t)
	s := New(db.DB, t.TempDir(), t.TempDir())
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		writeTestFile(t, filepath.Join(root, name), []byte("identical comic content"))
	}
	files, err := collectProcessableFiles([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	values := make([]preparedFile, len(files))
	var group sync.WaitGroup
	for i, file := range files {
		group.Add(1)
		go func(i int, file fileInventoryItem) {
			defer group.Done()
			values[i] = s.prepareFile(context.Background(), 1, file)
		}(i, file)
	}
	group.Wait()
	for _, value := range values {
		if value.err != nil || !value.metadataReady {
			t.Fatalf("preparation: %v", value.err)
		}
	}
	for i, value := range values {
		result, err := s.processPreparedFile(1, value, 1, time.Now().Unix())
		if err != nil {
			t.Fatal(err)
		}
		if (i == 0) != result.Imported {
			t.Fatalf("duplicate writer result %+v", result)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM book`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("parallel preparation created %d books", count)
	}
}

func TestConcurrentRelinkingKeepsOriginalIdentity(t *testing.T) {
	db := setupScannerTestDB(t)
	s := New(db.DB, t.TempDir(), t.TempDir())
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	oldPath := filepath.Join(oldRoot, "old.txt")
	writeTestFile(t, oldPath, []byte("copied content"))
	if _, err := s.ScanLibrary(1, []string{oldRoot}); err != nil {
		t.Fatal(err)
	}
	id := onlyBookID(t, db.DB)
	mustScannerExec(t, db.DB, `UPDATE book_metadata SET title='User title' WHERE book_id=?`, id)
	mustScannerExec(t, db.DB, `INSERT INTO reading_progress(book_id,status,percent,updated_at,owner_user_id) VALUES(?,'reading',42,1,1)`, id)
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"copy-a.txt", "copy-b.txt"} {
		writeTestFile(t, filepath.Join(newRoot, name), []byte("copied content"))
	}
	var final ScanProgress
	if _, err := s.ScanLibraryWithProgress(2, []string{newRoot}, func(p ScanProgress) { final = p }); err != nil {
		t.Fatal(err)
	}
	if final.RelinkedFiles != 1 || final.DuplicateFiles != 1 {
		t.Fatalf("relink race: %+v", final)
	}
	assertSingleRelinkedBook(t, db.DB, id, 2, filepath.Join(newRoot, "copy-a.txt"), "User title")
	var percent int
	if err := db.QueryRow(`SELECT percent FROM reading_progress WHERE book_id=?`, id).Scan(&percent); err != nil {
		t.Fatal(err)
	}
	if percent != 42 {
		t.Fatal("progress lost")
	}
}

func TestPreparedCoverAndSingleWorkerMode(t *testing.T) {
	db := setupScannerTestDB(t)
	s := NewWithWorkers(db.DB, t.TempDir(), t.TempDir(), 1)
	root := t.TempDir()
	path := filepath.Join(root, "book.txt")
	writeTestFile(t, path, []byte("book"))
	files, err := collectProcessableFiles([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := s.prepareFile(context.Background(), 1, files[0])
	canvas := image.NewRGBA(image.Rect(0, 0, 3, 4))
	canvas.Set(0, 0, color.White)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	value.meta.CoverData = buffer.Bytes()
	value.cover = prepareCover(value.meta, db.DB)
	value.meta.CoverData = nil // Writer must use prepared bytes, not encode again.
	if len(value.cover) == 0 {
		t.Fatal("empty prepared cover")
	}
	if _, err := s.processPreparedFile(1, value, 1, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	var cover string
	if err := db.QueryRow(`SELECT cover_path FROM book_metadata`).Scan(&cover); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(cover); err != nil || len(data) == 0 {
		t.Fatalf("cover not saved: %v", err)
	}
	var final ScanProgress
	if _, err := s.ScanLibraryWithProgress(1, []string{root}, func(p ScanProgress) { final = p }); err != nil {
		t.Fatal(err)
	}
	if final.Timings.Workers != 1 || final.UnchangedFiles != 1 || final.Timings.ElapsedMS <= 0 {
		t.Fatalf("single worker timings %+v", final)
	}
}

func TestHashPreparationHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.pdf")
	writeLargeTestFile(t, path, legacySampleMaxHashSize+1024)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := computeFileHashesContext(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("hash ignored cancellation: %v", err)
	}
}

func TestOverlappingRootsInventoryOnce(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "series")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(nested, "book.txt"), []byte("book"))
	files, err := collectProcessableFiles([]string{root, nested, root}, nil)
	if err != nil || len(files) != 1 {
		t.Fatalf("overlapping inventory: %d %v", len(files), err)
	}
}
