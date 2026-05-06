package watcher

import (
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches directories for file changes
type Watcher struct {
	db             *sql.DB
	watcher        *fsnotify.Watcher
	dataPath       string
	bookdropPath   string
	debounceTimers map[string]*time.Timer
	mu             sync.Mutex
	onFileAdded    func(path string)
}

// New creates a new file watcher
func New(db *sql.DB, dataPath string, bookdropPath string, onFileAdded func(path string)) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		db:             db,
		watcher:        fsWatcher,
		dataPath:       dataPath,
		bookdropPath:   bookdropPath,
		debounceTimers: make(map[string]*time.Timer),
		onFileAdded:    onFileAdded,
	}

	return w, nil
}

// Start starts watching directories
func (w *Watcher) Start(libraryPaths []string) error {
	// Watch library paths
	for _, path := range libraryPaths {
		if err := w.addWatch(path); err != nil {
			slog.Error("Failed to watch library path", "path", path, "error", err)
		}
	}

	// Watch bookdrop path
	if w.bookdropPath != "" {
		if err := w.addWatch(w.bookdropPath); err != nil {
			slog.Error("Failed to watch bookdrop path", "path", w.bookdropPath, "error", err)
		}
	}

	// Start event processing goroutine
	go w.processEvents()

	return nil
}

// addWatch adds a directory to the watcher
func (w *Watcher) addWatch(path string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	// Add the directory to the watcher
	if err := w.watcher.Add(path); err != nil {
		return err
	}

	// Walk and add all subdirectories
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") && p != path {
				return filepath.SkipDir
			}
			if err := w.watcher.Add(p); err != nil {
				slog.Debug("Failed to watch subdirectory", "path", p, "error", err)
			}
		}
		return nil
	})

	return err
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	w.watcher.Close()
}

// processEvents processes file system events
func (w *Watcher) processEvents() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("Watcher error", "error", err)
		}
	}
}

// handleEvent handles a file system event
func (w *Watcher) handleEvent(event fsnotify.Event) {
	// Only process create and write events
	if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
		return
	}

	// Skip directories
	info, err := os.Stat(event.Name)
	if err != nil {
		return
	}
	if info.IsDir() {
		// New subdirectory added - watch it
		if event.Op&fsnotify.Create != 0 {
			if err := w.watcher.Add(event.Name); err != nil {
				slog.Debug("Failed to watch new directory", "path", event.Name, "error", err)
			}
		}
		return
	}

	// Skip non-processable files
	if !isProcessableFile(event.Name) {
		return
	}

	// Debounce events for the same file
	w.mu.Lock()
	if timer, exists := w.debounceTimers[event.Name]; exists {
		timer.Stop()
	}
	w.debounceTimers[event.Name] = time.AfterFunc(3*time.Second, func() {
		w.mu.Lock()
		delete(w.debounceTimers, event.Name)
		w.mu.Unlock()

		// Process the file
		w.processFile(event.Name, event.Op&fsnotify.Create != 0)
	})
	w.mu.Unlock()
}

// processFile processes a file after debounce
func (w *Watcher) processFile(path string, isNew bool) {
	// Check if it's in the bookdrop directory
	if w.bookdropPath != "" && strings.HasPrefix(path, w.bookdropPath) {
		w.handleBookdropFile(path)
		return
	}

	// It's a library file change
	if w.onFileAdded != nil {
		w.onFileAdded(path)
	}
}

// handleBookdropFile handles a new file in the bookdrop directory
func (w *Watcher) handleBookdropFile(path string) {
	filename := filepath.Base(path)

	// Check if already in bookdrop queue
	var existingID int64
	err := w.db.QueryRow(`
		SELECT id FROM bookdrop_file WHERE path = ?
	`, path).Scan(&existingID)

	if err == nil {
		// Already in queue, skip
		slog.Debug("Bookdrop file already in queue", "path", path)
		return
	}

	// Add to bookdrop queue
	_, err = w.db.Exec(`
		INSERT INTO bookdrop_file (filename, path, status, added_at)
		VALUES (?, ?, 'pending', ?)
	`, filename, path, time.Now().Unix())

	if err != nil {
		slog.Error("Failed to add bookdrop file", "path", path, "error", err)
		return
	}

	slog.Info("New file in bookdrop", "path", path)
}

// isProcessableFile checks if a file is a supported format
func isProcessableFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return false
	}
	ext = ext[1:] // Remove the dot

	supportedFormats := map[string]bool{
		"epub": true, "pdf": true,
		"cbz": true, "cbr": true, "cb7": true,
		"mp3": true, "m4a": true, "m4b": true,
		"flac": true, "ogg": true, "wav": true,
		"mobi": true, "azw3": true,
	}

	return supportedFormats[ext]
}
