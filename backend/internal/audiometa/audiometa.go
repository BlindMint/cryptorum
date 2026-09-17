package audiometa

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var supportedFormats = map[string]bool{
	"mp3": true, "m4a": true, "m4b": true, "flac": true, "ogg": true, "wav": true,
}

type Chapter struct {
	Title string
	Start float64
	End   float64
}

type Metadata struct {
	Title         string
	Artists       []string
	AlbumArtist   string
	Album         string
	TrackNumber   *int
	DiscNumber    *int
	ReleaseDate   string
	Genre         string
	Duration      float64
	ShowTitle     string
	EpisodeNumber *int
	PublishedAt   string
	Chapters      []Chapter
}

func Supported(format string) bool {
	return supportedFormats[strings.ToLower(strings.TrimSpace(format))]
}

func Probe(path string) (Metadata, error) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return Metadata{}, err
	}
	output, err := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_chapters", path).Output()
	if err != nil {
		return Metadata{}, err
	}
	var parsed struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
		Chapters []struct {
			Start string            `json:"start_time"`
			End   string            `json:"end_time"`
			Tags  map[string]string `json:"tags"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return Metadata{}, err
	}
	tags := lowerTags(parsed.Format.Tags)
	metadata := Metadata{
		Title:       tags["title"],
		Artists:     splitList(first(tags["artist"], tags["author"], tags["composer"])),
		AlbumArtist: tags["album_artist"],
		Album:       tags["album"],
		TrackNumber: parseNumber(tags["track"]),
		DiscNumber:  parseNumber(tags["disc"]),
		ReleaseDate: first(tags["date"], tags["year"]),
		Genre:       tags["genre"],
		ShowTitle:   first(tags["show"], tags["podcast"]),
		PublishedAt: first(tags["creation_time"], tags["date"]),
	}
	metadata.EpisodeNumber = parseNumber(first(tags["episode_id"], tags["episode_sort"], tags["episode"]))
	metadata.Duration, _ = strconv.ParseFloat(parsed.Format.Duration, 64)
	for index, chapter := range parsed.Chapters {
		start, startErr := strconv.ParseFloat(chapter.Start, 64)
		end, endErr := strconv.ParseFloat(chapter.End, 64)
		if startErr != nil || endErr != nil || end <= start {
			continue
		}
		chapterTags := lowerTags(chapter.Tags)
		title := chapterTags["title"]
		if title == "" {
			title = fmt.Sprintf("Chapter %d", index+1)
		}
		metadata.Chapters = append(metadata.Chapters, Chapter{Title: title, Start: start, End: end})
	}
	return metadata, nil
}

func SyncFile(db *sql.DB, ownerID, bookID, fileID int64, path, format, sourceHash string) error {
	if !Supported(format) {
		return nil
	}
	var itemID int64
	var storedHash, lockedJSON, category string
	err := db.QueryRow(`SELECT id, source_hash, locked_fields, category FROM audio_item WHERE owner_user_id = ? AND file_id = ?`, ownerID, fileID).Scan(&itemID, &storedHash, &lockedJSON, &category)
	if err == nil && storedHash == sourceHash && storedHash != "" {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	metadata, probeErr := Probe(path)
	storedSourceHash := sourceHash
	metadataSource := "ffprobe"
	if probeErr != nil {
		storedSourceHash = ""
		metadataSource = "fallback"
	}
	var fallbackTitle, fallbackArtists string
	_ = db.QueryRow(`SELECT COALESCE(title, ''), COALESCE(authors, '[]') FROM book_metadata WHERE book_id = ?`, bookID).Scan(&fallbackTitle, &fallbackArtists)
	if metadata.Title == "" {
		metadata.Title = fallbackTitle
	}
	if len(metadata.Artists) == 0 {
		_ = json.Unmarshal([]byte(fallbackArtists), &metadata.Artists)
	}
	artistsJSON, _ := json.Marshal(metadata.Artists)
	if category == "" {
		_ = db.QueryRow(`SELECT COALESCE(audio_default_category, 'audiobook') FROM library WHERE id = (SELECT library_id FROM book WHERE id = ?)`, bookID).Scan(&category)
	}
	if category != "music" && category != "podcast" {
		category = "audiobook"
	}
	locked := map[string]bool{}
	var lockedFields []string
	_ = json.Unmarshal([]byte(lockedJSON), &lockedFields)
	for _, field := range lockedFields {
		locked[field] = true
	}
	now := time.Now().Unix()
	if itemID == 0 {
		result, insertErr := db.Exec(`
			INSERT INTO audio_item (
				owner_user_id, book_id, file_id, category, title, artists, album_artist, album,
				track_number, disc_number, release_date, genre, duration_seconds, show_title,
				episode_number, published_at, metadata_source, source_hash, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ownerID, bookID, fileID, category, metadata.Title, string(artistsJSON), metadata.AlbumArtist,
			metadata.Album, metadata.TrackNumber, metadata.DiscNumber, metadata.ReleaseDate, metadata.Genre,
			metadata.Duration, metadata.ShowTitle, metadata.EpisodeNumber, metadata.PublishedAt, metadataSource,
			storedSourceHash, now, now)
		if insertErr != nil {
			return insertErr
		}
		itemID, _ = result.LastInsertId()
	} else {
		var current Metadata
		var currentArtists string
		_ = db.QueryRow(`SELECT title, artists, album_artist, album, track_number, disc_number, release_date, genre, duration_seconds, show_title, episode_number, published_at FROM audio_item WHERE id = ?`, itemID).Scan(
			&current.Title, &currentArtists, &current.AlbumArtist, &current.Album, &current.TrackNumber, &current.DiscNumber,
			&current.ReleaseDate, &current.Genre, &current.Duration, &current.ShowTitle, &current.EpisodeNumber, &current.PublishedAt)
		if locked["title"] {
			metadata.Title = current.Title
		}
		if locked["artists"] {
			artistsJSON = []byte(currentArtists)
		}
		if locked["album_artist"] {
			metadata.AlbumArtist = current.AlbumArtist
		}
		if locked["album"] {
			metadata.Album = current.Album
		}
		if locked["track_number"] {
			metadata.TrackNumber = current.TrackNumber
		}
		if locked["disc_number"] {
			metadata.DiscNumber = current.DiscNumber
		}
		if locked["release_date"] {
			metadata.ReleaseDate = current.ReleaseDate
		}
		if locked["genre"] {
			metadata.Genre = current.Genre
		}
		if locked["show_title"] {
			metadata.ShowTitle = current.ShowTitle
		}
		if locked["episode_number"] {
			metadata.EpisodeNumber = current.EpisodeNumber
		}
		if locked["published_at"] {
			metadata.PublishedAt = current.PublishedAt
		}
		_, err = db.Exec(`UPDATE audio_item SET book_id = ?, category = ?, title = ?, artists = ?, album_artist = ?, album = ?, track_number = ?, disc_number = ?, release_date = ?, genre = ?, duration_seconds = ?, show_title = ?, episode_number = ?, published_at = ?, metadata_source = ?, source_hash = ?, updated_at = ? WHERE id = ?`,
			bookID, category, metadata.Title, string(artistsJSON), metadata.AlbumArtist, metadata.Album,
			metadata.TrackNumber, metadata.DiscNumber, metadata.ReleaseDate, metadata.Genre, metadata.Duration,
			metadata.ShowTitle, metadata.EpisodeNumber, metadata.PublishedAt, metadataSource, storedSourceHash, now, itemID)
		if err != nil {
			return err
		}
	}
	if probeErr == nil {
		if _, err := db.Exec(`DELETE FROM audio_chapter WHERE audio_item_id = ?`, itemID); err != nil {
			return err
		}
		for position, chapter := range metadata.Chapters {
			if _, err := db.Exec(`INSERT INTO audio_chapter (audio_item_id, position, title, start_seconds, end_seconds) VALUES (?, ?, ?, ?, ?)`, itemID, position, chapter.Title, chapter.Start, chapter.End); err != nil {
				return err
			}
		}
	}
	groupKey := "book:" + strconv.FormatInt(bookID, 10)
	if category == "podcast" && strings.TrimSpace(metadata.ShowTitle) != "" {
		groupKey = "show:" + strings.ToLower(strings.TrimSpace(metadata.ShowTitle))
	}
	if category == "audiobook" || category == "podcast" {
		var preferredSpeed float64
		if err := db.QueryRow(`SELECT playback_speed FROM audio_playback_preference WHERE owner_user_id = ? AND category = ? AND group_key = ?`, ownerID, category, groupKey).Scan(&preferredSpeed); err == nil {
			_, _ = db.Exec(`UPDATE audio_item SET playback_speed = ? WHERE id = ?`, preferredSpeed, itemID)
		}
	}
	return nil
}

func lowerTags(values map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range values {
		result[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return result
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == '&' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func parseNumber(value string) *int {
	value = strings.TrimSpace(strings.Split(value, "/")[0])
	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		return nil
	}
	return &number
}
