package metadata

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	CoverData    []byte   `json:"-"` // Cover image data
	PageCount    int      `json:"page_count,omitempty"`
	Language     string   `json:"language,omitempty"`
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
	ID    string `xml:"id,attr"`
	Value string `xml:",chardata"`
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

	// Extract ISBN from identifiers
	for _, id := range pkg.Metadata.ISBN {
		if strings.HasPrefix(id.Value, "978") || strings.HasPrefix(id.Value, "979") {
			metadata.ISBN = id.Value
			break
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
					metadata.CoverData = readZipEntry(reader, coverPath)
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
				metadata.CoverData = readZipEntry(reader, coverPath)
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

// PDF metadata extraction (simplified - using filename parsing)
func extractPDFMetadata(filePath string) (*BookMetadata, error) {
	// For now, use filename parsing
	// Full PDF metadata extraction would require pdfcpu library
	metadata := extractFromFilename(filePath)

	// Try to open PDF and extract basic info
	f, err := os.Open(filePath)
	if err != nil {
		return metadata, nil
	}
	defer f.Close()

	// Read first few KB to check for PDF header
	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	if n < 5 || string(buf[:5]) != "%PDF-" {
		return metadata, nil
	}

	metadata.CoverData = renderPDFCover(filePath)
	if len(metadata.CoverData) == 0 {
		metadata.CoverData = findFolderCoverImage(filePath)
	}

	return metadata, nil
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

	if len(metadata.CoverData) == 0 {
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
		if err == nil && len(data) > 0 {
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
		if err == nil && len(data) > 0 {
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
	cmd := exec.Command("pdftoppm", "-f", "1", "-singlefile", "-jpeg", "-r", "150", filePath, prefix)
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
	// For a complete implementation, we would use dhowden/tag library
	// For now, use filename parsing
	metadata := extractFromFilename(filePath)
	return metadata, nil
}

// MOBI metadata extraction (simplified)
func extractMOBIMetadata(filePath string) (*BookMetadata, error) {
	// MOBI format is complex, for now use filename parsing
	metadata := extractFromFilename(filePath)
	return metadata, nil
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
