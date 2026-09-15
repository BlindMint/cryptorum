package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPreviewSourceCoverNotFound(t *testing.T) {
	setupCombineTestDB(t)
	insertCombineTestBook(t, 10, 1, "No Cover Book", "/missing/book.pdf", "pdf")

	req := httptest.NewRequest(http.MethodGet, "/api/books/10/cover/source", nil)
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("bookID", "10")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	PreviewSourceCoverHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}
