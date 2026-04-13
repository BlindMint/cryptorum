package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/robfig/cron/v3"

	"cryptorum/internal/config"
	"cryptorum/internal/db"
	"cryptorum/internal/scanner"
	"cryptorum/internal/watcher"
)

var (
	appConfig         *config.Config
	appDB             *db.DB
	appScanner        *scanner.Scanner
	appWatcher        *watcher.Watcher
	cronRunner        *cron.Cron
	backupCronRunner  *cron.Cron
	backupCronMu      sync.Mutex
	scanningLibraries map[int64]bool
	isScanning        bool
	maintenanceMode   atomic.Bool
)

func main() {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Load configuration
	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	var err error
	appConfig, err = config.Load(configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize scanning libraries map
	scanningLibraries = make(map[int64]bool)

	// Initialize database
	appDB, err = db.New(appConfig.Server.DataPath)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer appDB.Close()

	bootstrapUser, err := ensureBootstrapUser()
	if err != nil {
		slog.Error("Failed to ensure bootstrap user", "error", err)
		os.Exit(1)
	}
	_ = bootstrapUser

	if err := ensureBootstrapOwnership(); err != nil {
		slog.Error("Failed to initialize ownership", "error", err)
		os.Exit(1)
	}

	startupTime := time.Now().Unix()
	if count, err := closeStaleReadingSessions(startupTime); err != nil {
		slog.Error("Failed to reconcile stale reading sessions", "error", err)
		os.Exit(1)
	} else if count > 0 {
		slog.Info("Closed stale reading sessions after restart", "count", count)
	}

	// Initialize scanner
	appScanner = scanner.New(appDB.DB, appConfig.Server.DataPath, appConfig.GetCoversPath())

	// Initialize router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Initialize routes
	initRoutes(r)

	// Start initial library scan
	go performInitialScan()

	// Initialize file watcher
	initializeWatcher()

	// Start background cron tasks
	startCronTasks()

	// Create server
	port := appConfig.Server.Port
	if port == "" {
		port = "6060"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Start server
	go func() {
		slog.Info("Server starting", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	<-quit
	slog.Info("Server shutting down")

	// Stop watcher
	if appWatcher != nil {
		appWatcher.Stop()
	}

	// Stop cron
	if cronRunner != nil {
		cronRunner.Stop()
	}
	stopDatabaseBackupSchedule()

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Server stopped gracefully")
}

func performInitialScan() {
	slog.Info("Starting initial library scan")

	for _, lib := range appConfig.Libraries {
		// Get or create library
		var libraryID int64
		err := appDB.QueryRow("SELECT id FROM library WHERE name = ? AND owner_user_id = ?", lib.Name, 1).Scan(&libraryID)
		if err != nil {
			libraryID, err = getFirstAvailableLibraryID(appDB)
			if err != nil {
				slog.Error("Failed to find available library ID", "name", lib.Name, "error", err)
				continue
			}
			_, err = appDB.Exec("INSERT INTO library (id, name, owner_user_id) VALUES (?, ?, ?)", libraryID, lib.Name, 1)
			if err != nil {
				slog.Error("Failed to create library", "name", lib.Name, "error", err)
				continue
			}

			// Add paths
			for _, path := range lib.Paths {
				appDB.Exec("INSERT INTO library_path (library_id, path) VALUES (?, ?)", libraryID, path)
			}
		}

		count, err := appScanner.ScanLibrary(libraryID, lib.Paths)
		if err != nil {
			slog.Error("Failed to scan library", "name", lib.Name, "error", err)
			continue
		}
		slog.Info("Library scan completed", "name", lib.Name, "imported", count)
	}
}

func initializeWatcher() {
	var err error
	appWatcher, err = watcher.New(
		appDB.DB,
		appConfig.Server.DataPath,
		appConfig.Bookdrop.Path,
		func(path string) {
			slog.Info("File changed, rescanning", "path", path)
			// Rescan the library containing this file
			for _, lib := range appConfig.Libraries {
				for _, libPath := range lib.Paths {
					if filepath.HasPrefix(path, libPath) {
						var libraryID int64
						appDB.QueryRow("SELECT id FROM library WHERE name = ? AND owner_user_id = ?", lib.Name, 1).Scan(&libraryID)
						appScanner.ScanLibrary(libraryID, []string{filepath.Dir(path)})
						return
					}
				}
			}
		},
	)
	if err != nil {
		slog.Error("Failed to initialize file watcher", "error", err)
		return
	}

	// Collect all library paths
	var allPaths []string
	for _, lib := range appConfig.Libraries {
		allPaths = append(allPaths, lib.Paths...)
	}

	if err := appWatcher.Start(allPaths); err != nil {
		slog.Error("Failed to start file watcher", "error", err)
	}
}

func startCronTasks() {
	cronRunner = cron.New()

	// Library scan task
	if appConfig.Tasks.LibraryScan.Cron != "" {
		_, err := cronRunner.AddFunc(appConfig.Tasks.LibraryScan.Cron, func() {
			slog.Info("Running scheduled library scan")
			for _, lib := range appConfig.Libraries {
				var libraryID int64
				appDB.QueryRow("SELECT id FROM library WHERE name = ? AND owner_user_id = ?", lib.Name, 1).Scan(&libraryID)
				appScanner.ScanLibrary(libraryID, lib.Paths)
			}
		})
		if err != nil {
			slog.Error("Failed to schedule library scan", "error", err)
		}
	}

	// Metadata refresh task — re-extract metadata for books missing title or cover
	if appConfig.Tasks.MetadataRefresh.Cron != "" {
		_, err := cronRunner.AddFunc(appConfig.Tasks.MetadataRefresh.Cron, func() {
			slog.Info("Running scheduled metadata refresh")
			count, err := appScanner.RefreshMissingMetadata(100)
			if err != nil {
				slog.Error("Metadata refresh failed", "error", err)
				return
			}
			slog.Info("Metadata refresh complete", "updated", count)
		})
		if err != nil {
			slog.Error("Failed to schedule metadata refresh", "error", err)
		}
	}

	cronRunner.Start()
	startDatabaseBackupSchedule()
}

// Helper function for JSON responses
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// Helper function for error responses
func errorResponse(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}
