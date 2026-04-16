package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var coverThumbSizes = map[string]int{
	"small":  240,
	"medium": 360,
	"large":  520,
}

// osRemove wraps os.Remove for use across handler files
func osRemove(path string) error {
	return os.Remove(path)
}

func resolveCoverFile(bookID string) (string, int64, error) {
	var coverPath string
	var coverUpdatedOn int64
	err := appDB.QueryRow(`
		SELECT COALESCE(cover_path, ''), COALESCE(cover_updated_on, 0) FROM book_metadata WHERE book_id = ?
	`, bookID).Scan(&coverPath, &coverUpdatedOn)
	if err != nil && err != sql.ErrNoRows {
		return "", 0, err
	}

	if coverPath == "" {
		coversPath := appConfig.GetCoversPath()
		possiblePaths := []string{
			filepath.Join(coversPath, bookID+".webp"),
			filepath.Join(coversPath, bookID+".jpg"),
			filepath.Join(coversPath, bookID+".jpeg"),
			filepath.Join(coversPath, bookID+".png"),
			filepath.Join(coversPath, bookID+".gif"),
		}

		for _, path := range possiblePaths {
			if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
				coverPath = path
				coverUpdatedOn = info.ModTime().Unix()
				break
			}
		}
	}

	if coverPath == "" {
		return "", 0, nil
	}

	coverPath = translateHostPathToContainerPath(coverPath)

	if info, statErr := os.Stat(coverPath); statErr == nil && !info.IsDir() {
		fileModTime := info.ModTime().Unix()
		if fileModTime > coverUpdatedOn {
			coverUpdatedOn = fileModTime
		}
	}

	return coverPath, coverUpdatedOn, nil
}

func serveCoverFile(w http.ResponseWriter, r *http.Request, bookID string, coverPath string, coverUpdatedOn int64) {
	if coverPath == "" {
		errorResponse(w, http.StatusNotFound, "Cover not found")
		return
	}

	w.Header().Set("ETag", fmt.Sprintf("\"cover-%s-%d\"", bookID, coverUpdatedOn))
	w.Header().Set("Last-Modified", time.Unix(coverUpdatedOn, 0).UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")

	http.ServeFile(w, r, coverPath)
}

func generateCoverThumb(sourcePath, thumbPath string, sizeName string) error {
	target, ok := coverThumbSizes[sizeName]
	if !ok {
		target = coverThumbSizes["medium"]
	}

	if err := os.MkdirAll(filepath.Dir(thumbPath), 0o755); err != nil {
		return err
	}

	tmpPath := thumbPath + ".tmp.jpg"
	_ = os.Remove(tmpPath)

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return err
	}

	filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", target, target)
	cmd := exec.Command(
		ffmpegPath,
		"-y",
		"-v", "error",
		"-i", sourcePath,
		"-vf", filter,
		"-frames:v", "1",
		"-f", "image2",
		"-q:v", "4",
		tmpPath,
	)

	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ffmpeg thumbnail generation failed: %w: %s", runErr, strings.TrimSpace(string(output)))
	}

	if err := os.Rename(tmpPath, thumbPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

// translateHostPathToContainerPath translates host filesystem paths to container paths
// This is needed because Docker volumes may be mounted at different paths inside the container
func translateHostPathToContainerPath(filePath string) string {
	// If file exists at original path, return it unchanged
	if _, err := os.Stat(filePath); err == nil {
		return filePath
	}

	// Try to discover mount mappings from /proc/mounts
	mountMappings := getContainerMountMappings()

	for hostPath, containerPath := range mountMappings {
		if strings.HasPrefix(filePath, hostPath) {
			translatedPath := strings.Replace(filePath, hostPath, containerPath, 1)
			if _, err := os.Stat(translatedPath); err == nil {
				return translatedPath
			}
		}
	}

	// Fallback: search for file by name in common book directories
	filename := filepath.Base(filePath)
	searchDirs := []string{"/books", "/data"}

	for _, searchDir := range searchDirs {
		if _, err := os.Stat(searchDir); err != nil {
			continue
		}

		// Walk directory tree looking for file with matching name
		foundPath := findFileByName(searchDir, filename)
		if foundPath != "" {
			return foundPath
		}
	}

	return filePath
}

// findFileByName recursively searches for a file with given filename
func findFileByName(rootDir, filename string) string {
	var found string

	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && info.Name() == filename {
			found = path
			return filepath.SkipAll // stop walking
		}

		// Limit depth to avoid searching too deep
		relPath, _ := filepath.Rel(rootDir, path)
		if strings.Count(relPath, string(filepath.Separator)) > 15 {
			return filepath.SkipDir
		}

		return nil
	})

	return found
}

// getContainerMountMappings reads /proc/mounts to discover host->container path mappings
func getContainerMountMappings() map[string]string {
	mappings := make(map[string]string)

	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return mappings
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		hostPath := fields[0]
		containerPath := fields[1]
		fstype := fields[2]

		// Only consider bind mounts and regular filesystems (ignore虚拟文件系统如proc, sysfs, tmpfs）
		// Common fstypes for bind mounts: ext4, xfs, btrfs, nfs, vfat, ntfs, etc.
		// Skip虚拟filesystems
		skipFstypes := map[string]bool{
			"proc": true, "sysfs": true, "devpts": true, "tmpfs": true,
			"devtmpfs": true, "cgroup": true, "cgroup2": true, "pstore": true,
			"securityfs": true, "debugfs": true, "tracefs": true, "hugetlbfs": true,
			"mqueue": true, "fusectl": true, "configfs": true, "ramfs": true,
			"binfmt_misc": true, "autofs": true, "overlay": true,
		}

		if skipFstypes[fstype] {
			continue
		}

		// Only map directories that look like book/media paths
		// Skip system directories
		if strings.HasPrefix(containerPath, "/usr") ||
			strings.HasPrefix(containerPath, "/bin") ||
			strings.HasPrefix(containerPath, "/sbin") ||
			strings.HasPrefix(containerPath, "/lib") ||
			strings.HasPrefix(containerPath, "/etc") ||
			strings.HasPrefix(containerPath, "/var") ||
			strings.HasPrefix(containerPath, "/sys") ||
			strings.HasPrefix(containerPath, "/dev") ||
			strings.HasPrefix(containerPath, "/run") ||
			strings.HasPrefix(containerPath, "/boot") ||
			strings.HasPrefix(containerPath, "/root") ||
			strings.HasPrefix(containerPath, "/home") ||
			containerPath == "/" {
			continue
		}

		mappings[hostPath] = containerPath
	}

	return mappings
}

// countCbzPages returns the number of image pages in a CBZ archive
func countCbzPages(filePath string) (int, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	count := 0
	for _, f := range reader.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
			count++
		}
	}
	return count, nil
}

func selectBookFileByFormat(bookID int64, preferredFormat string) (string, string, error) {
	rows, err := appDB.Query(`
		SELECT path, format
		FROM book_file
		WHERE book_id = ?
		ORDER BY id
	`, bookID)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	preferredFormat = strings.ToLower(strings.TrimSpace(preferredFormat))
	var fallbackPath, fallbackFormat string

	for rows.Next() {
		var filePath, format string
		if err := rows.Scan(&filePath, &format); err != nil {
			continue
		}
		if fallbackPath == "" {
			fallbackPath = filePath
			fallbackFormat = format
		}
		if preferredFormat != "" && strings.EqualFold(format, preferredFormat) {
			return filePath, format, nil
		}
	}

	if err := rows.Err(); err != nil {
		return "", "", err
	}
	if fallbackPath == "" {
		return "", "", sql.ErrNoRows
	}

	return fallbackPath, fallbackFormat, nil
}

// ServeBookFileHandler serves the raw book file for download
func ServeBookFileHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	requestedFormat := r.URL.Query().Get("format")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}

	// Translate host path to container path if needed
	filePath = translateHostPathToContainerPath(filePath)

	// Set content type based on format
	contentTypes := map[string]string{
		"epub": "application/epub+zip",
		"pdf":  "application/pdf",
		"cbz":  "application/vnd.comicbook+zip",
		"cbr":  "application/vnd.comicbook-rar",
		"mp3":  "audio/mpeg",
		"m4a":  "audio/mp4",
		"m4b":  "audio/mp4",
		"flac": "audio/flac",
		"ogg":  "audio/ogg",
	}

	if contentType, ok := contentTypes[format]; ok {
		w.Header().Set("Content-Type", contentType)
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(filePath)))
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, filePath)
}

func serveBookFileByID(w http.ResponseWriter, r *http.Request, disposition string) {
	bookID := chi.URLParam(r, "bookID")
	fileID := chi.URLParam(r, "fileID")
	current := getUserFromContext(r.Context())

	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	fileIDInt, err := strconv.ParseInt(fileID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid file ID")
		return
	}

	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if !requirePermission(current, PermissionDownloadBooks) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var filePath, format string
	err = appDB.QueryRow(`
		SELECT path, format
		FROM book_file
		WHERE id = ? AND book_id = ?
	`, fileIDInt, bookIDInt).Scan(&filePath, &format)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}

	filePath = translateHostPathToContainerPath(filePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disposition, filepath.Base(filePath)))

	http.ServeFile(w, r, filePath)
}

func ServeBookFileByIDHandler(w http.ResponseWriter, r *http.Request) {
	serveBookFileByID(w, r, "attachment")
}

func ConvertBookFileHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	fileID := chi.URLParam(r, "fileID")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	current := getUserFromContext(r.Context())

	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	fileIDInt, err := strconv.ParseInt(fileID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid file ID")
		return
	}

	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if !requirePermission(current, PermissionDownloadBooks) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	switch format {
	case "epub", "fb2", "txt", "rtf":
	default:
		errorResponse(w, http.StatusBadRequest, "Unsupported conversion format")
		return
	}

	var filePath, sourceFormat string
	err = appDB.QueryRow(`
		SELECT path, format
		FROM book_file
		WHERE id = ? AND book_id = ?
	`, fileIDInt, bookIDInt).Scan(&filePath, &sourceFormat)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}

	filePath = translateHostPathToContainerPath(filePath)
	sourceFormat = strings.ToLower(sourceFormat)

	if sourceFormat == format {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", replaceFileExt(filepath.Base(filePath), format)))
		http.ServeFile(w, r, filePath)
		return
	}

	tempDir := filepath.Join(appConfig.GetBookCachePath(), "downloads")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to prepare conversion")
		return
	}

	tempFile, err := os.CreateTemp(tempDir, fmt.Sprintf("book-%d-*."+format, bookIDInt))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create conversion output")
		return
	}
	tempFilePath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempFilePath)

	if err := convertWithCalibre(filePath, tempFilePath); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	downloadName := replaceFileExt(filepath.Base(filePath), format)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
	switch format {
	case "epub":
		w.Header().Set("Content-Type", "application/epub+zip")
	case "fb2":
		w.Header().Set("Content-Type", "application/fb2+xml")
	case "txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case "rtf":
		w.Header().Set("Content-Type", "application/rtf")
	}
	http.ServeFile(w, r, tempFilePath)
}

func replaceFileExt(name, ext string) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + "." + ext
}

// ServeEpubResourceHandler serves individual resources from an EPUB file
func ServeEpubResourceHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	resourcePath := chi.URLParam(r, "*")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var filePath string
	err = appDB.QueryRow(`
		SELECT path FROM book_file WHERE book_id = ? AND format = 'epub' LIMIT 1
	`, bookID).Scan(&filePath)

	if err != nil {
		errorResponse(w, http.StatusNotFound, "EPUB file not found")
		return
	}

	// Translate host path to container path if needed
	filePath = translateHostPathToContainerPath(filePath)

	// Open the EPUB as a ZIP file
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to open EPUB")
		return
	}
	defer reader.Close()

	// Find and serve the resource
	for _, f := range reader.File {
		// Normalize paths for comparison
		f.Name = strings.TrimPrefix(f.Name, "./")
		resourcePath = strings.TrimPrefix(resourcePath, "./")

		if f.Name == resourcePath {
			rc, err := f.Open()
			if err != nil {
				errorResponse(w, http.StatusInternalServerError, "Failed to open resource")
				return
			}
			defer rc.Close()

			// Set content type based on extension
			ext := strings.ToLower(filepath.Ext(resourcePath))
			contentTypes := map[string]string{
				".html":  "text/html",
				".xhtml": "application/xhtml+xml",
				".css":   "text/css",
				".js":    "application/javascript",
				".jpg":   "image/jpeg",
				".jpeg":  "image/jpeg",
				".png":   "image/png",
				".gif":   "image/gif",
				".svg":   "image/svg+xml",
				".ncx":   "application/x-dtbncx+xml",
				".opf":   "application/oebps-package+xml",
			}

			if contentType, ok := contentTypes[ext]; ok {
				w.Header().Set("Content-Type", contentType)
			} else {
				w.Header().Set("Content-Type", "application/octet-stream")
			}

			io.Copy(w, rc)
			return
		}
	}

	errorResponse(w, http.StatusNotFound, "Resource not found")
}

// ServeCbxPageHandler serves individual pages from a CBX archive
func ServeCbxPageHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	pageNumStr := chi.URLParam(r, "pageNum")
	requestedFormat := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	pageNum, err := strconv.Atoi(pageNumStr)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid page number")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil || (requestedFormat == "" && format != "cbz" && format != "cbr" && format != "cb7") {
		err = appDB.QueryRow(`
			SELECT path, format FROM book_file
			WHERE book_id = ? AND format IN ('cbz', 'cbr', 'cb7')
			ORDER BY id
			LIMIT 1
		`, bookIDInt).Scan(&filePath, &format)
	}
	if err != nil {
		errorResponse(w, http.StatusNotFound, "CBX file not found")
		return
	}
	if format != "cbz" && format != "cbr" && format != "cb7" {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Format '%s' is not supported for comic reading.", format))
		return
	}

	// Translate host path to container path if needed
	filePath = translateHostPathToContainerPath(filePath)

	switch format {
	case "cbz":
		serveCbzPage(w, filePath, pageNum)
	case "cbr":
		serveCbrPage(w, filePath, pageNum)
	case "cb7":
		errorResponse(w, http.StatusNotImplemented, "CB7 reading is not supported yet")
	default:
		errorResponse(w, http.StatusNotImplemented, "Format not supported")
	}
}

func serveCbzPage(w http.ResponseWriter, filePath string, pageNum int) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to open CBZ")
		return
	}
	defer reader.Close()

	// Collect image files
	var images []*zip.File
	for _, f := range reader.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
			images = append(images, f)
		}
	}

	if pageNum < 1 || pageNum > len(images) {
		errorResponse(w, http.StatusNotFound, "Page not found")
		return
	}

	// Open and serve the page (pageNum is 1-based)
	f := images[pageNum-1]
	rc, err := f.Open()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to open page")
		return
	}
	defer rc.Close()

	ext := strings.ToLower(filepath.Ext(f.Name))
	contentTypes := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
	}

	if contentType, ok := contentTypes[ext]; ok {
		w.Header().Set("Content-Type", contentType)
	}

	io.Copy(w, rc)
}

func serveCbrPage(w http.ResponseWriter, filePath string, pageNum int) {
	// CBR (RAR) support would require the rardecode library
	// For now, return not implemented
	errorResponse(w, http.StatusNotImplemented, "CBR format not yet supported")
}

// ServeCoverHandler serves book covers
func ServeCoverHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	coverPath, coverUpdatedOn, err := resolveCoverFile(bookID)
	if err != nil || coverPath == "" {
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to resolve cover")
		} else {
			errorResponse(w, http.StatusNotFound, "Cover not found")
		}
		return
	}
	serveCoverFile(w, r, bookID, coverPath, coverUpdatedOn)
}

// ServeCoverThumbHandler serves book cover thumbnails
func ServeCoverThumbHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	sizeName := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("size")))
	if _, ok := coverThumbSizes[sizeName]; !ok {
		sizeName = "medium"
	}

	coverPath, coverUpdatedOn, err := resolveCoverFile(bookID)
	if err != nil || coverPath == "" {
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to resolve cover")
		} else {
			errorResponse(w, http.StatusNotFound, "Cover not found")
		}
		return
	}

	thumbDir := appConfig.GetThumbsPath()
	thumbPath := filepath.Join(thumbDir, fmt.Sprintf("%s-%s.jpg", bookID, sizeName))
	requestVersion := strings.TrimSpace(r.URL.Query().Get("v"))
	versionMatches := requestVersion != "" && requestVersion == strconv.FormatInt(coverUpdatedOn, 10)

	if info, statErr := os.Stat(thumbPath); statErr == nil && !info.IsDir() && info.ModTime().Unix() >= coverUpdatedOn {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("ETag", fmt.Sprintf("\"cover-thumb-%s-%s-%d\"", bookID, sizeName, coverUpdatedOn))
		w.Header().Set("Last-Modified", time.Unix(coverUpdatedOn, 0).UTC().Format(http.TimeFormat))
		if versionMatches {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
		}
		http.ServeFile(w, r, thumbPath)
		return
	}

	if err := generateCoverThumb(coverPath, thumbPath, sizeName); err != nil {
		log.Printf("Failed to generate cover thumb for book %s (%s): %v", bookID, sizeName, err)
		serveCoverFile(w, r, bookID, coverPath, coverUpdatedOn)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("ETag", fmt.Sprintf("\"cover-thumb-%s-%s-%d\"", bookID, sizeName, coverUpdatedOn))
	w.Header().Set("Last-Modified", time.Unix(coverUpdatedOn, 0).UTC().Format(http.TimeFormat))
	if versionMatches {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	}
	http.ServeFile(w, r, thumbPath)
}

// TocItem represents a table of contents entry
type TocItem struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Level    int       `json:"level"`
	Children []TocItem `json:"children,omitempty"`
}

// ServeContinuousBookHandler serves cached HTML derived from the canonical
// processed text-book package for continuous scrolling reading.
func ServeContinuousBookHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	requestedFormat := r.URL.Query().Get("format")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}

	if !isSupportedTextBookFormat(format) {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf(
			"Format '%s' is not supported for continuous reading.", format,
		))
		return
	}

	filePath = translateHostPathToContainerPath(filePath)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		errorResponse(w, http.StatusNotFound, "Book file not found at path: "+filePath)
		return
	}

	log.Printf("Serving continuous content for book %s at path: %s\n", bookID, filePath)

	htmlContent, err := getOrConvertBook(bookID, filePath, format)
	if err != nil {
		log.Printf("Conversion error for book %s: %v\n", bookID, err)
		errorResponse(w, http.StatusInternalServerError, "Failed to convert book: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write([]byte(htmlContent))
}

// getOrConvertBook returns cached HTML or builds the canonical processed package.
func getOrConvertBook(bookID, filePath, format string) (string, error) {
	result, err := ensureProcessedTextBook(bookID, filePath, format)
	if err != nil {
		return "", err
	}

	return result.HTMLContent, nil
}

// ServeContinuousMediaHandler serves media files (images) extracted from converted books
func ServeContinuousMediaHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	mediaPath := chi.URLParam(r, "*")
	requestedFormat := r.URL.Query().Get("format")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}
	if !isSupportedTextBookFormat(format) {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf(
			"Format '%s' is not supported for continuous reading.", format,
		))
		return
	}
	filePath = translateHostPathToContainerPath(filePath)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		errorResponse(w, http.StatusNotFound, "Book file not found at path: "+filePath)
		return
	}
	if _, err := ensureProcessedTextBook(bookID, filePath, format); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to process book: "+err.Error())
		return
	}

	paths := getTextBookCachePaths(bookID, format)
	explodedPath := filepath.Join(paths.ExplodedDir, filepath.FromSlash(mediaPath))

	// Security: prevent path traversal
	cleanCacheDir := filepath.Clean(paths.ExplodedDir)
	cleanFilePath := filepath.Clean(explodedPath)
	if !strings.HasPrefix(cleanFilePath, cleanCacheDir+string(filepath.Separator)) {
		errorResponse(w, http.StatusForbidden, "Invalid media path")
		return
	}

	if _, err := os.Stat(cleanFilePath); os.IsNotExist(err) {
		errorResponse(w, http.StatusNotFound, "Media file not found")
		return
	}

	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, cleanFilePath)
}

// ServeContinuousTocHandler returns the table of contents for a book's continuous view
func ServeContinuousTocHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	requestedFormat := r.URL.Query().Get("format")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}

	filePath = translateHostPathToContainerPath(filePath)

	var toc []TocItem

	if isSupportedTextBookFormat(format) {
		// Ensure book is converted (TOC is parsed from NCX)
		if _, err := ensureProcessedTextBook(bookID, filePath, format); err != nil {
			log.Printf("Failed to convert book %s for TOC: %v\n", bookID, err)
			toc = []TocItem{}
		} else {
			tocCachePath := getTextBookCachePaths(bookID, format).TocCachePath
			if data, err := os.ReadFile(tocCachePath); err == nil {
				json.Unmarshal(data, &toc)
			} else {
				log.Printf("Failed to read cached TOC: %v\n", err)
				toc = []TocItem{}
			}
		}
	} else {
		toc = []TocItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(toc); err != nil {
		log.Printf("Failed to encode TOC: %v\n", err)
	}
}

// extractTocFromHTML extracts a table of contents from pandoc-generated HTML headings
func extractTocFromHTML(htmlContent string) []TocItem {
	re := regexp.MustCompile(`(?s)<h([1-3])([^>]*)>(.*?)</h[1-3]>`)
	idRe := regexp.MustCompile(`\bid="([^"]*)"`)

	var flat []TocItem
	matches := re.FindAllStringSubmatch(htmlContent, -1)

	for _, m := range matches {
		level, _ := strconv.Atoi(m[1])
		attrs := m[2]
		content := m[3]

		label := stripHTMLTags(strings.TrimSpace(content))
		if label == "" {
			continue
		}

		var id string
		if idMatch := idRe.FindStringSubmatch(attrs); len(idMatch) > 1 {
			id = idMatch[1]
		}

		flat = append(flat, TocItem{
			ID:    id,
			Label: label,
			Level: level,
		})
	}

	return buildTocHierarchy(flat)
}

// buildTocHierarchy converts a flat list of TOC items into a nested tree
func buildTocHierarchy(flat []TocItem) []TocItem {
	if len(flat) == 0 {
		return []TocItem{}
	}

	// Simple approach: use a stack to build hierarchy
	var roots []TocItem
	// Stack of pointers into the result slice - track last item at each level
	type stackEntry struct {
		item  *TocItem
		level int
	}
	var stack []stackEntry

	for _, item := range flat {
		item := item // copy
		item.Children = nil

		// Pop stack items at same or deeper level
		for len(stack) > 0 && stack[len(stack)-1].level >= item.Level {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			roots = append(roots, item)
			stack = append(stack, stackEntry{&roots[len(roots)-1], item.Level})
		} else {
			parent := stack[len(stack)-1].item
			parent.Children = append(parent.Children, item)
			stack = append(stack, stackEntry{&parent.Children[len(parent.Children)-1], item.Level})
		}
	}

	return roots
}

// ServeContinuousStylesHandler serves the preserved stylesheet.css for original layout
func ServeContinuousStylesHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	requestedFormat := r.URL.Query().Get("format")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}
	if !isSupportedTextBookFormat(format) {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf(
			"Format '%s' is not supported for continuous reading.", format,
		))
		return
	}
	filePath = translateHostPathToContainerPath(filePath)
	if _, err := ensureProcessedTextBook(bookID, filePath, format); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to process book: "+err.Error())
		return
	}
	cssPath := getTextBookCachePaths(bookID, format).CSSCachePath
	if _, err := os.Stat(cssPath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "text/css")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("/* No preserved styles available */"))
		return
	}

	w.Header().Set("Content-Type", "text/css")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, cssPath)
}

// ServeProcessedBookFileHandler serves the canonical processed EPUB for text readers.
func ServeProcessedBookFileHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	requestedFormat := r.URL.Query().Get("format")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}

	if !isSupportedTextBookFormat(format) {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf(
			"Format '%s' is not supported for processed ebook reading.", format,
		))
		return
	}

	filePath = translateHostPathToContainerPath(filePath)
	result, err := ensureProcessedTextBook(bookID, filePath, format)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to process book: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s-processed.epub\"", bookID))
	http.ServeFile(w, r, result.EPUBPath)
}
