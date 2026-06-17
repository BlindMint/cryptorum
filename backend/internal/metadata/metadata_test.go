package metadata

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCBZMetadataUsesComicInfoFrontCover(t *testing.T) {
	red := pngBytes(t, color.NRGBA{R: 220, A: 255})
	blue := pngBytes(t, color.NRGBA{B: 220, A: 255})
	cbzPath := writeCBZ(t, map[string][]byte{
		"Pages/Page 01.png": red,
		"Pages/Page 02.png": blue,
		"ComicInfo.xml": []byte(`<ComicInfo>
			<Title>ComicInfo Title</Title>
			<Pages>
				<Page Image="0" Type="Story" />
				<Page Image="1" Type="FrontCover" />
			</Pages>
		</ComicInfo>`),
	})

	meta, err := Extract(cbzPath)
	if err != nil {
		t.Fatalf("extract metadata: %v", err)
	}
	if meta.Title != "ComicInfo Title" {
		t.Fatalf("expected ComicInfo title, got %q", meta.Title)
	}
	if meta.PageCount != 2 {
		t.Fatalf("expected 2 pages, got %d", meta.PageCount)
	}
	assertCoverColor(t, meta.CoverData, color.NRGBA{B: 220, A: 255})
}

func TestExtractCBZMetadataPrefersNamedCoverImage(t *testing.T) {
	red := pngBytes(t, color.NRGBA{R: 220, A: 255})
	blue := pngBytes(t, color.NRGBA{B: 220, A: 255})
	cbzPath := writeCBZ(t, map[string][]byte{
		"Pages/Page 001.png":  red,
		"ZZZ/front_cover.png": blue,
	})

	meta, err := Extract(cbzPath)
	if err != nil {
		t.Fatalf("extract metadata: %v", err)
	}
	assertCoverColor(t, meta.CoverData, color.NRGBA{B: 220, A: 255})
}

func TestExtractCBZMetadataFallsBackToFirstPage(t *testing.T) {
	red := pngBytes(t, color.NRGBA{R: 220, A: 255})
	green := pngBytes(t, color.NRGBA{G: 220, A: 255})
	blue := pngBytes(t, color.NRGBA{B: 220, A: 255})
	cbzPath := writeCBZ(t, map[string][]byte{
		"Pages/Page 10.png": blue,
		"Pages/Page 2.png":  green,
		"Pages/Page 1.png":  red,
	})

	meta, err := Extract(cbzPath)
	if err != nil {
		t.Fatalf("extract metadata: %v", err)
	}
	assertCoverColor(t, meta.CoverData, color.NRGBA{R: 220, A: 255})
}

func pngBytes(t *testing.T, c color.NRGBA) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, c)
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

func writeCBZ(t *testing.T, entries map[string][]byte) string {
	t.Helper()

	cbzPath := filepath.Join(t.TempDir(), "Comic.cbz")
	file, err := os.Create(cbzPath)
	if err != nil {
		t.Fatalf("create cbz: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, data := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create cbz entry %s: %v", name, err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatalf("write cbz entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close cbz: %v", err)
	}

	return cbzPath
}

func assertCoverColor(t *testing.T, data []byte, expected color.NRGBA) {
	t.Helper()
	if len(data) == 0 {
		t.Fatal("expected cover data")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode cover data: %v", err)
	}
	got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if got != expected {
		t.Fatalf("expected cover color %+v, got %+v", expected, got)
	}
}
