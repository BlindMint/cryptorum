package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatsUseActiveTimeAndExcludeUntrackedSessions(t *testing.T) {
	setupReadingPositionHandlerTestDB(t)
	mustExec(t, `
		INSERT INTO reading_session (
			book_id, started_at, ended_at, owner_user_id, activity_tracked, active_seconds
		) VALUES (1, 100, 3700, 1, 1, 3600),
		         (1, 100, 1000000, 1, 0, 0)
	`)
	statsResponseCache.Lock()
	statsResponseCache.entries = make(map[string]cachedStatsResponse)
	statsResponseCache.Unlock()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	GetStatsHandler(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("stats status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		TotalSessionMinutes   int64   `json:"total_session_minutes"`
		AverageSessionMinutes float64 `json:"average_session_minutes"`
		UntrackedSessions     int64   `json:"untracked_sessions"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if response.TotalSessionMinutes != 60 || response.AverageSessionMinutes != 60 {
		t.Fatalf("active time total=%d average=%v, want 60 minutes", response.TotalSessionMinutes, response.AverageSessionMinutes)
	}
	if response.UntrackedSessions != 1 {
		t.Fatalf("untracked sessions = %d, want 1", response.UntrackedSessions)
	}
}
