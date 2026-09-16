package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func setupAudioQueueTestDB(t *testing.T) {
	t.Helper()
	setupReadingPositionHandlerTestDB(t)
	mustExec(t, `INSERT INTO book_metadata (book_id, title, authors, owner_user_id) VALUES (1, 'Test Audio', '["Narrator"]', 1)`)
	mustExec(t, `INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, owner_user_id) VALUES (12, 1, '/library/test.mp3', 'mp3', 1000, 'audio-one', 100, 1)`)
}

func decodeAudioQueueResponse(t *testing.T, recorder *httptest.ResponseRecorder) audioQueueResponse {
	t.Helper()
	var response audioQueueResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode queue response: %v", err)
	}
	return response
}

func TestAudioQueueAddSelectAndClear(t *testing.T) {
	setupAudioQueueTestDB(t)

	add := httptest.NewRecorder()
	AddAudioQueueItemHandler(add, readingPositionRequest(http.MethodPost, "/api/audio/queue/items", `{"book_id":1,"make_current":true}`, nil))
	if add.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", add.Code, add.Body.String())
	}
	queue := decodeAudioQueueResponse(t, add)
	if len(queue.Items) != 1 || queue.Items[0].FileID != 12 || queue.Items[0].Title != "Test Audio" {
		t.Fatalf("unexpected queue: %+v", queue)
	}
	if queue.CurrentItemID == nil || *queue.CurrentItemID != queue.Items[0].ID {
		t.Fatalf("current item = %v, want %d", queue.CurrentItemID, queue.Items[0].ID)
	}

	clear := httptest.NewRecorder()
	ClearAudioQueueHandler(clear, readingPositionRequest(http.MethodDelete, "/api/audio/queue", "", nil))
	if clear.Code != http.StatusOK {
		t.Fatalf("clear status = %d: %s", clear.Code, clear.Body.String())
	}
	queue = decodeAudioQueueResponse(t, clear)
	if len(queue.Items) != 0 || queue.CurrentItemID != nil {
		t.Fatalf("queue not cleared: %+v", queue)
	}
}

func TestAudioQueueRejectsNonAudioFile(t *testing.T) {
	setupAudioQueueTestDB(t)
	recorder := httptest.NewRecorder()
	AddAudioQueueItemHandler(recorder, readingPositionRequest(http.MethodPost, "/api/audio/queue/items", `{"book_id":1,"file_id":10}`, nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAudioQueueReorderValidatesCompleteOrder(t *testing.T) {
	setupAudioQueueTestDB(t)
	mustExec(t, `INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id) VALUES (2, 1, 100, 100, 1)`)
	mustExec(t, `INSERT INTO book_metadata (book_id, title, authors, owner_user_id) VALUES (2, 'Second Audio', '[]', 1)`)
	mustExec(t, `INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, owner_user_id) VALUES (20, 2, '/library/second.m4b', 'm4b', 1000, 'audio-two', 100, 1)`)

	for _, body := range []string{`{"book_id":1,"file_id":12}`, `{"book_id":2,"file_id":20}`} {
		recorder := httptest.NewRecorder()
		AddAudioQueueItemHandler(recorder, readingPositionRequest(http.MethodPost, "/api/audio/queue/items", body, nil))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("add status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}

	getRecorder := httptest.NewRecorder()
	GetAudioQueueHandler(getRecorder, readingPositionRequest(http.MethodGet, "/api/audio/queue", "", nil))
	queue := decodeAudioQueueResponse(t, getRecorder)
	body := `{"item_ids":[` + jsonNumber(queue.Items[1].ID) + `,` + jsonNumber(queue.Items[0].ID) + `]}`
	reorder := httptest.NewRecorder()
	ReorderAudioQueueHandler(reorder, readingPositionRequest(http.MethodPut, "/api/audio/queue/reorder", body, nil))
	if reorder.Code != http.StatusOK {
		t.Fatalf("reorder status = %d: %s", reorder.Code, reorder.Body.String())
	}
	reordered := decodeAudioQueueResponse(t, reorder)
	if reordered.Items[0].BookID != 2 || reordered.Items[1].BookID != 1 {
		t.Fatalf("unexpected reordered queue: %+v", reordered.Items)
	}

	selectCurrent := httptest.NewRecorder()
	SetAudioQueueCurrentHandler(selectCurrent, readingPositionRequest(http.MethodPut, "/api/audio/queue/current", `{"item_id":`+jsonNumber(reordered.Items[0].ID)+`}`, nil))
	if selectCurrent.Code != http.StatusOK {
		t.Fatalf("set current status = %d: %s", selectCurrent.Code, selectCurrent.Body.String())
	}
	removeCurrent := httptest.NewRecorder()
	DeleteAudioQueueItemHandler(removeCurrent, readingPositionRequest(http.MethodDelete, "/api/audio/queue/items/"+jsonNumber(reordered.Items[0].ID), "", map[string]string{"itemID": jsonNumber(reordered.Items[0].ID)}))
	if removeCurrent.Code != http.StatusOK {
		t.Fatalf("remove current status = %d: %s", removeCurrent.Code, removeCurrent.Body.String())
	}
	afterRemove := decodeAudioQueueResponse(t, removeCurrent)
	if len(afterRemove.Items) != 1 || afterRemove.CurrentItemID == nil || *afterRemove.CurrentItemID != afterRemove.Items[0].ID {
		t.Fatalf("queue did not advance after removing current item: %+v", afterRemove)
	}

	invalid := httptest.NewRecorder()
	ReorderAudioQueueHandler(invalid, readingPositionRequest(http.MethodPut, "/api/audio/queue/reorder", `{"item_ids":[]}`, nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("incomplete order status = %d, want %d", invalid.Code, http.StatusBadRequest)
	}
}

func TestAudioQueueBulkAddSkipsNonAudioAndDuplicates(t *testing.T) {
	setupAudioQueueTestDB(t)
	mustExec(t, `INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id) VALUES (2, 1, 100, 100, 1), (3, 1, 100, 100, 1)`)
	mustExec(t, `INSERT INTO book_metadata (book_id, title, authors, owner_user_id) VALUES (2, 'Second Audio', '[]', 1), (3, 'Text Book', '[]', 1)`)
	mustExec(t, `INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, owner_user_id) VALUES (20, 2, '/library/second.m4b', 'm4b', 1000, 'audio-two', 100, 1), (30, 3, '/library/text.epub', 'epub', 1000, 'text-three', 100, 1)`)

	recorder := httptest.NewRecorder()
	AddAudioQueueItemsBulkHandler(recorder, readingPositionRequest(http.MethodPost, "/api/audio/queue/items/bulk", `{"book_ids":[1,2,3,2]}`, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("bulk add status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response audioQueueBulkResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode bulk response: %v", err)
	}
	if response.AddedCount != 2 || response.SkippedCount != 2 || len(response.Items) != 2 {
		t.Fatalf("unexpected bulk response: %+v", response)
	}
	if response.CurrentItemID == nil || *response.CurrentItemID != response.Items[0].ID {
		t.Fatalf("bulk add did not select the first audio item: %+v", response)
	}
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
