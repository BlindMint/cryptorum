package metadataaudit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cryptorum/internal/filenameinfo"
	"cryptorum/internal/metaprotection"
)

const ManifestVersion = 1

var spacePattern = regexp.MustCompile(`\s+`)

type Manifest struct {
	Version     int        `json:"version"`
	GeneratedAt int64      `json:"generated_at"`
	SourceDB    string     `json:"source_db"`
	Offset      int        `json:"offset"`
	Limit       int        `json:"limit"`
	Proposals   []Proposal `json:"proposals"`
}

type Proposal struct {
	BookID                 int64    `json:"book_id"`
	Library                string   `json:"library"`
	Path                   string   `json:"path"`
	CurrentTitle           string   `json:"current_title"`
	ProposedTitle          string   `json:"proposed_title"`
	CurrentAuthors         []string `json:"current_authors"`
	CurrentAuthorsRaw      string   `json:"current_authors_raw"`
	ProposedAuthors        []string `json:"proposed_authors"`
	CurrentLocked          string   `json:"current_locked_fields"`
	CurrentUpdatedAt       int64    `json:"current_metadata_updated_at"`
	AllowedFields          []string `json:"allowed_fields"`
	Confidence             string   `json:"confidence"`
	Reasons                []string `json:"reasons"`
	FilenamePattern        string   `json:"filename_pattern"`
	AuthorListAmbiguous    bool     `json:"author_list_ambiguous"`
	ExistingAuthorConflict bool     `json:"existing_author_conflict"`
}

type SkippedItem struct {
	BookID  int64
	Library string
	Path    string
	Title   string
	Authors []string
	Reason  string
}

type GenerateOptions struct {
	SourceDB string
	Output   string
	Limit    int
	Offset   int
}

type GenerateResult struct {
	ManifestPath string
	ReviewPath   string
	CompactPath  string
	SummaryPath  string
	SkippedPath  string
	Checksum     string
	Selected     int
	Candidates   int
	Scoped       int
	Skipped      int
}

type ApplyOptions struct {
	SourceDB      string
	ManifestPath  string
	DecisionsPath string
	BackupDir     string
	ActorUserID   int64
}

type ApplyResult struct {
	Approved   int
	Updated    int
	Rejected   int
	Unreviewed int
	BackupPath string
}

type filenameCandidate = filenameinfo.Analysis

type inventoryItem struct {
	BookID            int64
	Library           string
	Path              string
	Title             string
	AuthorsRaw        string
	LockedRaw         string
	MetadataUpdatedAt int64
	TitleEdited       bool
	AuthorsEdited     bool
}

type connDBTX struct {
	conn *sql.Conn
}

func (db connDBTX) Exec(query string, args ...any) (sql.Result, error) {
	return db.conn.ExecContext(context.Background(), query, args...)
}

func (db connDBTX) QueryRow(query string, args ...any) *sql.Row {
	return db.conn.QueryRowContext(context.Background(), query, args...)
}

func Generate(ctx context.Context, db *sql.DB, options GenerateOptions) (GenerateResult, error) {
	if options.Limit <= 0 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		return GenerateResult{}, fmt.Errorf("offset must not be negative")
	}
	if strings.TrimSpace(options.Output) == "" {
		return GenerateResult{}, fmt.Errorf("output directory is required")
	}

	items, err := loadInventory(ctx, db)
	if err != nil {
		return GenerateResult{}, err
	}

	proposals := make([]Proposal, 0)
	skipped := make([]SkippedItem, 0)
	scoped := 0
	for _, item := range items {
		proposal, skipReason, inScope := propose(item)
		if !inScope {
			continue
		}
		scoped++
		if skipReason != "" {
			skipped = append(skipped, skippedFromInventory(item, skipReason))
			continue
		}
		proposals = append(proposals, proposal)
	}

	sort.SliceStable(proposals, func(i, j int) bool {
		left := confidenceRank(proposals[i].Confidence)
		right := confidenceRank(proposals[j].Confidence)
		if left != right {
			return left < right
		}
		return proposals[i].BookID < proposals[j].BookID
	})
	proposals = weightedReviewOrder(proposals)

	allCandidateCount := len(proposals)
	start := min(options.Offset, len(proposals))
	end := min(start+options.Limit, len(proposals))
	selected := append([]Proposal(nil), proposals[start:end]...)
	manifest := Manifest{
		Version:     ManifestVersion,
		GeneratedAt: time.Now().Unix(),
		SourceDB:    filepath.Clean(options.SourceDB),
		Offset:      options.Offset,
		Limit:       options.Limit,
		Proposals:   selected,
	}

	if err := os.MkdirAll(options.Output, 0o755); err != nil {
		return GenerateResult{}, fmt.Errorf("create output directory: %w", err)
	}
	manifestPath := filepath.Join(options.Output, "manifest.json")
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return GenerateResult{}, fmt.Errorf("encode manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return GenerateResult{}, fmt.Errorf("write manifest: %w", err)
	}
	checksum := sha256.Sum256(manifestBytes)
	checksumText := hex.EncodeToString(checksum[:])

	reviewPath := filepath.Join(options.Output, "review.csv")
	if err := writeReviewCSV(reviewPath, checksumText, selected); err != nil {
		return GenerateResult{}, err
	}
	compactPath := filepath.Join(options.Output, "review-compact.txt")
	if err := writeCompactReview(compactPath, selected); err != nil {
		return GenerateResult{}, err
	}
	skippedPath := filepath.Join(options.Output, "skipped.csv")
	if err := writeSkippedCSV(skippedPath, skipped); err != nil {
		return GenerateResult{}, err
	}
	summaryPath := filepath.Join(options.Output, "summary.md")
	if err := writeSummary(summaryPath, manifest, checksumText, len(items), scoped, allCandidateCount, len(skipped)); err != nil {
		return GenerateResult{}, err
	}

	return GenerateResult{
		ManifestPath: manifestPath,
		ReviewPath:   reviewPath,
		CompactPath:  compactPath,
		SummaryPath:  summaryPath,
		SkippedPath:  skippedPath,
		Checksum:     checksumText,
		Selected:     len(selected),
		Candidates:   allCandidateCount,
		Scoped:       scoped,
		Skipped:      len(skipped),
	}, nil
}

func loadInventory(ctx context.Context, db *sql.DB) ([]inventoryItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT b.id, l.name, COALESCE(bf.path, ''),
		       COALESCE(bm.title, ''), COALESCE(bm.authors, '[]'),
		       COALESCE(bm.locked_fields, '[]'), COALESCE(bm.metadata_updated_at, 0),
		       EXISTS (
		           SELECT 1 FROM book_metadata_revision r, json_each(r.changed_fields) field
		           WHERE r.book_id = b.id
		             AND r.change_source IN ('manual_edit', 'bulk_edit', 'provider_apply', 'revision_restore')
		             AND field.value = 'title'
		       ),
		       EXISTS (
		           SELECT 1 FROM book_metadata_revision r, json_each(r.changed_fields) field
		           WHERE r.book_id = b.id
		             AND r.change_source IN ('manual_edit', 'bulk_edit', 'provider_apply', 'revision_restore')
		             AND field.value = 'authors'
		       )
		FROM book b
		JOIN library l ON l.id = b.library_id
		LEFT JOIN book_metadata bm ON bm.book_id = b.id
		LEFT JOIN book_file bf ON bf.id = (
			SELECT candidate.id FROM book_file candidate
			WHERE candidate.book_id = b.id AND candidate.missing_at IS NULL
			ORDER BY candidate.id LIMIT 1
		)
		ORDER BY b.id
	`)
	if err != nil {
		return nil, fmt.Errorf("load metadata inventory: %w", err)
	}
	defer rows.Close()

	items := make([]inventoryItem, 0)
	for rows.Next() {
		var item inventoryItem
		if err := rows.Scan(
			&item.BookID, &item.Library, &item.Path, &item.Title, &item.AuthorsRaw,
			&item.LockedRaw, &item.MetadataUpdatedAt, &item.TitleEdited, &item.AuthorsEdited,
		); err != nil {
			return nil, fmt.Errorf("scan metadata inventory: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metadata inventory: %w", err)
	}
	return items, nil
}

func propose(item inventoryItem) (Proposal, string, bool) {
	authors, authorsValid := parseAuthors(item.AuthorsRaw)
	titleSuspicious := suspiciousTitle(item.Title)
	authorsSuspicious := suspiciousAuthors(authors, authorsValid) || (len(authors) == 1 && sameText(authors[0], item.Title))
	if !titleSuspicious && !authorsSuspicious {
		return Proposal{}, "", false
	}
	if item.Path == "" {
		return Proposal{}, "no active file", true
	}

	candidate := parseFilename(item.Path)
	authorConflict := titleSuspicious && candidate.Pattern != "filename title" && len(candidate.Authors) > 0 &&
		!suspiciousAuthors(candidate.Authors, true) && !sameAuthorList(candidate.Authors, authors) && !authorsSuspicious
	locked := metaprotection.ParseLocked(item.LockedRaw)
	proposal := Proposal{
		BookID:                 item.BookID,
		Library:                item.Library,
		Path:                   item.Path,
		CurrentTitle:           item.Title,
		ProposedTitle:          item.Title,
		CurrentAuthors:         authors,
		CurrentAuthorsRaw:      item.AuthorsRaw,
		ProposedAuthors:        append([]string{}, authors...),
		CurrentLocked:          item.LockedRaw,
		CurrentUpdatedAt:       item.MetadataUpdatedAt,
		FilenamePattern:        candidate.Pattern,
		AuthorListAmbiguous:    candidate.AmbiguousAuthors,
		ExistingAuthorConflict: authorConflict,
	}

	blocked := make([]string, 0)
	titleFromFilename := filenameinfo.Stem(item.Path)
	titleIncludesAuthor := authorsSuspicious && candidate.Pattern != "filename title" && sameText(item.Title, titleFromFilename)
	if titleSuspicious || titleIncludesAuthor {
		switch {
		case locked[metaprotection.FieldTitle]:
			blocked = append(blocked, "title is locked")
		case item.TitleEdited:
			blocked = append(blocked, "title has user/provider revision history")
		case strings.TrimSpace(candidate.Title) == "":
			blocked = append(blocked, "filename did not yield a title")
		case !sameText(candidate.Title, item.Title):
			proposal.ProposedTitle = candidate.Title
			proposal.AllowedFields = append(proposal.AllowedFields, metaprotection.FieldTitle)
			if titleIncludesAuthor && !titleSuspicious {
				proposal.Reasons = append(proposal.Reasons, "current title includes the filename author suffix")
			} else {
				proposal.Reasons = append(proposal.Reasons, titleReason(item.Title))
			}
		}
	}
	if authorsSuspicious || authorConflict {
		switch {
		case locked[metaprotection.FieldAuthors]:
			blocked = append(blocked, "authors are locked")
		case item.AuthorsEdited:
			blocked = append(blocked, "authors have user/provider revision history")
		case len(candidate.Authors) == 0 || suspiciousAuthors(candidate.Authors, true):
			blocked = append(blocked, "filename did not yield an author")
		case !sameAuthorList(candidate.Authors, authors):
			proposal.ProposedAuthors = append([]string{}, candidate.Authors...)
			proposal.AllowedFields = append(proposal.AllowedFields, metaprotection.FieldAuthors)
			if authorConflict {
				proposal.Reasons = append(proposal.Reasons, "current authors conflict with an explicit filename author while the title is suspicious")
			} else {
				proposal.Reasons = append(proposal.Reasons, authorReason(authors, authorsValid))
			}
		}
	}

	if len(proposal.AllowedFields) == 0 {
		if len(blocked) == 0 {
			blocked = append(blocked, "filename candidate matches current metadata")
		}
		return Proposal{}, strings.Join(blocked, "; "), true
	}
	if len(blocked) > 0 {
		proposal.Reasons = append(proposal.Reasons, "unchanged field: "+strings.Join(blocked, "; "))
	}
	proposal.Confidence = confidenceFor(candidate, proposal)
	return proposal, "", true
}

func parseFilename(path string) filenameCandidate {
	return filenameinfo.Analyze(path)
}

func suspiciousTitle(title string) bool {
	return filenameinfo.SuspiciousTitle(title)
}

func suspiciousAuthors(authors []string, valid bool) bool {
	return filenameinfo.SuspiciousAuthors(authors, valid)
}

func parseAuthors(raw string) ([]string, bool) {
	var authors []string
	if err := json.Unmarshal([]byte(raw), &authors); err != nil {
		return []string{}, false
	}
	result := make([]string, 0, len(authors))
	for _, author := range authors {
		if author = normalizeSpace(author); author != "" {
			result = append(result, author)
		}
	}
	return result, true
}

func confidenceFor(candidate filenameCandidate, proposal Proposal) string {
	if proposal.ExistingAuthorConflict {
		return "ambiguous"
	}
	if candidate.Confidence == "medium" && !contains(proposal.AllowedFields, metaprotection.FieldTitle) && candidate.Pattern == "filename title" {
		return "ambiguous"
	}
	return candidate.Confidence
}

func titleReason(current string) string {
	if strings.TrimSpace(current) == "" {
		return "current title is missing"
	}
	return "current title looks like a document or placeholder filename"
}

func authorReason(current []string, valid bool) string {
	if !valid {
		return "current authors JSON is invalid"
	}
	if len(current) == 0 {
		return "current authors are missing"
	}
	return "current authors contain a placeholder value"
}

func normalizeSpace(value string) string {
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}

func sameText(left, right string) bool {
	return strings.EqualFold(normalizeSpace(left), normalizeSpace(right))
}

func sameAuthorList(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !sameText(left[i], right[i]) {
			return false
		}
	}
	return true
}

func confidenceRank(value string) int {
	switch value {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func weightedReviewOrder(proposals []Proposal) []Proposal {
	queues := map[string][]Proposal{"high": {}, "medium": {}, "ambiguous": {}}
	for _, proposal := range proposals {
		queues[proposal.Confidence] = append(queues[proposal.Confidence], proposal)
	}
	weights := []struct {
		confidence string
		count      int
	}{{"high", 12}, {"medium", 5}, {"ambiguous", 3}}
	positions := map[string]int{}
	ordered := make([]Proposal, 0, len(proposals))
	for len(ordered) < len(proposals) {
		before := len(ordered)
		for _, weight := range weights {
			queue := queues[weight.confidence]
			start := positions[weight.confidence]
			end := min(start+weight.count, len(queue))
			ordered = append(ordered, queue[start:end]...)
			positions[weight.confidence] = end
		}
		if len(ordered) == before {
			break
		}
	}
	return ordered
}

func skippedFromInventory(item inventoryItem, reason string) SkippedItem {
	authors, _ := parseAuthors(item.AuthorsRaw)
	return SkippedItem{BookID: item.BookID, Library: item.Library, Path: item.Path, Title: item.Title, Authors: authors, Reason: reason}
}

func writeReviewCSV(path, checksum string, proposals []Proposal) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create review CSV: %w", err)
	}
	defer file.Close()
	w := csv.NewWriter(file)
	header := []string{
		"manifest_sha256", "decision", "book_id", "library", "filename", "current_title",
		"proposed_title", "current_authors_json", "proposed_authors_json", "changed_fields",
		"confidence", "filename_pattern", "reasons", "notes",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, proposal := range proposals {
		currentAuthors, _ := json.Marshal(proposal.CurrentAuthors)
		proposedAuthors, _ := json.Marshal(proposal.ProposedAuthors)
		record := []string{
			checksum, "", strconv.FormatInt(proposal.BookID, 10), proposal.Library, filepath.Base(proposal.Path),
			proposal.CurrentTitle, proposal.ProposedTitle, string(currentAuthors), string(proposedAuthors),
			strings.Join(proposal.AllowedFields, ","), proposal.Confidence, proposal.FilenamePattern,
			strings.Join(proposal.Reasons, "; "), "",
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write review CSV: %w", err)
	}
	return file.Sync()
}

// RenderCompactReview creates a terminal-friendly companion to the detailed CSV
// without changing the immutable manifest or its checksum.
func RenderCompactReview(manifestPath, outputPath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Version != ManifestVersion {
		return fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	return writeCompactReview(outputPath, manifest.Proposals)
}

func writeCompactReview(path string, proposals []Proposal) error {
	var output strings.Builder
	output.WriteString("Filename metadata review\n")
	output.WriteString("Detailed decisions remain in review.csv. Width is limited to 120 columns.\n\n")
	for _, proposal := range proposals {
		fmt.Fprintf(&output, "Book %d | %s | change: %s\n", proposal.BookID, strings.ToUpper(proposal.Confidence), strings.Join(proposal.AllowedFields, ", "))
		writeCompactField(&output, "File", filepath.Base(proposal.Path))
		writeCompactField(&output, "Current", proposal.CurrentTitle)
		writeCompactField(&output, "Author", compactAuthors(proposal.CurrentAuthors))
		writeCompactField(&output, "Proposed", proposal.ProposedTitle)
		writeCompactField(&output, "Author", compactAuthors(proposal.ProposedAuthors))
		output.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(output.String()), 0o644); err != nil {
		return fmt.Errorf("write compact review: %w", err)
	}
	return nil
}

func compactAuthors(authors []string) string {
	if len(authors) == 0 {
		return "—"
	}
	return strings.Join(authors, "; ")
}

func writeCompactField(output *strings.Builder, label, value string) {
	const lineWidth = 120
	const labelWidth = 10
	lines := wrapCompactText(normalizeSpace(value), lineWidth-labelWidth)
	for index, line := range lines {
		if index == 0 {
			fmt.Fprintf(output, "%-10s%s\n", label, line)
		} else {
			fmt.Fprintf(output, "%-10s%s\n", "", line)
		}
	}
}

func wrapCompactText(value string, width int) []string {
	if value == "" {
		return []string{"—"}
	}
	words := strings.Fields(value)
	lines := make([]string, 0, 2)
	current := make([]rune, 0, width)
	flush := func() {
		if len(current) > 0 {
			lines = append(lines, string(current))
			current = current[:0]
		}
	}
	for _, word := range words {
		runes := []rune(word)
		if len(current) > 0 && len(current)+1+len(runes) > width {
			flush()
		}
		for len(runes) > width {
			lines = append(lines, string(runes[:width]))
			runes = runes[width:]
		}
		if len(runes) == 0 {
			continue
		}
		if len(current) > 0 {
			current = append(current, ' ')
		}
		current = append(current, runes...)
	}
	flush()
	return lines
}

func writeSkippedCSV(path string, skipped []SkippedItem) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create skipped CSV: %w", err)
	}
	defer file.Close()
	w := csv.NewWriter(file)
	_ = w.Write([]string{"book_id", "library", "filename", "current_title", "current_authors_json", "reason"})
	for _, item := range skipped {
		authors, _ := json.Marshal(item.Authors)
		_ = w.Write([]string{strconv.FormatInt(item.BookID, 10), item.Library, filepath.Base(item.Path), item.Title, string(authors), item.Reason})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write skipped CSV: %w", err)
	}
	return file.Sync()
}

func writeSummary(path string, manifest Manifest, checksum string, inventory, scoped, candidates, skipped int) error {
	counts := map[string]int{}
	for _, proposal := range manifest.Proposals {
		counts[proposal.Confidence]++
	}
	remaining := max(0, candidates-manifest.Offset-len(manifest.Proposals))
	content := fmt.Sprintf(`# Filename Metadata Audit

- Generated: %s
- Source database: %s
- Manifest SHA-256: %s
- Inventory records: %d
- Missing or suspicious records: %d
- Total proposals available: %d
- Proposals in this batch: %d
- High confidence: %d
- Medium confidence: %d
- Ambiguous: %d
- Skipped records: %d
- Remaining proposals after this batch: %d

Use review-compact.txt for a wrapped overview. Edit only the decision, proposed_title, proposed_authors_json, and notes columns in review.csv. Use approve or reject in the decision column. Blank decisions remain unapplied.
`, time.Unix(manifest.GeneratedAt, 0).Format(time.RFC3339), manifest.SourceDB, checksum, inventory, scoped,
		candidates, len(manifest.Proposals), counts["high"], counts["medium"], counts["ambiguous"], skipped, remaining)
	return os.WriteFile(path, []byte(content), 0o644)
}

type decisionRow struct {
	Decision        string
	BookID          int64
	Checksum        string
	ProposedTitle   string
	ProposedAuthors []string
}

func Apply(ctx context.Context, db *sql.DB, options ApplyOptions) (ApplyResult, error) {
	manifestBytes, err := os.ReadFile(options.ManifestPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("read manifest: %w", err)
	}
	checksum := sha256.Sum256(manifestBytes)
	checksumText := hex.EncodeToString(checksum[:])
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return ApplyResult{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Version != ManifestVersion {
		return ApplyResult{}, fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	manifestDB, err := filepath.Abs(manifest.SourceDB)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("resolve manifest database: %w", err)
	}
	requestedDB, err := filepath.Abs(options.SourceDB)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("resolve requested database: %w", err)
	}
	if filepath.Clean(manifestDB) != filepath.Clean(requestedDB) {
		return ApplyResult{}, fmt.Errorf("manifest was generated for %s, not %s", manifestDB, requestedDB)
	}
	decisions, result, err := readDecisions(options.DecisionsPath, checksumText)
	if err != nil {
		return ApplyResult{}, err
	}
	if result.Approved == 0 {
		return result, nil
	}

	proposals := make(map[int64]Proposal, len(manifest.Proposals))
	for _, proposal := range manifest.Proposals {
		proposals[proposal.BookID] = proposal
	}
	for bookID, decision := range decisions {
		proposal, ok := proposals[bookID]
		if !ok {
			return ApplyResult{}, fmt.Errorf("book %d is not in the manifest", bookID)
		}
		if decision.Decision != "approve" {
			continue
		}
		if err := validateDecision(proposal, decision); err != nil {
			return ApplyResult{}, fmt.Errorf("book %d: %w", bookID, err)
		}
	}
	if len(decisions) != len(proposals) {
		return ApplyResult{}, fmt.Errorf("decisions CSV has %d rows; manifest has %d proposals", len(decisions), len(proposals))
	}

	backupDir := options.BackupDir
	if strings.TrimSpace(backupDir) == "" {
		backupDir = filepath.Join(filepath.Dir(options.SourceDB), "backups")
	}
	backupPath, err := createBackup(db, backupDir)
	if err != nil {
		return ApplyResult{}, err
	}
	result.BackupPath = backupPath
	if options.ActorUserID <= 0 {
		options.ActorUserID = 1
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("acquire database connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return ApplyResult{}, fmt.Errorf("begin metadata audit transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	for _, proposal := range manifest.Proposals {
		decision, ok := decisions[proposal.BookID]
		if !ok || decision.Decision != "approve" {
			continue
		}
		if err := applyOne(ctx, conn, proposal, decision, options.ActorUserID); err != nil {
			return ApplyResult{}, fmt.Errorf("apply book %d: %w", proposal.BookID, err)
		}
		result.Updated++
	}
	data, _ := json.Marshal(map[string]any{
		"approved": result.Approved, "updated": result.Updated, "manifest_sha256": checksumText,
		"backup": backupPath,
	})
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO app_log (level, category, message, data_json, created_at)
		VALUES ('info', 'metadata', 'Applied filename metadata audit', ?, ?)
	`, string(data), time.Now().Unix()); err != nil {
		return ApplyResult{}, fmt.Errorf("record audit log: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return ApplyResult{}, fmt.Errorf("commit metadata audit: %w", err)
	}
	committed = true
	return result, nil
}

func readDecisions(path, checksum string) (map[int64]decisionRow, ApplyResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, ApplyResult{}, fmt.Errorf("open decisions CSV: %w", err)
	}
	defer file.Close()
	r := csv.NewReader(file)
	header, err := r.Read()
	if err != nil {
		return nil, ApplyResult{}, fmt.Errorf("read decisions header: %w", err)
	}
	columns := make(map[string]int, len(header))
	for index, value := range header {
		columns[strings.TrimSpace(value)] = index
	}
	required := []string{"manifest_sha256", "decision", "book_id", "proposed_title", "proposed_authors_json"}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			return nil, ApplyResult{}, fmt.Errorf("decisions CSV is missing %s", name)
		}
	}
	decisions := make(map[int64]decisionRow)
	result := ApplyResult{}
	for line := 2; ; line++ {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, ApplyResult{}, fmt.Errorf("read decisions line %d: %w", line, err)
		}
		value := func(name string) string {
			index := columns[name]
			if index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		if value("manifest_sha256") != checksum {
			return nil, ApplyResult{}, fmt.Errorf("manifest checksum mismatch on line %d", line)
		}
		bookID, err := strconv.ParseInt(value("book_id"), 10, 64)
		if err != nil || bookID <= 0 {
			return nil, ApplyResult{}, fmt.Errorf("invalid book ID on line %d", line)
		}
		decision := strings.ToLower(value("decision"))
		if decision != "" && decision != "approve" && decision != "reject" {
			return nil, ApplyResult{}, fmt.Errorf("invalid decision %q on line %d", decision, line)
		}
		var authors []string
		if err := json.Unmarshal([]byte(value("proposed_authors_json")), &authors); err != nil {
			return nil, ApplyResult{}, fmt.Errorf("invalid proposed authors on line %d: %w", line, err)
		}
		if _, exists := decisions[bookID]; exists {
			return nil, ApplyResult{}, fmt.Errorf("duplicate book ID %d in decisions", bookID)
		}
		decisions[bookID] = decisionRow{
			Decision: decision, BookID: bookID, Checksum: checksum,
			ProposedTitle: normalizeSpace(value("proposed_title")), ProposedAuthors: normalizeAuthors(authors),
		}
		switch decision {
		case "approve":
			result.Approved++
		case "reject":
			result.Rejected++
		default:
			result.Unreviewed++
		}
	}
	return decisions, result, nil
}

func validateDecision(proposal Proposal, decision decisionRow) error {
	if contains(proposal.AllowedFields, metaprotection.FieldTitle) && decision.ProposedTitle == "" {
		return fmt.Errorf("approved title must not be empty")
	}
	if contains(proposal.AllowedFields, metaprotection.FieldAuthors) && len(decision.ProposedAuthors) == 0 {
		return fmt.Errorf("approved authors must not be empty")
	}
	return nil
}

func createBackup(db *sql.DB, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("cryptorum-pre-metadata-%s.db", time.Now().Format("20060102-150405.000000000")))
	escaped := strings.ReplaceAll(backupPath, "'", "''")
	if _, err := db.Exec("VACUUM INTO '" + escaped + "'"); err != nil {
		return "", fmt.Errorf("create pre-apply backup: %w", err)
	}
	return backupPath, nil
}

func applyOne(ctx context.Context, conn *sql.Conn, proposal Proposal, decision decisionRow, actorUserID int64) error {
	var current metaprotection.Snapshot
	var activePath string
	err := conn.QueryRowContext(ctx, `
		SELECT bm.book_id, COALESCE(bm.title, ''), COALESCE(bm.authors, '[]'),
		       COALESCE(bm.series, ''), COALESCE(bm.series_number, 0), COALESCE(bm.series_number_display, ''),
		       COALESCE(bm.publisher, ''), COALESCE(bm.pub_date, ''), COALESCE(bm.description, ''),
		       COALESCE(bm.rating, 0), COALESCE(bm.genres, '[]'), COALESCE(bm.tags, '[]'),
		       COALESCE(bm.isbn, ''), COALESCE(bm.asin, ''), COALESCE(bm.language, ''),
		       COALESCE(bm.page_count, 0), COALESCE(bm.cover_path, ''), COALESCE(bm.cover_source, ''),
		       COALESCE(bm.cover_updated_on, 0), COALESCE(bm.locked_fields, '[]'),
		       COALESCE(bm.extracted_from_hash, ''), COALESCE(bm.metadata_updated_at, 0),
		       COALESCE(bm.owner_user_id, 1), COALESCE(bf.path, '')
		FROM book_metadata bm
		LEFT JOIN book_file bf ON bf.id = (
			SELECT candidate.id FROM book_file candidate
			WHERE candidate.book_id = bm.book_id AND candidate.missing_at IS NULL
			ORDER BY candidate.id LIMIT 1
		)
		WHERE bm.book_id = ?
	`, proposal.BookID).Scan(
		&current.BookID, &current.Title, &current.Authors, &current.Series, &current.SeriesNumber,
		&current.SeriesNumberDisplay, &current.Publisher, &current.PubDate, &current.Description,
		&current.Rating, &current.Genres, &current.Tags, &current.ISBN, &current.ASIN,
		&current.Language, &current.PageCount, &current.CoverPath, &current.CoverSource,
		&current.CoverUpdatedOn, &current.LockedFields, &current.ExtractedFromHash,
		&current.MetadataUpdatedAt, &current.OwnerUserID, &activePath,
	)
	if err != nil {
		return fmt.Errorf("load current metadata: %w", err)
	}
	currentAuthors, valid := parseAuthors(current.Authors)
	if !valid || current.Title != proposal.CurrentTitle || current.Authors != proposal.CurrentAuthorsRaw ||
		current.LockedFields != proposal.CurrentLocked || current.MetadataUpdatedAt != proposal.CurrentUpdatedAt || activePath != proposal.Path {
		return fmt.Errorf("metadata or active file changed since the audit was generated")
	}

	changed := make([]string, 0, 2)
	newTitle := current.Title
	newAuthors := currentAuthors
	if contains(proposal.AllowedFields, metaprotection.FieldTitle) && !sameText(current.Title, decision.ProposedTitle) {
		newTitle = decision.ProposedTitle
		changed = append(changed, metaprotection.FieldTitle)
	}
	if contains(proposal.AllowedFields, metaprotection.FieldAuthors) && !sameAuthorList(currentAuthors, decision.ProposedAuthors) {
		newAuthors = decision.ProposedAuthors
		changed = append(changed, metaprotection.FieldAuthors)
	}
	if len(changed) == 0 {
		return fmt.Errorf("approved row does not change an allowed field")
	}
	if err := metaprotection.RecordRevision(connDBTX{conn: conn}, current, changed, "filename_audit", actorUserID); err != nil {
		return fmt.Errorf("record metadata revision: %w", err)
	}
	authorsJSON, _ := json.Marshal(newAuthors)
	locked := metaprotection.MergeLocked(current.LockedFields, changed...)
	now := time.Now().Unix()
	if _, err := conn.ExecContext(ctx, `DELETE FROM book_fts WHERE rowid = (SELECT id FROM book_metadata WHERE book_id = ?)`, proposal.BookID); err != nil {
		return fmt.Errorf("delete stale search index entry: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `
		UPDATE book_metadata
		SET title = ?, authors = ?, locked_fields = ?, metadata_updated_at = ?
		WHERE book_id = ?
	`, newTitle, string(authorsJSON), locked, now, proposal.BookID); err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO book_fts(rowid, title, authors, description, series)
		SELECT id, title, ?, COALESCE(description, ''), COALESCE(series, '')
		FROM book_metadata WHERE book_id = ?
	`, strings.Join(newAuthors, " "), proposal.BookID); err != nil {
		return fmt.Errorf("update search index: %w", err)
	}
	return nil
}

func normalizeAuthors(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = normalizeSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
