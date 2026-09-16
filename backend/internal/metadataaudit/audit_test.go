package metadataaudit

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestParseFilename(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		title     string
		authors   []string
		pattern   string
		ambiguous bool
	}{
		{
			name:    "dash author",
			path:    "/books/Making Money - Terry Pratchett.mobi",
			title:   "Making Money",
			authors: []string{"Terry Pratchett"},
			pattern: "final dash author",
		},
		{
			name:    "parenthetical author",
			path:    "/books/Secret History The Story of Cryptology (Craig P. Bauer).pdf",
			title:   "Secret History The Story of Cryptology",
			authors: []string{"Craig P. Bauer"},
			pattern: "trailing parenthetical author",
		},
		{
			name:    "final parentheses beat subtitle dash",
			path:    "/books/The Circus - MI5 Operations 1945-1972 (Nigel West).pdf",
			title:   "The Circus - MI5 Operations 1945-1972",
			authors: []string{"Nigel West"},
			pattern: "trailing parenthetical author",
		},
		{
			name:    "series parentheses before dash author",
			path:    "/books/Mimic Arcanist (Astra Academy 2) - Shami Stovall.epub",
			title:   "Mimic Arcanist (Astra Academy 2)",
			authors: []string{"Shami Stovall"},
			pattern: "final dash author",
		},
		{
			name:    "by author and chained extension",
			path:    "/books/The political economy of Stalinism by Paul R. Gregory.pdf.epub",
			title:   "The political economy of Stalinism",
			authors: []string{"Paul R. Gregory"},
			pattern: "by author",
		},
		{
			name:      "comma list remains one ambiguous value",
			path:      "/books/Particle Physics - Brian Martin, Graham Shaw.pdf",
			title:     "Particle Physics",
			authors:   []string{"Brian Martin, Graham Shaw"},
			pattern:   "final dash author",
			ambiguous: true,
		},
		{
			name:    "series title is not mistaken for an author",
			path:    "/books/Series 01 - The Beginning.cbz",
			title:   "Series 01 - The Beginning",
			pattern: "filename title",
		},
		{
			name:    "author first dash is left for review",
			path:    "/books/DiLorenzo - The Antitrust Economists Paradox.pdf",
			title:   "DiLorenzo - The Antitrust Economists Paradox",
			pattern: "filename title",
		},
		{
			name:    "duplicate suffix is removed from author",
			path:    "/books/Kingdom of the Sands - Ringo Hunnigan (1).epub",
			title:   "Kingdom of the Sands",
			authors: []string{"Ringo Hunnigan"},
			pattern: "final dash author",
		},
		{
			name:    "acronym in parentheses remains in title",
			path:    "/books/Mastering Azure Kubernetes Service (AKS).epub",
			title:   "Mastering Azure Kubernetes Service (AKS)",
			pattern: "filename title",
		},
		{
			name:    "year phrase remains with dash author",
			path:    "/books/01 - Black Mass Volume I - vx-underground (Halloween 2022).pdf",
			title:   "01 - Black Mass Volume I",
			authors: []string{"vx-underground (Halloween 2022)"},
			pattern: "final dash author",
		},
		{
			name:    "source tag after dash author is ignored",
			path:    "/books/Political Ideologies An Introduction - Andrew Heywood (Z-Library).epub",
			title:   "Political Ideologies An Introduction",
			authors: []string{"Andrew Heywood"},
			pattern: "final dash author",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseFilename(test.path)
			if got.Title != test.title || !sameAuthorList(got.Authors, test.authors) || got.Pattern != test.pattern || got.AmbiguousAuthors != test.ambiguous {
				t.Fatalf("parseFilename(%q) = %+v, want title=%q authors=%#v pattern=%q ambiguous=%v", test.path, got, test.title, test.authors, test.pattern, test.ambiguous)
			}
		})
	}
}

func TestProposeRespectsLocksAndRevisionHistory(t *testing.T) {
	base := inventoryItem{
		BookID: 7, Library: "Test", Path: "/books/Artificial Intelligence - Melanie Mitchell.epub",
		Title: "Artificial Intelligence", AuthorsRaw: "[]", LockedRaw: "[]",
	}
	proposal, skipped, scoped := propose(base)
	if !scoped || skipped != "" || !sameAuthorList(proposal.ProposedAuthors, []string{"Melanie Mitchell"}) {
		t.Fatalf("expected author proposal, got proposal=%+v skipped=%q scoped=%v", proposal, skipped, scoped)
	}

	locked := base
	locked.LockedRaw = `["authors"]`
	if _, reason, scoped := propose(locked); !scoped || !strings.Contains(reason, "locked") {
		t.Fatalf("locked author reason=%q scoped=%v", reason, scoped)
	}

	edited := base
	edited.AuthorsEdited = true
	if _, reason, scoped := propose(edited); !scoped || !strings.Contains(reason, "revision history") {
		t.Fatalf("edited author reason=%q scoped=%v", reason, scoped)
	}
}

func TestProposeCleansFilenameAuthorFromTitleAndRecognizesOpaqueTitles(t *testing.T) {
	byAuthor := inventoryItem{
		BookID: 8, Library: "Test", Path: "/books/Black Hat Python by Justin Seitz.pdf",
		Title: "Black Hat Python by Justin Seitz", AuthorsRaw: "[]", LockedRaw: "[]",
	}
	proposal, skipped, scoped := propose(byAuthor)
	if !scoped || skipped != "" || proposal.ProposedTitle != "Black Hat Python" ||
		!sameAuthorList(proposal.ProposedAuthors, []string{"Justin Seitz"}) {
		t.Fatalf("unexpected by-author proposal: %+v skipped=%q scoped=%v", proposal, skipped, scoped)
	}
	if !contains(proposal.AllowedFields, "title") || !contains(proposal.AllowedFields, "authors") {
		t.Fatalf("expected title and authors changes: %+v", proposal.AllowedFields)
	}

	opaque := inventoryItem{
		BookID: 9, Library: "Test", Path: "/books/Mastering Data Science - Daniel Huston.epub",
		Title: "B0C46X8YKT", AuthorsRaw: `["Unknown"]`, LockedRaw: "[]",
	}
	proposal, skipped, scoped = propose(opaque)
	if !scoped || skipped != "" || proposal.ProposedTitle != "Mastering Data Science" {
		t.Fatalf("unexpected opaque-title proposal: %+v skipped=%q scoped=%v", proposal, skipped, scoped)
	}

	conflict := inventoryItem{
		BookID: 10, Library: "Test", Path: "/books/Fallacy of the Public Sector by Murray Rothbard.pdf",
		Title: "PUBLIC.PDF", AuthorsRaw: `["Jeff Tucker"]`, LockedRaw: "[]",
	}
	proposal, skipped, scoped = propose(conflict)
	if !scoped || skipped != "" || !proposal.ExistingAuthorConflict || proposal.Confidence != "ambiguous" ||
		!sameAuthorList(proposal.ProposedAuthors, []string{"Murray Rothbard"}) {
		t.Fatalf("unexpected conflicting-author proposal: %+v skipped=%q scoped=%v", proposal, skipped, scoped)
	}
}

func TestWeightedReviewOrderIncludesEveryConfidenceInPilot(t *testing.T) {
	proposals := make([]Proposal, 0, 40)
	for _, confidence := range []string{"high", "medium", "ambiguous"} {
		for index := 0; index < 20; index++ {
			proposals = append(proposals, Proposal{BookID: int64(len(proposals) + 1), Confidence: confidence})
		}
	}
	ordered := weightedReviewOrder(proposals)
	counts := map[string]int{}
	for _, proposal := range ordered[:20] {
		counts[proposal.Confidence]++
	}
	if counts["high"] != 12 || counts["medium"] != 5 || counts["ambiguous"] != 3 {
		t.Fatalf("first review cycle counts = %#v", counts)
	}
}

func TestCompactReviewIsReadableWithinTerminalWidth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-compact.txt")
	proposal := Proposal{
		BookID: 42, Path: "/books/A Very Long Filename With Enough Words To Wrap Cleanly Across A Narrow Terminal Without Horizontal Scrolling - Ada Lovelace.epub",
		CurrentTitle: "Unknown", ProposedTitle: "A Very Long Filename With Enough Words To Wrap Cleanly Across A Narrow Terminal Without Horizontal Scrolling",
		CurrentAuthors: []string{"Unknown"}, ProposedAuthors: []string{"Ada Lovelace"},
		AllowedFields: []string{"title", "authors"}, Confidence: "high",
	}
	if err := writeCompactReview(path, []Proposal{proposal}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for lineNumber, line := range strings.Split(content, "\n") {
		if len([]rune(line)) > 120 {
			t.Fatalf("line %d is %d columns: %q", lineNumber+1, len([]rune(line)), line)
		}
	}
	for _, expected := range []string{"Book 42 | HIGH", "Current", "Unknown", "Proposed", "Ada Lovelace"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("compact review missing %q:\n%s", expected, content)
		}
	}
}

func TestGenerateAndApplyApprovedMetadata(t *testing.T) {
	dbPath, db := setupAuditDB(t)
	insertAuditBook(t, db, 1, "/books/Artificial Intelligence - Melanie Mitchell.epub", "Artificial Intelligence", "[]")

	output := filepath.Join(t.TempDir(), "batch")
	result, err := Generate(context.Background(), db, GenerateOptions{SourceDB: dbPath, Output: output, Limit: 100})
	if err != nil {
		t.Fatalf("generate audit: %v", err)
	}
	if result.Selected != 1 {
		t.Fatalf("selected = %d, want 1", result.Selected)
	}
	approveAll(t, result.ReviewPath)

	applyResult, err := Apply(context.Background(), db, ApplyOptions{
		SourceDB: dbPath, ManifestPath: result.ManifestPath, DecisionsPath: result.ReviewPath,
		BackupDir: filepath.Join(t.TempDir(), "backups"), ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("apply audit: %v", err)
	}
	if applyResult.Updated != 1 {
		t.Fatalf("updated = %d, want 1", applyResult.Updated)
	}
	if _, err := os.Stat(applyResult.BackupPath); err != nil {
		t.Fatalf("pre-apply backup missing: %v", err)
	}

	var authors, locked string
	if err := db.QueryRow(`SELECT authors, locked_fields FROM book_metadata WHERE book_id = 1`).Scan(&authors, &locked); err != nil {
		t.Fatal(err)
	}
	if authors != `["Melanie Mitchell"]` || locked != `["authors"]` {
		t.Fatalf("authors=%s locked=%s", authors, locked)
	}
	var source, changedFields string
	if err := db.QueryRow(`SELECT change_source, changed_fields FROM book_metadata_revision WHERE book_id = 1`).Scan(&source, &changedFields); err != nil {
		t.Fatal(err)
	}
	if source != "filename_audit" || changedFields != `["authors"]` {
		t.Fatalf("source=%q changed_fields=%s", source, changedFields)
	}
	var indexedMatches int
	if err := db.QueryRow(`SELECT COUNT(*) FROM book_fts WHERE rowid = 1 AND book_fts MATCH 'Melanie'`).Scan(&indexedMatches); err != nil {
		t.Fatal(err)
	}
	if indexedMatches != 1 {
		t.Fatalf("indexed author matches = %d", indexedMatches)
	}
}

func TestApplyRollsBackWholeBatchWhenRowIsStale(t *testing.T) {
	dbPath, db := setupAuditDB(t)
	insertAuditBook(t, db, 1, "/books/First Book - Ada Lovelace.epub", "First Book", "[]")
	insertAuditBook(t, db, 2, "/books/Second Book - Grace Hopper.epub", "Second Book", "[]")

	output := filepath.Join(t.TempDir(), "batch")
	result, err := Generate(context.Background(), db, GenerateOptions{SourceDB: dbPath, Output: output, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	approveAll(t, result.ReviewPath)
	if _, err := db.Exec(`UPDATE book_metadata SET title = 'Changed after generation' WHERE book_id = 2`); err != nil {
		t.Fatal(err)
	}

	_, err = Apply(context.Background(), db, ApplyOptions{
		SourceDB: dbPath, ManifestPath: result.ManifestPath, DecisionsPath: result.ReviewPath,
		BackupDir: filepath.Join(t.TempDir(), "backups"), ActorUserID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("expected stale-row error, got %v", err)
	}
	var authors string
	if err := db.QueryRow(`SELECT authors FROM book_metadata WHERE book_id = 1`).Scan(&authors); err != nil {
		t.Fatal(err)
	}
	if authors != "[]" {
		t.Fatalf("first row was not rolled back: authors=%s", authors)
	}
	var revisions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM book_metadata_revision`).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 0 {
		t.Fatalf("revision count = %d after rollback", revisions)
	}
}

func setupAuditDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cryptorum.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := `
		CREATE TABLE library (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE book (id INTEGER PRIMARY KEY, library_id INTEGER NOT NULL, owner_user_id INTEGER NOT NULL DEFAULT 1);
		CREATE TABLE book_file (id INTEGER PRIMARY KEY, book_id INTEGER NOT NULL, path TEXT NOT NULL, missing_at INTEGER);
		CREATE TABLE book_metadata (
			id INTEGER PRIMARY KEY, book_id INTEGER NOT NULL UNIQUE, title TEXT, authors TEXT,
			series TEXT, series_number REAL, series_number_display TEXT, publisher TEXT, pub_date TEXT,
			description TEXT, rating REAL, genres TEXT, tags TEXT, isbn TEXT, asin TEXT, language TEXT,
			page_count INTEGER, cover_path TEXT, cover_source TEXT NOT NULL DEFAULT '', cover_updated_on INTEGER NOT NULL DEFAULT 0,
			locked_fields TEXT, extracted_from_hash TEXT NOT NULL DEFAULT '', metadata_updated_at INTEGER NOT NULL DEFAULT 0,
			owner_user_id INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE book_metadata_revision (
			id INTEGER PRIMARY KEY, book_id INTEGER NOT NULL, changed_at INTEGER NOT NULL,
			changed_by_user_id INTEGER, change_source TEXT NOT NULL, changed_fields TEXT NOT NULL,
			previous_metadata_json TEXT NOT NULL
		);
		CREATE TABLE app_log (
			id INTEGER PRIMARY KEY, level TEXT NOT NULL, category TEXT NOT NULL, message TEXT NOT NULL,
			data_json TEXT, created_at INTEGER NOT NULL
		);
		CREATE VIRTUAL TABLE book_fts USING fts5(
			title, authors, description, series, content='book_metadata', content_rowid='id'
		);
		INSERT INTO library (id, name) VALUES (1, 'Test Library');
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return absPath, db
}

func insertAuditBook(t *testing.T, db *sql.DB, id int64, path, title, authors string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO book (id, library_id) VALUES (?, 1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO book_file (id, book_id, path) VALUES (?, ?, ?)`, id, id, path); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO book_metadata (id, book_id, title, authors, series, description, genres, tags, locked_fields, metadata_updated_at)
		VALUES (?, ?, ?, ?, '', '', '[]', '[]', '[]', 7)
	`, id, id, title, authors); err != nil {
		t.Fatal(err)
	}
	var parsedAuthors []string
	_ = json.Unmarshal([]byte(authors), &parsedAuthors)
	if _, err := db.Exec(`INSERT INTO book_fts(rowid, title, authors, description, series) VALUES (?, ?, ?, '', '')`, id, title, strings.Join(parsedAuthors, " ")); err != nil {
		t.Fatal(err)
	}
}

func approveAll(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(file).ReadAll()
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	decisionColumn := -1
	for index, value := range rows[0] {
		if value == "decision" {
			decisionColumn = index
		}
	}
	if decisionColumn < 0 {
		t.Fatal("decision column missing")
	}
	for index := 1; index < len(rows); index++ {
		rows[index][decisionColumn] = "approve"
	}
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(output)
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}
