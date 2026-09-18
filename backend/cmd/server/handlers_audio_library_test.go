package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupAudioLibraryTestDB(t *testing.T) {
	t.Helper()
	setupAudioQueueTestDB(t)
	mustExec(t, `INSERT INTO audio_item (id, owner_user_id, book_id, file_id, category, title, artists, album_artist, album, duration_seconds, show_title, metadata_source, locked_fields, source_hash, created_at, updated_at) VALUES (100, 1, 1, 12, 'audiobook', 'Test Audio', '["Narrator"]', '', '', 3600, '', 'test', '[]', 'audio-one', 100, 100)`)
}

func TestAudioLibraryListClassifyAndUpdate(t *testing.T) {
	setupAudioLibraryTestDB(t)
	mustExec(t, `INSERT INTO reading_progress (book_id, file_id, percent, status, updated_at, owner_user_id) VALUES (1, 12, 25, 'reading', 100, 1)`)

	list := httptest.NewRecorder()
	listAudioItemsHandler(list, readingPositionRequest(http.MethodGet, "/api/audio/items?category=audiobook", "", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body.String())
	}
	var response struct {
		Items []audioLibraryItem `json:"items"`
	}
	if err := json.NewDecoder(list.Body).Decode(&response); err != nil || len(response.Items) != 1 {
		t.Fatalf("unexpected list response: %+v, %v", response, err)
	}
	if response.Items[0].ID != 100 || response.Items[0].StreamURL == "" || response.Items[0].Status != "in_progress" {
		t.Fatalf("unexpected audio item: %+v", response.Items[0])
	}

	classify := httptest.NewRecorder()
	bulkClassifyAudioHandler(classify, readingPositionRequest(http.MethodPost, "/api/audio/items/classify", `{"ids":[100],"category":"music"}`, nil))
	if classify.Code != http.StatusOK {
		t.Fatalf("classify status = %d: %s", classify.Code, classify.Body.String())
	}

	update := httptest.NewRecorder()
	updateAudioItemHandler(update, readingPositionRequest(http.MethodPut, "/api/audio/items/100", `{"category":"music","title":"Updated Track","artists":["Artist"],"album_artist":"Artist","album":"Album","genre":"Ambient"}`, map[string]string{"audioID": "100"}))
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	var category, title, locked string
	if err := appDB.QueryRow(`SELECT category, title, locked_fields FROM audio_item WHERE id = 100`).Scan(&category, &title, &locked); err != nil {
		t.Fatal(err)
	}
	if category != "music" || title != "Updated Track" || locked == "[]" {
		t.Fatalf("updated values = %q, %q, %q", category, title, locked)
	}
}

func TestAudioPlaylistAndQueueReplacement(t *testing.T) {
	setupAudioLibraryTestDB(t)

	create := httptest.NewRecorder()
	createAudioPlaylistHandler(create, readingPositionRequest(http.MethodPost, "/api/audio/playlists", `{"name":"Focus","audio_ids":[100,100]}`, nil))
	if create.Code != http.StatusCreated {
		t.Fatalf("create playlist status = %d: %s", create.Code, create.Body.String())
	}
	items := httptest.NewRecorder()
	getAudioPlaylistItemsHandler(items, readingPositionRequest(http.MethodGet, "/api/audio/playlists/1/items", "", map[string]string{"playlistID": "1"}))
	if items.Code != http.StatusOK {
		t.Fatalf("playlist items status = %d: %s", items.Code, items.Body.String())
	}
	var playlistItems []audioLibraryItem
	if err := json.NewDecoder(items.Body).Decode(&playlistItems); err != nil || len(playlistItems) != 1 {
		t.Fatalf("playlist items = %+v, %v", playlistItems, err)
	}

	replace := httptest.NewRecorder()
	ReplaceAudioQueueHandler(replace, readingPositionRequest(http.MethodPut, "/api/audio/queue/replace", `{"audio_ids":[100]}`, nil))
	if replace.Code != http.StatusOK {
		t.Fatalf("replace queue status = %d: %s", replace.Code, replace.Body.String())
	}
	queue := decodeAudioQueueResponse(t, replace)
	if len(queue.Items) != 1 || queue.Items[0].AudioID == nil || *queue.Items[0].AudioID != 100 {
		t.Fatalf("replacement queue = %+v", queue)
	}
}

func TestAudioListeningAndBookmarksRemainSeparateFromReadingProgress(t *testing.T) {
	setupAudioLibraryTestDB(t)
	mustExec(t, `UPDATE audio_item SET category = 'podcast' WHERE id = 100`)

	listen := httptest.NewRecorder()
	updateAudioListeningHandler(listen, readingPositionRequest(http.MethodPut, "/api/audio/items/100/listening", `{"position_seconds":90,"duration_seconds":3600,"status":"in_progress"}`, map[string]string{"audioID": "100"}))
	if listen.Code != http.StatusNoContent {
		t.Fatalf("listening status = %d: %s", listen.Code, listen.Body.String())
	}
	var position float64
	if err := appDB.QueryRow(`SELECT position_seconds FROM audio_listening_state WHERE audio_item_id = 100`).Scan(&position); err != nil || position != 90 {
		t.Fatalf("listening position = %v, %v", position, err)
	}
	var readingCount int
	_ = appDB.QueryRow(`SELECT COUNT(*) FROM reading_position WHERE file_id = 12`).Scan(&readingCount)
	if readingCount != 0 {
		t.Fatalf("podcast listening created %d reading positions", readingCount)
	}

	bookmark := httptest.NewRecorder()
	createAudioBookmarkHandler(bookmark, readingPositionRequest(http.MethodPost, "/api/audio/items/100/bookmarks", `{"seconds":95,"label":"Quote"}`, map[string]string{"audioID": "100"}))
	if bookmark.Code != http.StatusCreated {
		t.Fatalf("bookmark status = %d: %s", bookmark.Code, bookmark.Body.String())
	}
	bookmarks, err := loadAudioBookmarks(1, 100)
	if err != nil || len(bookmarks) != 1 || bookmarks[0].Label != "Quote" {
		t.Fatalf("bookmarks = %+v, %v", bookmarks, err)
	}
}
