package main

import (
	"errors"
	"net/http"
	"os"
)

type fileAvailability struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func availabilityForError(err error) fileAvailability {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fileAvailability{"file_missing", "This book’s file is unavailable. Its library folder may have moved or become disconnected. Your saved reading progress is preserved."}
	case errors.Is(err, os.ErrPermission):
		return fileAvailability{"file_unreadable", "Cryptorum does not have permission to read this file. Check the library folder’s filesystem permissions."}
	default:
		return fileAvailability{"storage_error", "Cryptorum could not access this file. Check the library storage and try again."}
	}
}

// Opening without reading checks accessibility without parsing or hashing content.
// Availability is transient and must never change scanner state.
func checkFileAvailability(path string) fileAvailability {
	path = translateHostPathToContainerPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return availabilityForError(err)
	}
	if !info.Mode().IsRegular() {
		return fileAvailability{"file_unreadable", "The stored book path does not point to a regular file."}
	}
	f, err := os.Open(path)
	if err != nil {
		return availabilityForError(err)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return availabilityForError(err)
	}
	if !info.Mode().IsRegular() {
		return fileAvailability{"file_unreadable", "The stored book path does not point to a regular file."}
	}
	if info.Size() == 0 {
		return fileAvailability{"file_empty", "This book’s file is empty. Restore a complete copy and try again."}
	}
	return fileAvailability{Code: "available"}
}

func writeFileAccessError(w http.ResponseWriter, availability fileAvailability) {
	status := http.StatusServiceUnavailable
	if availability.Code == "file_missing" {
		status = http.StatusNotFound
	}
	if availability.Code == "file_unreadable" {
		status = http.StatusForbidden
	}
	if availability.Code == "file_empty" {
		status = http.StatusUnprocessableEntity
	}
	w.Header().Del("Content-Disposition")
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, status, map[string]string{"code": availability.Code, "error": availability.Message})
}

func requireAvailableFile(w http.ResponseWriter, path string) bool {
	availability := checkFileAvailability(path)
	if availability.Code == "available" {
		return true
	}
	writeFileAccessError(w, availability)
	return false
}
