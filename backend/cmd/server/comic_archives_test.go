package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestComicArchivePages(t *testing.T) {
	for _, format := range []string{"cbz", "cbt", "cb7"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "comic."+format)
			pages := []string{"10.png", "2.png", "1.png"}
			var data bytes.Buffer
			switch format {
			case "cbz":
				w := zip.NewWriter(&data)
				for _, name := range pages {
					entry, err := w.Create(name)
					if err != nil {
						t.Fatal(err)
					}
					entry.Write([]byte(name))
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
			case "cbt":
				w := tar.NewWriter(&data)
				for _, name := range pages {
					if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(name))}); err != nil {
						t.Fatal(err)
					}
					w.Write([]byte(name))
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
			case "cb7":
				if _, err := exec.LookPath("7z"); err != nil {
					t.Skip("7z unavailable")
				}
				for _, name := range pages {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
						t.Fatal(err)
					}
				}
				cmd := exec.Command("7z", "a", "-t7z", archive, "10.png", "2.png", "1.png")
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, out)
				}
			}
			if format != "cb7" {
				if err := os.WriteFile(archive, data.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			count, err := countCbxPages(archive, format)
			if err != nil {
				t.Fatal(err)
			}
			if count != 3 {
				t.Fatalf("page count = %d", count)
			}
			for page, expected := range []string{"1.png", "2.png", "10.png"} {
				rec := httptest.NewRecorder()
				if format == "cbz" {
					serveCbzPage(rec, httptest.NewRequest("GET", "/", nil), archive, page+1)
				} else {
					serveCbrPage(rec, archive, page+1)
				}
				if rec.Code != 200 || rec.Body.String() != expected {
					t.Fatalf("page %d: status %d, body %q", page+1, rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestComicArchiveListing(t *testing.T) {
	for _, tool := range []string{"unrar", "bsdtar", "7z"} {
		t.Run(tool, func(t *testing.T) {
			listing := "10.jpg\n2.jpg\n__MACOSX/1.jpg\nComicInfo.xml\n"
			if tool == "7z" {
				listing = "Path = 10.jpg\nPath = 2.jpg\nPath = __MACOSX/1.jpg\nPath = ComicInfo.xml\n"
			}
			entries := parseCbrListOutput(tool, listing)
			if len(entries) != 2 || entries[0].Name != "2.jpg" || entries[1].Name != "10.jpg" {
				t.Fatalf("unexpected entries: %+v", entries)
			}
		})
	}
}
