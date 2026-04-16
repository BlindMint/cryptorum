package metadata

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// BookMetadata represents extracted book metadata
type BookMetadata struct {
	Title        string   `json:"title"`
	Authors      []string `json:"authors"`
	Series       string   `json:"series"`
	SeriesNumber float64  `json:"series_number,omitempty"`
	Publisher    string   `json:"publisher"`
	PubDate      string   `json:"pub_date"`
	Description  string   `json:"description"`
	Rating       float64  `json:"rating,omitempty"`
	Genres       []string `json:"genres"`
	ISBN         string   `json:"isbn"`
	ASIN         string   `json:"asin,omitempty"`
	CoverData    []byte   `json:"-"` // Cover image data
	PageCount    int      `json:"page_count,omitempty"`
	Language     string   `json:"language,omitempty"`
	Source       string   `json:"-"`
}

// Extract extracts metadata from a book file based on its format
func Extract(filePath string) (*BookMetadata, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != "" {
		ext = ext[1:] // Remove dot
	}

	switch ext {
	case "epub":
		return extractEPUBMetadata(filePath)
	case "pdf":
		return extractPDFMetadata(filePath)
	case "cbz", "cbr", "cb7":
		return extractCBXMetadata(filePath)
	case "mp3", "m4a", "m4b", "flac", "ogg", "wav":
		return extractAudioMetadata(filePath)
	case "mobi", "azw3":
		return extractMOBIMetadata(filePath)
	default:
		return extractFromFilename(filePath), nil
	}
}

// ExtractFilename exposes the filename fallback parser for guarded metadata repair.
func ExtractFilename(filePath string) *BookMetadata {
	return extractFromFilename(filePath)
}

// EPUB metadata extraction
type opfPackage struct {
	XMLName  xml.Name    `xml:"package"`
	Metadata opfMetadata `xml:"metadata"`
	Manifest opfManifest `xml:"manifest"`
}

type opfMetadata struct {
	Title       []opfIdentifier `xml:"title"`
	Creator     []opfCreator    `xml:"creator"`
	Publisher   []opfIdentifier `xml:"publisher"`
	Description []opfIdentifier `xml:"description"`
	Date        []opfIdentifier `xml:"date"`
	Language    []opfIdentifier `xml:"language"`
	ISBN        []opfIdentifier `xml:"identifier"`
	CoverImage  []opfMeta       `xml:"meta"`
}

type opfIdentifier struct {
	ID     string `xml:"id,attr"`
	Scheme string `xml:"scheme,attr"`
	Value  string `xml:",chardata"`
}

type opfCreator struct {
	Role  string `xml:"role,attr"`
	Value string `xml:",chardata"`
}

type opfMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
}

type opfManifest struct {
	Items []opfItem `xml:"item"`
}

type opfItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

func extractEPUBMetadata(filePath string) (*BookMetadata, error) {
	metadata := &BookMetadata{
		Authors: []string{},
		Genres:  []string{},
	}

	// Open EPUB as ZIP
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open EPUB: %w", err)
	}
	defer reader.Close()

	// Find and parse OPF file
	var opfPath string
	var opfDir string
	for _, f := range reader.File {
		if strings.HasSuffix(f.Name, ".opf") && !strings.Contains(f.Name, "__") {
			opfPath = f.Name
			opfDir = filepath.Dir(f.Name)
			break
		}
	}

	if opfPath == "" {
		slog.Debug("No OPF file found in EPUB", "path", filePath)
		return extractFromFilename(filePath), nil
	}

	// Read OPF file
	var opfContent []byte
	for _, f := range reader.File {
		if f.Name == opfPath {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open OPF: %w", err)
			}
			opfContent, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read OPF: %w", err)
			}
			break
		}
	}

	// Parse OPF XML
	var pkg opfPackage
	if err := xml.Unmarshal(opfContent, &pkg); err != nil {
		slog.Debug("Failed to parse OPF", "error", err)
		return extractFromFilename(filePath), nil
	}

	// Extract metadata
	if len(pkg.Metadata.Title) > 0 {
		metadata.Title = pkg.Metadata.Title[0].Value
	}

	for _, creator := range pkg.Metadata.Creator {
		if creator.Value != "" {
			metadata.Authors = append(metadata.Authors, creator.Value)
		}
	}

	if len(pkg.Metadata.Publisher) > 0 {
		metadata.Publisher = pkg.Metadata.Publisher[0].Value
	}

	if len(pkg.Metadata.Description) > 0 {
		metadata.Description = pkg.Metadata.Description[0].Value
	}

	if len(pkg.Metadata.Date) > 0 {
		metadata.PubDate = pkg.Metadata.Date[0].Value
	}

	if len(pkg.Metadata.Language) > 0 {
		metadata.Language = pkg.Metadata.Language[0].Value
	}

	// Extract ISBN/ASIN from identifiers
	for _, id := range pkg.Metadata.ISBN {
		value := strings.TrimSpace(id.Value)
		scheme := strings.ToLower(strings.TrimSpace(id.Scheme + " " + id.ID))
		if metadata.ISBN == "" && (strings.HasPrefix(value, "978") || strings.HasPrefix(value, "979")) {
			metadata.ISBN = value
		}
		if metadata.ASIN == "" && (strings.Contains(scheme, "asin") || looksLikeASIN(value)) {
			metadata.ASIN = normalizeASIN(value)
		}
	}

	// Find cover image
	for _, meta := range pkg.Metadata.CoverImage {
		if meta.Name == "cover" {
			coverHref := meta.Content
			// Look for cover in manifest
			for _, item := range pkg.Manifest.Items {
				if item.ID == coverHref || item.Href == coverHref {
					coverPath := filepath.Join(opfDir, item.Href)
					metadata.CoverData = readValidZipEntry(reader, coverPath)
					break
				}
			}
			break
		}
	}

	// If no cover found via meta, look for cover in manifest by convention
	if metadata.CoverData == nil {
		for _, item := range pkg.Manifest.Items {
			if isImageFile(item.Href) && (strings.Contains(strings.ToLower(item.ID), "cover") ||
				strings.Contains(strings.ToLower(item.Href), "cover")) {
				coverPath := filepath.Join(opfDir, item.Href)
				metadata.CoverData = readValidZipEntry(reader, coverPath)
				break
			}
		}
	}

	if metadata.CoverData == nil {
		metadata.CoverData = findFolderCoverImage(filePath)
	}

	// Fallback to filename if no title
	if metadata.Title == "" {
		metadata.Title = extractTitleFromFilename(filePath)
	}

	return metadata, nil
}

func normalizeASIN(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func looksLikeASIN(value string) bool {
	value = normalizeASIN(value)
	if len(value) != 10 {
		return false
	}
	if looksLikeISBN10(value) {
		return false
	}
	hasLetter := false
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
			break
		}
	}
	return hasLetter
}

func looksLikeISBN10(value string) bool {
	if len(value) != 10 {
		return false
	}
	for i, r := range value {
		if i == 9 && r == 'X' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func readZipEntry(reader *zip.ReadCloser, path string) []byte {
	for _, f := range reader.File {
		if f.Name == path {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil
			}
			return data
		}
	}
	return nil
}

func readValidZipEntry(reader *zip.ReadCloser, path string) []byte {
	data := readZipEntry(reader, path)
	if isRenderableCoverData(data) {
		return data
	}
	return nil
}

func isRenderableCoverData(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return true
	}
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return true
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(string(trimmed)), "<svg")
}

func extractPDFMetadata(filePath string) (*BookMetadata, error) {
	metadata := &BookMetadata{
		Authors: []string{},
		Genres:  []string{},
		Source:  "pdf",
	}

	// Try to open PDF and extract basic info
	f, err := os.Open(filePath)
	if err != nil {
		return extractFromFilename(filePath), nil
	}
	defer f.Close()

	// Read first few KB to check for PDF header
	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	if n < 5 || string(buf[:5]) != "%PDF-" {
		return extractFromFilename(filePath), nil
	}

	extractPDFInfoMetadata(filePath, metadata)
	extractPDFXMPMetadata(filePath, metadata)

	filenameMetadata := extractFromFilename(filePath)
	if strings.TrimSpace(metadata.Title) == "" {
		metadata.Title = filenameMetadata.Title
	}
	if len(metadata.Authors) == 0 {
		metadata.Authors = filenameMetadata.Authors
	}
	if metadata.Series == "" {
		metadata.Series = filenameMetadata.Series
	}
	if metadata.SeriesNumber == 0 {
		metadata.SeriesNumber = filenameMetadata.SeriesNumber
	}

	metadata.CoverData = renderPDFCover(filePath)
	if !isRenderableCoverData(metadata.CoverData) {
		metadata.CoverData = findFolderCoverImage(filePath)
	}

	return metadata, nil
}

func extractPDFInfoMetadata(filePath string, metadata *BookMetadata) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return
	}

	cmd := exec.Command("pdfinfo", "-enc", "UTF-8", filePath)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		switch key {
		case "Title":
			metadata.Title = value
		case "Author":
			metadata.Authors = splitAuthors(value)
		case "Subject":
			metadata.Description = value
		case "Pages":
			if pages, err := strconv.Atoi(value); err == nil && pages > 0 {
				metadata.PageCount = pages
			}
		case "CreationDate":
			if metadata.PubDate == "" {
				metadata.PubDate = normalizePDFInfoDate(value)
			}
		}
	}
}

func extractPDFXMPMetadata(filePath string, metadata *BookMetadata) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return
	}

	cmd := exec.Command("pdfinfo", "-meta", filePath)
	output, err := cmd.Output()
	if err != nil || len(bytes.TrimSpace(output)) == 0 {
		return
	}

	decoder := xml.NewDecoder(bytes.NewReader(output))
	stack := []xml.Name{}
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" || len(stack) == 0 {
				continue
			}
			leaf := stack[len(stack)-1].Local
			parent := ""
			grandparent := ""
			if len(stack) >= 2 {
				parent = stack[len(stack)-2].Local
			}
			if len(stack) >= 3 {
				grandparent = stack[len(stack)-3].Local
			}

			switch {
			case leaf == "li" && parent == "Alt" && grandparent == "title":
				metadata.Title = text
			case leaf == "li" && parent == "Seq" && grandparent == "creator":
				metadata.Authors = splitAuthors(text)
			case leaf == "li" && parent == "Bag" && grandparent == "publisher" && metadata.Publisher == "":
				metadata.Publisher = text
			case leaf == "li" && parent == "Alt" && grandparent == "description" && metadata.Description == "":
				metadata.Description = text
			case leaf == "CreateDate" && metadata.PubDate == "":
				metadata.PubDate = normalizeISODate(text)
			case leaf == "MetadataDate" && metadata.PubDate == "":
				metadata.PubDate = normalizeISODate(text)
			case leaf == "Language" && metadata.Language == "":
				metadata.Language = text
			}
		}
	}
}

func splitAuthors(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	separators := []string{";", " & ", " and "}
	for _, separator := range separators {
		if strings.Contains(value, separator) {
			return cleanStringList(strings.Split(value, separator))
		}
	}
	return cleanStringList(strings.Split(value, ","))
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if strings.Contains(value, ";") {
		return cleanStringList(strings.Split(value, ";"))
	}
	return cleanStringList(strings.Split(value, ","))
}

func firstListValue(value string) string {
	values := splitList(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func cleanStringList(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeISODate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) >= 10 && value[4] == '-' && value[7] == '-' {
		return value[:10]
	}
	if len(value) >= 4 && isDigits(value[:4]) {
		return value[:4]
	}
	return value
}

func normalizePDFInfoDate(value string) string {
	parts := strings.Fields(value)
	if len(parts) >= 5 {
		month := map[string]string{
			"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04",
			"May": "05", "Jun": "06", "Jul": "07", "Aug": "08",
			"Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12",
		}[parts[1]]
		if month != "" && isDigits(parts[2]) && isDigits(parts[4]) {
			day, _ := strconv.Atoi(parts[2])
			return fmt.Sprintf("%s-%s-%02d", parts[4], month, day)
		}
	}
	if strings.HasPrefix(value, "D:") && len(value) >= 10 && isDigits(value[2:10]) {
		return fmt.Sprintf("%s-%s-%s", value[2:6], value[6:8], value[8:10])
	}
	return normalizeISODate(value)
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// CBX (comic book archive) metadata extraction
type comicInfoXML struct {
	XMLName   xml.Name `xml:"ComicInfo"`
	Title     string   `xml:"Title"`
	Series    string   `xml:"Series"`
	Summary   string   `xml:"Summary"`
	Writer    string   `xml:"Writer"`
	Publisher string   `xml:"Publisher"`
	Genre     string   `xml:"Genre"`
	Year      int      `xml:"Year"`
	Month     int      `xml:"Month"`
	Number    string   `xml:"Number"`
}

func extractCBXMetadata(filePath string) (*BookMetadata, error) {
	metadata := &BookMetadata{
		Authors: []string{},
		Genres:  []string{},
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".cbz":
		// ZIP archive
		reader, err := zip.OpenReader(filePath)
		if err != nil {
			return extractFromFilename(filePath), nil
		}
		defer reader.Close()

		// Look for ComicInfo.xml
		for _, f := range reader.File {
			if strings.EqualFold(f.Name, "ComicInfo.xml") {
				rc, err := f.Open()
				if err != nil {
					break
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					break
				}

				var comicInfo comicInfoXML
				if err := xml.Unmarshal(data, &comicInfo); err == nil {
					if comicInfo.Title != "" {
						metadata.Title = comicInfo.Title
					}
					if comicInfo.Series != "" {
						metadata.Series = comicInfo.Series
					}
					if comicInfo.Summary != "" {
						metadata.Description = comicInfo.Summary
					}
					if comicInfo.Writer != "" {
						metadata.Authors = append(metadata.Authors, comicInfo.Writer)
					}
					if comicInfo.Publisher != "" {
						metadata.Publisher = comicInfo.Publisher
					}
					if comicInfo.Genre != "" {
						metadata.Genres = append(metadata.Genres, strings.Split(comicInfo.Genre, ",")...)
					}
				}
				break
			}
		}

		metadata.CoverData = extractFirstZipImage(reader)

	case ".cbr":
		// RAR archive - would need rardecode library
		// For now, just use filename
		metadata = extractFromFilename(filePath)

	case ".cb7":
		// 7z archive - not supported in pure Go easily
		metadata = extractFromFilename(filePath)
	}

	if !isRenderableCoverData(metadata.CoverData) {
		metadata.CoverData = findFolderCoverImage(filePath)
	}

	if metadata.Title == "" {
		metadata.Title = extractTitleFromFilename(filePath)
	}

	return metadata, nil
}

func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif"
}

func extractFirstZipImage(reader *zip.ReadCloser) []byte {
	files := make([]*zip.File, 0, len(reader.File))
	for _, f := range reader.File {
		if isImageFile(f.Name) && !strings.Contains(f.Name, "__") {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	for _, f := range files {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err == nil && isRenderableCoverData(data) {
			return data
		}
	}

	return nil
}

func findFolderCoverImage(filePath string) []byte {
	dir := filepath.Dir(filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	bookBase := strings.ToLower(strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)))

	type candidate struct {
		path     string
		priority int
	}

	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !isImageFile(entry.Name()) {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		priority := 100
		switch name {
		case "cover", "folder", "front", "poster", "thumbnail":
			priority = 0
		default:
			if name == bookBase {
				priority = 5
			} else if strings.Contains(name, "cover") {
				priority = 10
			} else if strings.Contains(name, "front") || strings.Contains(name, "folder") {
				priority = 20
			} else {
				continue
			}
		}
		candidates = append(candidates, candidate{
			path:     filepath.Join(dir, entry.Name()),
			priority: priority,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		return strings.ToLower(candidates[i].path) < strings.ToLower(candidates[j].path)
	})

	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate.path)
		if err == nil && isRenderableCoverData(data) {
			return data
		}
	}

	return nil
}

func renderPDFCover(filePath string) []byte {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil
	}

	tempDir, err := os.MkdirTemp("", "cryptorum-pdf-cover-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tempDir)

	prefix := filepath.Join(tempDir, "cover")
	cmd := exec.Command("pdftoppm", "-f", "1", "-singlefile", "-jpeg", "-r", "150", "-cropbox", filePath, prefix)
	if err := cmd.Run(); err == nil {
		if data, readErr := os.ReadFile(prefix + ".jpg"); readErr == nil && len(data) > 0 {
			return data
		}
	}

	cmd = exec.Command("pdftoppm", "-f", "1", "-singlefile", "-png", "-r", "150", "-cropbox", filePath, prefix)
	if err := cmd.Run(); err == nil {
		if data, readErr := os.ReadFile(prefix + ".png"); readErr == nil && len(data) > 0 {
			return data
		}
	}

	cmd = exec.Command("pdftoppm", "-f", "1", "-singlefile", "-jpeg", "-r", "150", filePath, prefix)
	if err := cmd.Run(); err == nil {
		if data, readErr := os.ReadFile(prefix + ".jpg"); readErr == nil && len(data) > 0 {
			return data
		}
	}

	cmd = exec.Command("pdftoppm", "-f", "1", "-singlefile", "-png", "-r", "150", filePath, prefix)
	if err := cmd.Run(); err != nil {
		return nil
	}
	data, err := os.ReadFile(prefix + ".png")
	if err != nil {
		return nil
	}
	return data
}

// Audio metadata extraction (simplified)
func extractAudioMetadata(filePath string) (*BookMetadata, error) {
	metadata := extractFFProbeMetadata(filePath)
	if metadata == nil {
		metadata = extractFromFilename(filePath)
	}
	if !isRenderableCoverData(metadata.CoverData) {
		metadata.CoverData = findFolderCoverImage(filePath)
	}
	return metadata, nil
}

func extractFFProbeMetadata(filePath string) *BookMetadata {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return nil
	}

	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var parsed struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil
	}
	if len(parsed.Format.Tags) == 0 {
		return nil
	}

	tags := map[string]string{}
	for key, value := range parsed.Format.Tags {
		tags[strings.ToLower(key)] = strings.TrimSpace(value)
	}

	metadata := &BookMetadata{
		Authors: []string{},
		Genres:  []string{},
		Source:  "ffprobe",
	}
	metadata.Title = firstNonEmpty(tags["title"], tags["album"])
	author := firstNonEmpty(tags["artist"], tags["album_artist"], tags["author"], tags["composer"])
	metadata.Authors = splitAuthors(author)
	metadata.Publisher = tags["publisher"]
	metadata.PubDate = normalizeISODate(firstNonEmpty(tags["date"], tags["year"]))
	metadata.Genres = splitList(tags["genre"])
	metadata.ISBN = firstNonEmpty(tags["isbn"], tags["isbn13"], tags["isbn10"])
	metadata.ASIN = normalizeASIN(tags["asin"])
	metadata.Description = firstNonEmpty(tags["description"], tags["comment"])
	metadata.Language = tags["language"]

	if metadata.Title == "" && len(metadata.Authors) == 0 && metadata.Publisher == "" && metadata.PubDate == "" {
		return nil
	}

	filenameMetadata := extractFromFilename(filePath)
	if metadata.Title == "" {
		metadata.Title = filenameMetadata.Title
	}
	if len(metadata.Authors) == 0 {
		metadata.Authors = filenameMetadata.Authors
	}
	return metadata
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// MOBI metadata extraction (simplified)
func extractMOBIMetadata(filePath string) (*BookMetadata, error) {
	metadata := extractEbookMetaMetadata(filePath)
	if metadata == nil {
		metadata = extractFromFilename(filePath)
	}
	metadata.CoverData = extractEbookMetaCover(filePath)
	if !isRenderableCoverData(metadata.CoverData) {
		metadata.CoverData = findFolderCoverImage(filePath)
	}
	return metadata, nil
}

func extractEbookMetaMetadata(filePath string) *BookMetadata {
	if _, err := exec.LookPath("ebook-meta"); err != nil {
		return nil
	}

	cmd := exec.Command("ebook-meta", filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	metadata := &BookMetadata{
		Authors: []string{},
		Genres:  []string{},
		Source:  "ebook-meta",
	}

	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		switch key {
		case "title":
			metadata.Title = value
		case "author(s)", "authors", "author":
			metadata.Authors = splitAuthors(value)
		case "publisher":
			metadata.Publisher = value
		case "published":
			metadata.PubDate = normalizeISODate(value)
		case "comments", "description":
			metadata.Description = value
		case "tags":
			metadata.Genres = splitList(value)
		case "isbn":
			metadata.ISBN = value
		case "languages", "language":
			metadata.Language = firstListValue(value)
		case "series":
			metadata.Series = value
		case "series index":
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				metadata.SeriesNumber = parsed
			}
		}
	}

	if metadata.Title == "" && len(metadata.Authors) == 0 && metadata.Publisher == "" && metadata.ISBN == "" {
		return nil
	}

	filenameMetadata := extractFromFilename(filePath)
	if metadata.Title == "" {
		metadata.Title = filenameMetadata.Title
	}
	if len(metadata.Authors) == 0 {
		metadata.Authors = filenameMetadata.Authors
	}
	return metadata
}

func extractEbookMetaCover(filePath string) []byte {
	if _, err := exec.LookPath("ebook-meta"); err != nil {
		return nil
	}

	tempDir, err := os.MkdirTemp("", "cryptorum-ebook-cover-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tempDir)

	coverPath := filepath.Join(tempDir, "cover.jpg")
	cmd := exec.Command("ebook-meta", filePath, "--get-cover", coverPath)
	if err := cmd.Run(); err != nil {
		return nil
	}
	data, err := os.ReadFile(coverPath)
	if err != nil || !isRenderableCoverData(data) {
		return nil
	}
	return data
}

// extractFromFilename extracts metadata from filename patterns
// Common patterns:
// "Author - Title.epub"
// "Author - Series - Title.epub"
// "Title (Author).epub"
// "Series 01 - Title.epub"
func extractFromFilename(filePath string) *BookMetadata {
	metadata := &BookMetadata{
		Authors: []string{},
		Genres:  []string{},
		Source:  "filename",
	}

	filename := filepath.Base(filePath)
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Pattern: "Author - Title" or "Author - Series - Title"
	if parts := strings.SplitN(name, " - ", 3); len(parts) >= 2 {
		if len(parts) == 3 {
			metadata.Series = strings.TrimSpace(parts[1])
			metadata.Title = strings.TrimSpace(parts[2])
		} else {
			metadata.Title = strings.TrimSpace(parts[1])
		}
		metadata.Authors = append(metadata.Authors, strings.TrimSpace(parts[0]))
		return metadata
	}

	// Pattern: "Title (Author)"
	if match := findParenthesesContent(name); match != "" {
		metadata.Authors = append(metadata.Authors, match)
		metadata.Title = strings.TrimSpace(strings.Replace(name, "("+match+")", "", 1))
		return metadata
	}

	// Pattern: "Series XX - Title"
	if match := findSeriesNumber(name); match != "" {
		metadata.Series = match
		parts := strings.SplitN(name, "-", 2)
		if len(parts) == 2 {
			metadata.Title = strings.TrimSpace(parts[1])
		}
		return metadata
	}

	// Fallback: use full filename as title
	metadata.Title = name
	return metadata
}

func extractTitleFromFilename(filePath string) string {
	return extractFromFilename(filePath).Title
}

func findParenthesesContent(s string) string {
	start := strings.Index(s, "(")
	end := strings.Index(s, ")")
	if start >= 0 && end > start {
		return s[start+1 : end]
	}
	return ""
}

func findSeriesNumber(s string) string {
	// Look for patterns like "Book 1", "Vol 2", "01", etc.
	// Simplified implementation
	if idx := strings.IndexAny(s, "0123456789"); idx >= 0 {
		// Extract number
		end := idx
		for end < len(s) && (s[end] >= '0' && s[end] <= '9') {
			end++
		}
		return s[idx:end]
	}
	return ""
}

// SaveCover saves the cover image to disk
func SaveCover(coverData []byte, coversPath string, bookID int64) (string, error) {
	if len(coverData) == 0 {
		return "", nil
	}

	// Determine format from header
	format := "webp"
	if len(coverData) >= 3 && coverData[0] == 0xFF && coverData[1] == 0xD8 {
		format = "jpg"
	} else if len(coverData) >= 8 && string(coverData[:8]) == "\x89PNG\r\n\x1a\n" {
		format = "png"
	}

	filename := fmt.Sprintf("%d.%s", bookID, format)
	coverPath := filepath.Join(coversPath, filename)

	if err := os.MkdirAll(coversPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create covers directory: %w", err)
	}

	if err := os.WriteFile(coverPath, coverData, 0644); err != nil {
		return "", fmt.Errorf("failed to save cover: %w", err)
	}

	return coverPath, nil
}

// LoadSidecar loads sidecar metadata file if it exists
func LoadSidecar(bookDir string, baseName string) (*BookMetadata, error) {
	// Try .cryptorum-metadata.json first, then .grimmory-metadata.json
	for _, name := range []string{".cryptorum-metadata.json", ".grimmory-metadata.json"} {
		sidecarPath := filepath.Join(bookDir, baseName+name)
		if _, err := os.Stat(sidecarPath); err == nil {
			data, err := os.ReadFile(sidecarPath)
			if err != nil {
				return nil, err
			}

			var metadata BookMetadata
			if err := json.Unmarshal(data, &metadata); err != nil {
				return nil, err
			}

			slog.Info("Loaded sidecar metadata", "path", sidecarPath)
			return &metadata, nil
		}
	}
	return nil, nil
}
