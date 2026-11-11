package images

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func dims(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := jpeg.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func TestSaveDownscales(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(7, bytes.NewReader(encodePNG(t, 3200, 1600))); err != nil {
		t.Fatal(err)
	}
	if w, h := dims(t, s.Path(7, false)); w != 1600 || h != 800 {
		t.Fatalf("full = %dx%d, want 1600x800", w, h)
	}
	if w, h := dims(t, s.Path(7, true)); w != 320 || h != 160 {
		t.Fatalf("thumb = %dx%d, want 320x160", w, h)
	}
}

func TestSmallImageStaysSmall(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	if err := s.Save(1, bytes.NewReader(encodePNG(t, 100, 50))); err != nil {
		t.Fatal(err)
	}
	if w, h := dims(t, s.Path(1, false)); w != 100 || h != 50 {
		t.Fatalf("full = %dx%d, want 100x50", w, h)
	}
}

func TestRejectsGarbageAndHugeUploads(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	if err := s.Save(1, strings.NewReader("not an image")); err != ErrInvalid {
		t.Fatalf("garbage err = %v", err)
	}
	if err := s.Save(1, bytes.NewReader(make([]byte, MaxUpload+10))); err != ErrInvalid {
		t.Fatalf("oversize err = %v", err)
	}
	if _, err := os.Stat(s.Path(1, false)); !os.IsNotExist(err) {
		t.Fatal("no file may remain after a rejected upload")
	}
}

func TestSweepAndRemove(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	for _, id := range []int64{1, 2} {
		if err := s.Save(id, bytes.NewReader(encodePNG(t, 10, 10))); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.Sweep(map[int64]bool{1: true})
	if err != nil || n != 2 {
		t.Fatalf("sweep = %d, %v; want 2 files of gift 2", n, err)
	}
	if _, err := os.Stat(s.Path(1, false)); err != nil {
		t.Fatal("kept gift must remain")
	}
	s.Remove(1)
	if _, err := os.Stat(s.Path(1, true)); !os.IsNotExist(err) {
		t.Fatal("Remove must delete the preview")
	}
}

func TestLargePhoto(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4000, 3000))
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	s, _ := NewStore(t.TempDir())
	if err := s.Save(3, &b); err != nil {
		t.Fatal(err)
	}
	if w, h := dims(t, s.Path(3, false)); w != 1600 || h != 1200 {
		t.Fatalf("full = %dx%d, want 1600x1200", w, h)
	}
}
