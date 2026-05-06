package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ConversionResult is the canonical processed representation for text-first ebooks.
type ConversionResult struct {
	HTMLContent string
	PlainText   string
	TOC         []TocItem
	CSSPath     string
	EPUBPath    string
	Metadata    BookMetadata
}

// BookMetadata extracted from content.opf.
type BookMetadata struct {
	Title    string
	Creator  string
	Language string
}

type textBookCachePaths struct {
	BaseDir        string
	NormalizedEPUB string
	ExplodedDir    string
	HTMLCachePath  string
	TextCachePath  string
	TocCachePath   string
	MetaCachePath  string
	CSSCachePath   string
}

type epubContainerDocument struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// NCXDocument represents the structure of toc.ncx.
type NCXDocument struct {
	XMLName xml.Name `xml:"ncx"`
	NavMap  NavMap   `xml:"navMap"`
}

type NavMap struct {
	NavPoints []NavPoint `xml:"navPoint"`
}

type NavPoint struct {
	ID        string     `xml:"id,attr"`
	PlayOrder string     `xml:"playOrder,attr"`
	NavLabel  NavLabel   `xml:"navLabel"`
	Content   Content    `xml:"content"`
	Children  []NavPoint `xml:"navPoint"`
}

type NavLabel struct {
	Text string `xml:"text"`
}

type Content struct {
	Src string `xml:"src,attr"`
}

// OPFDocument represents content.opf structure (simplified).
type OPFDocument struct {
	XMLName  xml.Name    `xml:"package"`
	Metadata OPFMetadata `xml:"metadata"`
	Manifest Manifest    `xml:"manifest"`
	Spine    Spine       `xml:"spine"`
}

type OPFMetadata struct {
	Title    []string `xml:"title"`
	Creator  []string `xml:"creator"`
	Language []string `xml:"language"`
}

type Manifest struct {
	Items []ManifestItem `xml:"item"`
}

type ManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type Spine struct {
	ItemRefs []ItemRef `xml:"itemref"`
}

type ItemRef struct {
	IDRef string `xml:"idref,attr"`
}

var supportedTextBookFormats = map[string]bool{
	"epub": true, "azw3": true, "azw4": true, "mobi": true,
	"fb2": true, "docx": true, "html": true,
	"rtf": true, "txt": true, "text": true, "odt": true,
	"pdb": true, "lrf": true,
}

func isSupportedTextBookFormat(format string) bool {
	return supportedTextBookFormats[strings.ToLower(format)]
}

func getTextBookCachePaths(bookID, format string) textBookCachePaths {
	cacheKey := strings.TrimSpace(bookID)
	if normalized := normalizeBookFormatKey(format); normalized != "" {
		cacheKey = cacheKey + "-" + normalized
	}
	baseDir := filepath.Join(appConfig.GetBookCachePath(), cacheKey)
	return textBookCachePaths{
		BaseDir:        baseDir,
		NormalizedEPUB: filepath.Join(baseDir, "canonical", "book.epub"),
		ExplodedDir:    filepath.Join(baseDir, "canonical", "exploded"),
		HTMLCachePath:  filepath.Join(baseDir, "content.html"),
		TextCachePath:  filepath.Join(baseDir, "content.txt"),
		TocCachePath:   filepath.Join(baseDir, "toc.json"),
		MetaCachePath:  filepath.Join(baseDir, "metadata.json"),
		CSSCachePath:   filepath.Join(baseDir, "styles.css"),
	}
}

func normalizeBookFormatKey(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	format = strings.TrimPrefix(format, ".")
	format = strings.ReplaceAll(format, string(filepath.Separator), "-")
	format = strings.ReplaceAll(format, " ", "-")
	return format
}

func ensureProcessedTextBook(bookID, filePath, format string) (*ConversionResult, error) {
	paths := getTextBookCachePaths(bookID, format)

	if isProcessedTextBookCacheValid(paths, filePath) {
		return loadProcessedTextBook(paths)
	}

	return processTextBookWithCalibre(bookID, filePath, format, paths)
}

func isProcessedTextBookCacheValid(paths textBookCachePaths, sourcePath string) bool {
	requiredPaths := []string{
		paths.NormalizedEPUB,
		paths.HTMLCachePath,
		paths.TextCachePath,
		paths.TocCachePath,
		paths.MetaCachePath,
		paths.CSSCachePath,
	}

	var newestCacheModTime int64
	for _, p := range requiredPaths {
		info, err := os.Stat(p)
		if err != nil {
			return false
		}
		if info.ModTime().Unix() > newestCacheModTime {
			newestCacheModTime = info.ModTime().Unix()
		}
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return false
	}

	return newestCacheModTime >= sourceInfo.ModTime().Unix()
}

func loadProcessedTextBook(paths textBookCachePaths) (*ConversionResult, error) {
	htmlContent, err := os.ReadFile(paths.HTMLCachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cached HTML: %w", err)
	}

	plainText, err := os.ReadFile(paths.TextCachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cached text: %w", err)
	}

	var toc []TocItem
	if data, err := os.ReadFile(paths.TocCachePath); err == nil {
		_ = json.Unmarshal(data, &toc)
	}

	var metadata BookMetadata
	if data, err := os.ReadFile(paths.MetaCachePath); err == nil {
		_ = json.Unmarshal(data, &metadata)
	}

	return &ConversionResult{
		HTMLContent: string(htmlContent),
		PlainText:   string(plainText),
		TOC:         toc,
		CSSPath:     paths.CSSCachePath,
		EPUBPath:    paths.NormalizedEPUB,
		Metadata:    metadata,
	}, nil
}

func processTextBookWithCalibre(bookID, filePath, format string, paths textBookCachePaths) (*ConversionResult, error) {
	if err := os.MkdirAll(filepath.Dir(paths.NormalizedEPUB), 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directories: %w", err)
	}

	if err := os.RemoveAll(paths.ExplodedDir); err != nil {
		return nil, fmt.Errorf("failed to clear old exploded cache: %w", err)
	}

	if err := convertWithCalibre(filePath, paths.NormalizedEPUB); err != nil {
		return nil, err
	}

	if err := unzipEPUB(paths.NormalizedEPUB, paths.ExplodedDir); err != nil {
		return nil, fmt.Errorf("failed to explode normalized epub: %w", err)
	}

	result, err := buildConversionResultFromExploded(bookID, paths)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(paths.HTMLCachePath, []byte(result.HTMLContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write HTML cache: %w", err)
	}
	if err := os.WriteFile(paths.TextCachePath, []byte(result.PlainText), 0644); err != nil {
		return nil, fmt.Errorf("failed to write text cache: %w", err)
	}
	if tocData, err := json.Marshal(result.TOC); err == nil {
		_ = os.WriteFile(paths.TocCachePath, tocData, 0644)
	}
	if metaData, err := json.Marshal(result.Metadata); err == nil {
		_ = os.WriteFile(paths.MetaCachePath, metaData, 0644)
	}

	return result, nil
}

func convertWithCalibre(filePath, outputEPUB string) error {
	calibrePath, err := exec.LookPath("ebook-convert")
	if err != nil {
		return fmt.Errorf("ebook-convert not found. Please ensure Calibre is installed")
	}

	tmpOutput := strings.TrimSuffix(outputEPUB, filepath.Ext(outputEPUB)) + ".tmp" + filepath.Ext(outputEPUB)
	_ = os.Remove(tmpOutput)

	cmd := exec.Command(calibrePath, filePath, tmpOutput)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	log.Printf("Running ebook-convert for %s\n", filePath)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpOutput)
		return fmt.Errorf("ebook-convert failed: %s", strings.TrimSpace(stderr.String()))
	}

	if err := os.Rename(tmpOutput, outputEPUB); err != nil {
		return fmt.Errorf("failed to finalize normalized epub: %w", err)
	}

	return nil
}

func unzipEPUB(epubPath, destDir string) error {
	reader, err := zip.OpenReader(epubPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	cleanDest := filepath.Clean(destDir)
	for _, f := range reader.File {
		targetPath := filepath.Join(destDir, f.Name)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(filepath.Separator)) && cleanTarget != cleanDest {
			return fmt.Errorf("invalid zip path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.Create(cleanTarget)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(dst, rc); err != nil {
			dst.Close()
			rc.Close()
			return err
		}
		dst.Close()
		rc.Close()
	}

	return nil
}

func buildConversionResultFromExploded(bookID string, paths textBookCachePaths) (*ConversionResult, error) {
	opfPath, pkg, err := loadExplodedOPF(paths.ExplodedDir)
	if err != nil {
		return nil, err
	}

	metadata := extractMetadataFromOPF(pkg)

	htmlContent, plainText, err := aggregateSpineContent(paths.ExplodedDir, opfPath, pkg, bookID)
	if err != nil {
		return nil, err
	}

	toc, err := buildTOC(paths.ExplodedDir, opfPath, pkg, htmlContent)
	if err != nil {
		log.Printf("Warning: failed to build TOC: %v\n", err)
		toc = []TocItem{}
	}

	cssPath, err := buildCombinedStylesheet(paths.ExplodedDir, opfPath, pkg, paths.CSSCachePath, bookID)
	if err != nil {
		log.Printf("Warning: failed to build combined stylesheet: %v\n", err)
		cssPath = paths.CSSCachePath
		_ = os.WriteFile(cssPath, []byte("/* No preserved styles available */"), 0644)
	}

	return &ConversionResult{
		HTMLContent: htmlContent,
		PlainText:   plainText,
		TOC:         toc,
		CSSPath:     cssPath,
		EPUBPath:    paths.NormalizedEPUB,
		Metadata:    metadata,
	}, nil
}

func loadExplodedOPF(explodedDir string) (string, *OPFDocument, error) {
	containerPath := filepath.Join(explodedDir, "META-INF", "container.xml")
	data, err := os.ReadFile(containerPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read container.xml: %w", err)
	}

	var container epubContainerDocument
	if err := xml.Unmarshal(data, &container); err != nil {
		return "", nil, fmt.Errorf("failed to parse container.xml: %w", err)
	}
	if len(container.Rootfiles) == 0 {
		return "", nil, fmt.Errorf("no rootfile found in container.xml")
	}

	opfPath := filepath.Join(explodedDir, filepath.FromSlash(container.Rootfiles[0].FullPath))
	opfData, err := os.ReadFile(opfPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read OPF file: %w", err)
	}

	var pkg OPFDocument
	if err := xml.Unmarshal(opfData, &pkg); err != nil {
		return "", nil, fmt.Errorf("failed to parse OPF XML: %w", err)
	}

	return opfPath, &pkg, nil
}

func extractMetadataFromOPF(pkg *OPFDocument) BookMetadata {
	metadata := BookMetadata{}
	if len(pkg.Metadata.Title) > 0 {
		metadata.Title = pkg.Metadata.Title[0]
	}
	if len(pkg.Metadata.Creator) > 0 {
		metadata.Creator = pkg.Metadata.Creator[0]
	}
	if len(pkg.Metadata.Language) > 0 {
		metadata.Language = pkg.Metadata.Language[0]
	}
	return metadata
}

func aggregateSpineContent(explodedDir, opfPath string, pkg *OPFDocument, bookID string) (string, string, error) {
	manifest := make(map[string]ManifestItem)
	for _, item := range pkg.Manifest.Items {
		manifest[item.ID] = item
	}

	opfDir := filepath.Dir(opfPath)
	var combinedHTML strings.Builder
	var plainText strings.Builder

	for _, ref := range pkg.Spine.ItemRefs {
		item, ok := manifest[ref.IDRef]
		if !ok {
			continue
		}

		if !isHTMLMediaType(item.MediaType) {
			continue
		}

		sectionPath := filepath.Join(opfDir, filepath.FromSlash(item.Href))
		data, err := os.ReadFile(sectionPath)
		if err != nil {
			continue
		}

		bodyContent := extractBodyContent(string(data))
		sectionDirRel, err := filepath.Rel(explodedDir, filepath.Dir(sectionPath))
		if err != nil {
			sectionDirRel = ""
		}

		rewrittenBody := rewriteHTMLResourcePaths(bodyContent, bookID, filepath.ToSlash(sectionDirRel))
		combinedHTML.WriteString(rewrittenBody)
		combinedHTML.WriteString("\n")
		plainText.WriteString(stripHTMLTags(rewrittenBody))
		plainText.WriteString(" ")
	}

	return combinedHTML.String(), strings.Join(strings.Fields(plainText.String()), " "), nil
}

func buildTOC(explodedDir, opfPath string, pkg *OPFDocument, htmlContent string) ([]TocItem, error) {
	opfDir := filepath.Dir(opfPath)
	manifest := make(map[string]ManifestItem)
	for _, item := range pkg.Manifest.Items {
		manifest[item.ID] = item
	}

	for _, item := range pkg.Manifest.Items {
		if strings.Contains(item.MediaType, "ncx") {
			ncxPath := filepath.Join(opfDir, filepath.FromSlash(item.Href))
			if toc, err := parseNCXFile(ncxPath); err == nil {
				return toc, nil
			}
		}
	}

	for _, item := range pkg.Manifest.Items {
		if strings.Contains(item.Properties, "nav") || strings.Contains(item.Href, "nav") {
			navPath := filepath.Join(opfDir, filepath.FromSlash(item.Href))
			if toc, err := parseNavDocument(navPath); err == nil && len(toc) > 0 {
				return toc, nil
			}
		}
	}

	return extractTocFromHTML(htmlContent), nil
}

func parseNCXFile(ncxPath string) ([]TocItem, error) {
	data, err := os.ReadFile(ncxPath)
	if err != nil {
		return nil, err
	}

	var ncx NCXDocument
	if err := xml.Unmarshal(data, &ncx); err != nil {
		return nil, err
	}

	return buildTocFromNavPoints(ncx.NavMap.NavPoints, 1), nil
}

func buildTocFromNavPoints(navPoints []NavPoint, level int) []TocItem {
	var toc []TocItem
	for _, np := range navPoints {
		item := TocItem{
			ID:    sanitizeTOCID(np.Content.Src),
			Label: strings.TrimSpace(np.NavLabel.Text),
			Level: level,
		}
		if len(np.Children) > 0 {
			item.Children = buildTocFromNavPoints(np.Children, level+1)
		}
		toc = append(toc, item)
	}
	return toc
}

func sanitizeTOCID(src string) string {
	if idx := strings.Index(src, "#"); idx >= 0 && idx+1 < len(src) {
		return src[idx+1:]
	}
	return ""
}

func parseNavDocument(navPath string) ([]TocItem, error) {
	data, err := os.ReadFile(navPath)
	if err != nil {
		return nil, err
	}

	content := string(data)
	navRe := regexp.MustCompile(`(?is)<nav[^>]*?>.*?<ol>(.*?)</ol>.*?</nav>`)
	liRe := regexp.MustCompile(`(?is)<li[^>]*>.*?<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>.*?</li>`)

	navMatch := navRe.FindStringSubmatch(content)
	if len(navMatch) < 2 {
		return nil, fmt.Errorf("no nav ol found")
	}

	var toc []TocItem
	for _, match := range liRe.FindAllStringSubmatch(navMatch[1], -1) {
		label := strings.TrimSpace(stripHTMLTags(match[2]))
		if label == "" {
			continue
		}
		toc = append(toc, TocItem{
			ID:    sanitizeTOCID(match[1]),
			Label: label,
			Level: 1,
		})
	}

	return toc, nil
}

func buildCombinedStylesheet(explodedDir, opfPath string, pkg *OPFDocument, outputPath, bookID string) (string, error) {
	opfDir := filepath.Dir(opfPath)
	var cssFiles []string

	for _, item := range pkg.Manifest.Items {
		if strings.Contains(strings.ToLower(item.MediaType), "css") || strings.HasSuffix(strings.ToLower(item.Href), ".css") {
			cssFiles = append(cssFiles, filepath.Join(opfDir, filepath.FromSlash(item.Href)))
		}
	}

	sort.Strings(cssFiles)

	var combined bytes.Buffer
	for _, cssFile := range cssFiles {
		data, err := os.ReadFile(cssFile)
		if err != nil {
			continue
		}

		sectionDirRel, err := filepath.Rel(explodedDir, filepath.Dir(cssFile))
		if err != nil {
			sectionDirRel = ""
		}

		combined.WriteString(rewriteCSSResourcePaths(string(data), bookID, filepath.ToSlash(sectionDirRel)))
		combined.WriteString("\n")
	}

	if combined.Len() == 0 {
		combined.WriteString("/* No preserved styles available */")
	}

	if err := os.WriteFile(outputPath, combined.Bytes(), 0644); err != nil {
		return "", err
	}

	return outputPath, nil
}

func isHTMLMediaType(mediaType string) bool {
	mediaType = strings.ToLower(mediaType)
	return strings.Contains(mediaType, "html") || strings.Contains(mediaType, "xhtml") || strings.Contains(mediaType, "xml")
}

func rewriteHTMLResourcePaths(content, bookID, sectionDir string) string {
	attrRe := regexp.MustCompile(`(?i)(src|href)=("([^"]+)"|'([^']+)')`)
	return attrRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := attrRe.FindStringSubmatch(match)
		if len(sub) < 5 {
			return match
		}

		attr := sub[1]
		rawValue := sub[3]
		quote := `"`
		if rawValue == "" {
			rawValue = sub[4]
			quote = `'`
		}

		lower := strings.ToLower(rawValue)
		if strings.HasPrefix(lower, "http://") ||
			strings.HasPrefix(lower, "https://") ||
			strings.HasPrefix(lower, "data:") ||
			strings.HasPrefix(lower, "mailto:") ||
			strings.HasPrefix(lower, "#") {
			return match
		}

		if strings.HasSuffix(lower, ".css") {
			return fmt.Sprintf(`%s=%s/api/books/%s/continuous/styles%s`, attr, quote, bookID, quote)
		}

		if isHTMLLikePath(lower) {
			return match
		}

		cleanPath := normalizeRelativeAssetPath(sectionDir, rawValue)
		return fmt.Sprintf(`%s=%s/api/books/%s/continuous/media/%s%s`, attr, quote, bookID, cleanPath, quote)
	})
}

func rewriteCSSResourcePaths(content, bookID, sectionDir string) string {
	urlRe := regexp.MustCompile(`url\(([^)]+)\)`)
	return urlRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := urlRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}

		raw := strings.TrimSpace(sub[1])
		raw = strings.Trim(raw, `"'`)
		lower := strings.ToLower(raw)
		if raw == "" ||
			strings.HasPrefix(lower, "http://") ||
			strings.HasPrefix(lower, "https://") ||
			strings.HasPrefix(lower, "data:") ||
			strings.HasPrefix(lower, "#") {
			return match
		}

		cleanPath := normalizeRelativeAssetPath(sectionDir, raw)
		return fmt.Sprintf(`url("/api/books/%s/continuous/media/%s")`, bookID, cleanPath)
	})
}

func normalizeRelativeAssetPath(baseDir, rawPath string) string {
	rawPath = strings.TrimSpace(rawPath)
	rawPath = strings.TrimPrefix(rawPath, "./")
	rawPath = filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, rawPath)))
	return strings.TrimPrefix(rawPath, "/")
}

func isHTMLLikePath(path string) bool {
	return strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".xhtml") || strings.HasSuffix(path, ".xml")
}

// extractBodyContent extracts content between <body> and </body> tags.
func extractBodyContent(html string) string {
	bodyStart := strings.Index(strings.ToLower(html), "<body")
	bodyEnd := strings.LastIndex(strings.ToLower(html), "</body>")

	if bodyStart == -1 || bodyEnd == -1 {
		return html
	}

	bodyTagEnd := strings.Index(html[bodyStart:], ">")
	if bodyTagEnd == -1 {
		return html
	}

	return html[bodyStart+bodyTagEnd+1 : bodyEnd]
}
