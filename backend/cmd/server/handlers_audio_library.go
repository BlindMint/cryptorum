package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const bookCatalogAudioVisibilitySQL = `EXISTS (
	SELECT 1 FROM book_file visible_bf
	LEFT JOIN audio_item visible_ai ON visible_ai.file_id = visible_bf.id
	WHERE visible_bf.book_id = b.id AND visible_bf.missing_at IS NULL
	  AND (LOWER(visible_bf.format) NOT IN ('mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav')
	       OR visible_ai.id IS NULL OR visible_ai.category = 'audiobook')
)`

const bookHasAudioSQL = `EXISTS (
	SELECT 1 FROM book_file audio_bf
	WHERE audio_bf.book_id = b.id AND audio_bf.missing_at IS NULL
	  AND LOWER(audio_bf.format) IN ('mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav')
)`

type audioLibraryItem struct {
	ID              int64    `json:"id"`
	BookID          int64    `json:"book_id"`
	FileID          int64    `json:"file_id"`
	LibraryID       int64    `json:"library_id"`
	Category        string   `json:"category"`
	Title           string   `json:"title"`
	Artists         []string `json:"artists"`
	AlbumArtist     string   `json:"album_artist"`
	Album           string   `json:"album"`
	TrackNumber     *int     `json:"track_number"`
	DiscNumber      *int     `json:"disc_number"`
	ReleaseDate     string   `json:"release_date"`
	Genre           string   `json:"genre"`
	DurationSeconds float64  `json:"duration_seconds"`
	ShowTitle       string   `json:"show_title"`
	EpisodeNumber   *int     `json:"episode_number"`
	PublishedAt     string   `json:"published_at"`
	Filename        string   `json:"filename"`
	Format          string   `json:"format"`
	Status          string   `json:"status"`
	PositionSeconds float64  `json:"position_seconds"`
	PlaybackSpeed   *float64 `json:"playback_speed"`
	Unavailable     bool     `json:"unavailable"`
	StreamURL       string   `json:"stream_url"`
	ChapterCount    int      `json:"chapter_count"`
	BookmarkCount   int      `json:"bookmark_count"`
}

type audioChapter struct {
	ID           int64   `json:"id"`
	Position     int     `json:"position"`
	Title        string  `json:"title"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
}

type audioBookmark struct {
	ID        int64   `json:"id"`
	AudioID   int64   `json:"audio_id"`
	Seconds   float64 `json:"seconds"`
	Label     string  `json:"label"`
	CreatedAt int64   `json:"created_at"`
}

func scanAudioLibraryItem(scanner interface{ Scan(...any) error }) (audioLibraryItem, error) {
	var item audioLibraryItem
	var artistsJSON, path string
	var trackNumber, discNumber, episodeNumber sql.NullInt64
	var playbackSpeed sql.NullFloat64
	var missingAt sql.NullInt64
	err := scanner.Scan(
		&item.ID, &item.BookID, &item.FileID, &item.LibraryID, &item.Category, &item.Title,
		&artistsJSON, &item.AlbumArtist, &item.Album, &trackNumber, &discNumber, &item.ReleaseDate,
		&item.Genre, &item.DurationSeconds, &item.ShowTitle, &episodeNumber, &item.PublishedAt,
		&path, &item.Format, &item.Status, &item.PositionSeconds, &playbackSpeed, &missingAt,
		&item.ChapterCount, &item.BookmarkCount,
	)
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal([]byte(artistsJSON), &item.Artists)
	if item.Artists == nil {
		item.Artists = []string{}
	}
	if trackNumber.Valid {
		value := int(trackNumber.Int64)
		item.TrackNumber = &value
	}
	if discNumber.Valid {
		value := int(discNumber.Int64)
		item.DiscNumber = &value
	}
	if episodeNumber.Valid {
		value := int(episodeNumber.Int64)
		item.EpisodeNumber = &value
	}
	if playbackSpeed.Valid {
		item.PlaybackSpeed = &playbackSpeed.Float64
	}
	item.Filename = filepath.Base(path)
	if item.Title == "" || item.Title == path {
		item.Title = strings.TrimSuffix(item.Filename, filepath.Ext(item.Filename))
	}
	item.Unavailable = missingAt.Valid
	item.StreamURL = fmt.Sprintf("/api/books/%d/file?file_id=%d", item.BookID, item.FileID)
	return item, nil
}

const audioLibrarySelect = `
	SELECT ai.id, ai.book_id, ai.file_id, b.library_id, ai.category, ai.title, ai.artists,
	       ai.album_artist, ai.album, ai.track_number, ai.disc_number, ai.release_date,
	       ai.genre, ai.duration_seconds, ai.show_title, ai.episode_number, ai.published_at,
	       bf.path, LOWER(bf.format),
	       CASE WHEN ai.category = 'audiobook' THEN
	           CASE COALESCE(rp.status, 'unread') WHEN 'reading' THEN 'in_progress' WHEN 'finished' THEN 'played' ELSE 'unplayed' END
	           ELSE COALESCE(ls.status, 'unplayed') END,
	       COALESCE(ls.position_seconds, 0), ai.playback_speed, bf.missing_at,
	       (SELECT COUNT(*) FROM audio_chapter ac WHERE ac.audio_item_id = ai.id),
	       (SELECT COUNT(*) FROM audio_bookmark ab WHERE ab.audio_item_id = ai.id AND ab.owner_user_id = ai.owner_user_id)
	FROM audio_item ai
	JOIN book b ON b.id = ai.book_id
	JOIN book_file bf ON bf.id = ai.file_id AND bf.book_id = ai.book_id
	LEFT JOIN audio_listening_state ls ON ls.audio_item_id = ai.id AND ls.owner_user_id = ai.owner_user_id
	LEFT JOIN reading_progress rp ON rp.book_id = ai.book_id AND rp.owner_user_id = ai.owner_user_id`

func listAudioItemsHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	where := []string{"ai.owner_user_id = ?"}
	args := []any{ownerID}
	if category := strings.TrimSpace(r.URL.Query().Get("category")); category != "" {
		if !validAudioCategory(category) {
			errorResponse(w, http.StatusBadRequest, "Invalid audio category")
			return
		}
		where = append(where, "ai.category = ?")
		args = append(args, category)
	}
	if query := strings.TrimSpace(r.URL.Query().Get("q")); query != "" {
		where = append(where, "(ai.title LIKE ? OR ai.artists LIKE ? OR ai.album LIKE ? OR ai.show_title LIKE ? OR bf.path LIKE ?)")
		pattern := "%" + query + "%"
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	if libraryID := strings.TrimSpace(r.URL.Query().Get("library_id")); libraryID != "" {
		where = append(where, "b.library_id = ?")
		args = append(args, libraryID)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		if status != "unplayed" && status != "in_progress" && status != "played" {
			errorResponse(w, http.StatusBadRequest, "Invalid listening status")
			return
		}
		where = append(where, `CASE WHEN ai.category = 'audiobook' THEN
			CASE COALESCE(rp.status, 'unread') WHEN 'reading' THEN 'in_progress' WHEN 'finished' THEN 'played' ELSE 'unplayed' END
			ELSE COALESCE(ls.status, 'unplayed') END = ?`)
		args = append(args, status)
	}
	orderBy := "ai.title COLLATE NOCASE, ai.id"
	switch r.URL.Query().Get("sort") {
	case "recent":
		orderBy = "CASE WHEN ai.category = 'audiobook' THEN COALESCE(rp.updated_at, 0) ELSE COALESCE(ls.last_played_at, 0) END DESC, ai.title COLLATE NOCASE"
	case "album":
		orderBy = "CASE WHEN ai.category = 'podcast' THEN ai.show_title ELSE ai.album END COLLATE NOCASE, CASE WHEN ai.category = 'podcast' THEN COALESCE(ai.episode_number, 0) ELSE COALESCE(ai.disc_number, 0) * 100000 + COALESCE(ai.track_number, 0) END, ai.title COLLATE NOCASE"
	case "published":
		orderBy = "ai.published_at DESC, COALESCE(ai.episode_number, 0) DESC, ai.title COLLATE NOCASE"
	case "added":
		orderBy = "ai.created_at DESC, ai.id DESC"
	}
	rows, err := appDB.Query(audioLibrarySelect+" WHERE "+strings.Join(where, " AND ")+" ORDER BY "+orderBy, args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load audio library")
		return
	}
	defer rows.Close()
	items := []audioLibraryItem{}
	for rows.Next() {
		item, scanErr := scanAudioLibraryItem(rows)
		if scanErr != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to load audio library")
			return
		}
		items = append(items, item)
	}
	jsonResponse(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func getAudioItemHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	audioID, err := strconv.ParseInt(chi.URLParam(r, "audioID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid audio ID")
		return
	}
	item, err := scanAudioLibraryItem(appDB.QueryRow(audioLibrarySelect+" WHERE ai.owner_user_id = ? AND ai.id = ?", ownerID, audioID))
	if errors.Is(err, sql.ErrNoRows) {
		errorResponse(w, http.StatusNotFound, "Audio item not found")
		return
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load audio item")
		return
	}
	chapters, _ := loadAudioChapters(ownerID, audioID)
	bookmarks, _ := loadAudioBookmarks(ownerID, audioID)
	jsonResponse(w, http.StatusOK, map[string]any{"item": item, "chapters": chapters, "bookmarks": bookmarks})
}

type updateAudioItemRequest struct {
	Category      string   `json:"category"`
	Title         string   `json:"title"`
	Artists       []string `json:"artists"`
	AlbumArtist   string   `json:"album_artist"`
	Album         string   `json:"album"`
	TrackNumber   *int     `json:"track_number"`
	DiscNumber    *int     `json:"disc_number"`
	ReleaseDate   string   `json:"release_date"`
	Genre         string   `json:"genre"`
	ShowTitle     string   `json:"show_title"`
	EpisodeNumber *int     `json:"episode_number"`
	PublishedAt   string   `json:"published_at"`
	PlaybackSpeed *float64 `json:"playback_speed"`
}

func updateAudioItemHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	audioID, err := strconv.ParseInt(chi.URLParam(r, "audioID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid audio ID")
		return
	}
	var request updateAudioItemRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid audio metadata")
		return
	}
	if !validAudioCategory(request.Category) || strings.TrimSpace(request.Title) == "" {
		errorResponse(w, http.StatusBadRequest, "Category and title are required")
		return
	}
	if request.PlaybackSpeed != nil && (*request.PlaybackSpeed < .5 || *request.PlaybackSpeed > 3) {
		errorResponse(w, http.StatusBadRequest, "Playback speed must be between 0.5 and 3")
		return
	}
	artistsJSON, _ := json.Marshal(cleanAudioStrings(request.Artists))
	locked, _ := json.Marshal([]string{"category", "title", "artists", "album_artist", "album", "track_number", "disc_number", "release_date", "genre", "show_title", "episode_number", "published_at"})
	result, err := appDB.Exec(`UPDATE audio_item SET category = ?, title = ?, artists = ?, album_artist = ?, album = ?, track_number = ?, disc_number = ?, release_date = ?, genre = ?, show_title = ?, episode_number = ?, published_at = ?, playback_speed = ?, locked_fields = ?, updated_at = ? WHERE id = ? AND owner_user_id = ?`,
		request.Category, strings.TrimSpace(request.Title), string(artistsJSON), strings.TrimSpace(request.AlbumArtist), strings.TrimSpace(request.Album), request.TrackNumber, request.DiscNumber, strings.TrimSpace(request.ReleaseDate), strings.TrimSpace(request.Genre), strings.TrimSpace(request.ShowTitle), request.EpisodeNumber, strings.TrimSpace(request.PublishedAt), request.PlaybackSpeed, string(locked), time.Now().Unix(), audioID, ownerID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio metadata")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		errorResponse(w, http.StatusNotFound, "Audio item not found")
		return
	}
	getAudioItemHandler(w, r)
}

func bulkClassifyAudioHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var request struct {
		IDs      []int64 `json:"ids"`
		Category string  `json:"category"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&request); err != nil || len(request.IDs) == 0 || len(request.IDs) > 1000 || !validAudioCategory(request.Category) {
		errorResponse(w, http.StatusBadRequest, "Select audio items and a valid category")
		return
	}
	placeholders := make([]string, 0, len(request.IDs))
	args := []any{request.Category, time.Now().Unix(), ownerID}
	for _, id := range request.IDs {
		if id <= 0 {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	if len(placeholders) == 0 {
		errorResponse(w, http.StatusBadRequest, "Select audio items")
		return
	}
	result, err := appDB.Exec(`UPDATE audio_item SET category = ?, updated_at = ? WHERE owner_user_id = ? AND id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to classify audio")
		return
	}
	count, _ := result.RowsAffected()
	jsonResponse(w, http.StatusOK, map[string]any{"updated_count": count})
}

func loadAudioChapters(ownerID, audioID int64) ([]audioChapter, error) {
	rows, err := appDB.Query(`SELECT ac.id, ac.position, ac.title, ac.start_seconds, ac.end_seconds FROM audio_chapter ac JOIN audio_item ai ON ai.id = ac.audio_item_id WHERE ai.owner_user_id = ? AND ai.id = ? ORDER BY ac.position`, ownerID, audioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []audioChapter{}
	for rows.Next() {
		var chapter audioChapter
		if err := rows.Scan(&chapter.ID, &chapter.Position, &chapter.Title, &chapter.StartSeconds, &chapter.EndSeconds); err != nil {
			return nil, err
		}
		result = append(result, chapter)
	}
	return result, rows.Err()
}

func getAudioChaptersHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	audioID, _ := strconv.ParseInt(chi.URLParam(r, "audioID"), 10, 64)
	chapters, err := loadAudioChapters(ownerID, audioID)
	if err != nil {
		errorResponse(w, 500, "Failed to load chapters")
		return
	}
	jsonResponse(w, 200, chapters)
}

func loadAudioBookmarks(ownerID, audioID int64) ([]audioBookmark, error) {
	rows, err := appDB.Query(`SELECT id, audio_item_id, seconds, label, created_at FROM audio_bookmark WHERE owner_user_id = ? AND audio_item_id = ? ORDER BY seconds, id`, ownerID, audioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []audioBookmark{}
	for rows.Next() {
		var bookmark audioBookmark
		if err := rows.Scan(&bookmark.ID, &bookmark.AudioID, &bookmark.Seconds, &bookmark.Label, &bookmark.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, bookmark)
	}
	return result, rows.Err()
}

func getAudioBookmarksHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	audioID, _ := strconv.ParseInt(chi.URLParam(r, "audioID"), 10, 64)
	bookmarks, err := loadAudioBookmarks(ownerID, audioID)
	if err != nil {
		errorResponse(w, 500, "Failed to load audio bookmarks")
		return
	}
	jsonResponse(w, 200, bookmarks)
}

func createAudioBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	audioID, err := strconv.ParseInt(chi.URLParam(r, "audioID"), 10, 64)
	if err != nil {
		errorResponse(w, 400, "Invalid audio ID")
		return
	}
	var request struct {
		Seconds float64 `json:"seconds"`
		Label   string  `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Seconds < 0 {
		errorResponse(w, 400, "Invalid bookmark")
		return
	}
	var exists bool
	if err := appDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM audio_item WHERE id = ? AND owner_user_id = ?)`, audioID, ownerID).Scan(&exists); err != nil || !exists {
		errorResponse(w, 404, "Audio item not found")
		return
	}
	result, err := appDB.Exec(`INSERT INTO audio_bookmark (owner_user_id, audio_item_id, seconds, label, created_at) VALUES (?, ?, ?, ?, ?)`, ownerID, audioID, request.Seconds, strings.TrimSpace(request.Label), time.Now().Unix())
	if err != nil {
		errorResponse(w, 500, "Failed to create audio bookmark")
		return
	}
	id, _ := result.LastInsertId()
	jsonResponse(w, 201, map[string]any{"id": id})
}

func deleteAudioBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	result, err := appDB.Exec(`DELETE FROM audio_bookmark WHERE id = ? AND owner_user_id = ?`, chi.URLParam(r, "bookmarkID"), ownerID)
	if err != nil {
		errorResponse(w, 500, "Failed to delete audio bookmark")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		errorResponse(w, 404, "Bookmark not found")
		return
	}
	w.WriteHeader(204)
}

func updateAudioBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	var request struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errorResponse(w, 400, "Invalid bookmark label")
		return
	}
	result, err := appDB.Exec(`UPDATE audio_bookmark SET label = ? WHERE id = ? AND audio_item_id = ? AND owner_user_id = ?`, strings.TrimSpace(request.Label), chi.URLParam(r, "bookmarkID"), chi.URLParam(r, "audioID"), ownerID)
	if err != nil {
		errorResponse(w, 500, "Failed to rename audio bookmark")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		errorResponse(w, 404, "Bookmark not found")
		return
	}
	w.WriteHeader(204)
}

func updateAudioListeningHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	audioID, err := strconv.ParseInt(chi.URLParam(r, "audioID"), 10, 64)
	if err != nil {
		errorResponse(w, 400, "Invalid audio ID")
		return
	}
	var request struct {
		Position  float64 `json:"position_seconds"`
		Duration  float64 `json:"duration_seconds"`
		Status    string  `json:"status"`
		Completed bool    `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Position < 0 || request.Duration < 0 {
		errorResponse(w, 400, "Invalid listening state")
		return
	}
	if request.Completed {
		request.Status = "played"
	}
	if request.Status != "unplayed" && request.Status != "in_progress" && request.Status != "played" {
		request.Status = "in_progress"
	}
	now := time.Now().Unix()
	completed := 0
	if request.Completed {
		completed = 1
	}
	_, err = appDB.Exec(`INSERT INTO audio_listening_state (owner_user_id, audio_item_id, position_seconds, duration_seconds, status, play_count, last_played_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(owner_user_id, audio_item_id) DO UPDATE SET position_seconds = excluded.position_seconds, duration_seconds = excluded.duration_seconds, status = excluded.status, play_count = audio_listening_state.play_count + ?, last_played_at = excluded.last_played_at, updated_at = excluded.updated_at`, ownerID, audioID, request.Position, request.Duration, request.Status, completed, now, now, completed)
	if err != nil {
		errorResponse(w, 500, "Failed to save listening state")
		return
	}
	w.WriteHeader(204)
}

func updateAudioPlaybackSpeedHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	var request struct {
		Speed *float64 `json:"speed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || (request.Speed != nil && (*request.Speed < .5 || *request.Speed > 3)) {
		errorResponse(w, 400, "Playback speed must be between 0.5 and 3")
		return
	}
	var itemID, bookID int64
	var category, showTitle string
	if err := appDB.QueryRow(`SELECT id, book_id, category, show_title FROM audio_item WHERE id = ? AND owner_user_id = ?`, chi.URLParam(r, "audioID"), ownerID).Scan(&itemID, &bookID, &category, &showTitle); err != nil {
		errorResponse(w, 404, "Audio item not found")
		return
	}
	now := time.Now().Unix()
	groupKey := "book:" + strconv.FormatInt(bookID, 10)
	if category == "podcast" && strings.TrimSpace(showTitle) != "" {
		groupKey = "show:" + strings.ToLower(strings.TrimSpace(showTitle))
	}
	if request.Speed == nil {
		_, _ = appDB.Exec(`DELETE FROM audio_playback_preference WHERE owner_user_id = ? AND category = ? AND group_key = ?`, ownerID, category, groupKey)
	} else if category == "audiobook" || category == "podcast" {
		_, _ = appDB.Exec(`INSERT INTO audio_playback_preference (owner_user_id, category, group_key, playback_speed, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(owner_user_id, category, group_key) DO UPDATE SET playback_speed = excluded.playback_speed, updated_at = excluded.updated_at`, ownerID, category, groupKey, *request.Speed, now)
	}
	var result sql.Result
	var err error
	if category == "podcast" && strings.TrimSpace(showTitle) != "" {
		result, err = appDB.Exec(`UPDATE audio_item SET playback_speed = ?, updated_at = ? WHERE owner_user_id = ? AND category = 'podcast' AND LOWER(TRIM(show_title)) = LOWER(TRIM(?))`, request.Speed, now, ownerID, showTitle)
	} else if category == "audiobook" {
		result, err = appDB.Exec(`UPDATE audio_item SET playback_speed = ?, updated_at = ? WHERE owner_user_id = ? AND category = 'audiobook' AND book_id = ?`, request.Speed, now, ownerID, bookID)
	} else {
		result, err = appDB.Exec(`UPDATE audio_item SET playback_speed = ?, updated_at = ? WHERE id = ? AND owner_user_id = ?`, request.Speed, now, itemID, ownerID)
	}
	if err != nil {
		errorResponse(w, 500, "Failed to save playback speed")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		errorResponse(w, 404, "Audio item not found")
		return
	}
	w.WriteHeader(204)
}

type audioPlaylist struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	ItemCount       int     `json:"item_count"`
	DurationSeconds float64 `json:"duration_seconds"`
	CreatedAt       int64   `json:"created_at"`
	UpdatedAt       int64   `json:"updated_at"`
}

func listAudioPlaylistsHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	rows, err := appDB.Query(`SELECT p.id, p.name, p.description, COUNT(pi.id), COALESCE(SUM(ai.duration_seconds), 0), p.created_at, p.updated_at FROM audio_playlist p LEFT JOIN audio_playlist_item pi ON pi.playlist_id = p.id LEFT JOIN audio_item ai ON ai.id = pi.audio_item_id WHERE p.owner_user_id = ? GROUP BY p.id ORDER BY p.name COLLATE NOCASE`, ownerID)
	if err != nil {
		errorResponse(w, 500, "Failed to load playlists")
		return
	}
	defer rows.Close()
	result := []audioPlaylist{}
	for rows.Next() {
		var playlist audioPlaylist
		if rows.Scan(&playlist.ID, &playlist.Name, &playlist.Description, &playlist.ItemCount, &playlist.DurationSeconds, &playlist.CreatedAt, &playlist.UpdatedAt) == nil {
			result = append(result, playlist)
		}
	}
	jsonResponse(w, 200, result)
}

func createAudioPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	var request struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		AudioIDs    []int64 `json:"audio_ids"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || strings.TrimSpace(request.Name) == "" {
		errorResponse(w, 400, "Playlist name is required")
		return
	}
	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, 500, "Failed to create playlist")
		return
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	result, err := tx.Exec(`INSERT INTO audio_playlist (owner_user_id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, ownerID, strings.TrimSpace(request.Name), strings.TrimSpace(request.Description), now, now)
	if err != nil {
		errorResponse(w, 500, "Failed to create playlist")
		return
	}
	playlistID, _ := result.LastInsertId()
	for position, audioID := range uniqueAudioIDs(request.AudioIDs) {
		_, _ = tx.Exec(`INSERT OR IGNORE INTO audio_playlist_item (playlist_id, audio_item_id, position, added_at) SELECT ?, id, ?, ? FROM audio_item WHERE id = ? AND owner_user_id = ?`, playlistID, position, now, audioID, ownerID)
	}
	if tx.Commit() != nil {
		errorResponse(w, 500, "Failed to create playlist")
		return
	}
	jsonResponse(w, 201, map[string]any{"id": playlistID})
}

func updateAudioPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || strings.TrimSpace(request.Name) == "" {
		errorResponse(w, 400, "Playlist name is required")
		return
	}
	result, err := appDB.Exec(`UPDATE audio_playlist SET name = ?, description = ?, updated_at = ? WHERE id = ? AND owner_user_id = ?`, strings.TrimSpace(request.Name), strings.TrimSpace(request.Description), time.Now().Unix(), chi.URLParam(r, "playlistID"), ownerID)
	if err != nil {
		errorResponse(w, 500, "Failed to update playlist")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		errorResponse(w, 404, "Playlist not found")
		return
	}
	w.WriteHeader(204)
}

func deleteAudioPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	result, err := appDB.Exec(`DELETE FROM audio_playlist WHERE id = ? AND owner_user_id = ?`, chi.URLParam(r, "playlistID"), ownerID)
	if err != nil {
		errorResponse(w, 500, "Failed to delete playlist")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		errorResponse(w, 404, "Playlist not found")
		return
	}
	w.WriteHeader(204)
}

func getAudioPlaylistItemsHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	rows, err := appDB.Query(audioLibrarySelect+` JOIN audio_playlist_item pi ON pi.audio_item_id = ai.id JOIN audio_playlist p ON p.id = pi.playlist_id WHERE p.owner_user_id = ? AND p.id = ? ORDER BY pi.position, pi.id`, ownerID, chi.URLParam(r, "playlistID"))
	if err != nil {
		errorResponse(w, 500, "Failed to load playlist")
		return
	}
	defer rows.Close()
	items := []audioLibraryItem{}
	for rows.Next() {
		item, scanErr := scanAudioLibraryItem(rows)
		if scanErr != nil {
			errorResponse(w, 500, "Failed to load playlist")
			return
		}
		items = append(items, item)
	}
	jsonResponse(w, 200, items)
}

func setAudioPlaylistItemsHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := audioQueueOwnerID(r)
	var request struct {
		AudioIDs []int64 `json:"audio_ids"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.AudioIDs) > 5000 {
		errorResponse(w, 400, "Invalid playlist items")
		return
	}
	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, 500, "Failed to update playlist")
		return
	}
	defer tx.Rollback()
	var exists bool
	if tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM audio_playlist WHERE id = ? AND owner_user_id = ?)`, chi.URLParam(r, "playlistID"), ownerID).Scan(&exists) != nil || !exists {
		errorResponse(w, 404, "Playlist not found")
		return
	}
	if _, err = tx.Exec(`DELETE FROM audio_playlist_item WHERE playlist_id = ?`, chi.URLParam(r, "playlistID")); err != nil {
		errorResponse(w, 500, "Failed to update playlist")
		return
	}
	now := time.Now().Unix()
	for position, audioID := range uniqueAudioIDs(request.AudioIDs) {
		if _, err = tx.Exec(`INSERT INTO audio_playlist_item (playlist_id, audio_item_id, position, added_at) SELECT ?, id, ?, ? FROM audio_item WHERE id = ? AND owner_user_id = ?`, chi.URLParam(r, "playlistID"), position, now, audioID, ownerID); err != nil {
			errorResponse(w, 400, "Playlist contains unavailable items")
			return
		}
	}
	_, _ = tx.Exec(`UPDATE audio_playlist SET updated_at = ? WHERE id = ?`, now, chi.URLParam(r, "playlistID"))
	if tx.Commit() != nil {
		errorResponse(w, 500, "Failed to update playlist")
		return
	}
	w.WriteHeader(204)
}

func validAudioCategory(value string) bool {
	return value == "audiobook" || value == "music" || value == "podcast"
}
func cleanAudioStrings(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	return result
}
func uniqueAudioIDs(values []int64) []int64 {
	result := []int64{}
	seen := map[int64]bool{}
	for _, value := range values {
		if value > 0 && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
