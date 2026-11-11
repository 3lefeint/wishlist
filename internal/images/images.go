// Package images stores gift pictures: one downscaled image and a small
// preview per gift, both re-encoded as JPEG.
package images

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
)

const (
	// MaxUpload is the largest accepted upload in bytes.
	MaxUpload = 10 << 20
	// maxPixels guards against decompression bombs.
	maxPixels = 30_000_000
	fullSize  = 1600
	thumbSize = 320
)

// ErrInvalid means the upload is not a usable image.
var ErrInvalid = errors.New("not a valid image")

// Store keeps the files in one directory.
type Store struct {
	dir string
}

// NewStore returns a store and makes sure the directory is writable.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	probe := filepath.Join(dir, ".probe")
	if err := os.WriteFile(probe, nil, 0o640); err != nil {
		return nil, fmt.Errorf("upload directory not writable: %w", err)
	}
	_ = os.Remove(probe)
	return &Store{dir: dir}, nil
}

// Path returns the file of a gift's image or preview.
func (s *Store) Path(giftID int64, thumb bool) string {
	name := strconv.FormatInt(giftID, 10)
	if thumb {
		name += "_thumb"
	}
	return filepath.Join(s.dir, name+".jpg")
}

// Save decodes an upload, downscales it and writes the image and the preview.
func (s *Store) Save(giftID int64, r io.Reader) error {
	data, err := io.ReadAll(io.LimitReader(r, MaxUpload+1))
	if err != nil {
		return err
	}
	if len(data) > MaxUpload {
		return ErrInvalid
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return ErrInvalid
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return ErrInvalid
	}
	// Shrink first, then flatten: the large decoded image is the memory peak.
	full := flatten(imaging.Fit(img, fullSize, fullSize, imaging.Lanczos))
	img = nil
	thumb := imaging.Fit(full, thumbSize, thumbSize, imaging.Lanczos)
	if err := write(s.Path(giftID, false), full, 85); err != nil {
		return err
	}
	if err := write(s.Path(giftID, true), thumb, 80); err != nil {
		_ = os.Remove(s.Path(giftID, false))
		return err
	}
	return nil
}

// flatten puts transparency onto white; JPEG has no alpha channel.
func flatten(img image.Image) image.Image {
	flat := imaging.New(img.Bounds().Dx(), img.Bounds().Dy(), color.White)
	return imaging.Overlay(flat, img, image.Pt(0, 0), 1)
}

func write(path string, img image.Image, quality int) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: quality}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Remove deletes the files of a gift; missing files are fine.
func (s *Store) Remove(giftID int64) {
	_ = os.Remove(s.Path(giftID, false))
	_ = os.Remove(s.Path(giftID, true))
}

// Sweep deletes files of gifts that no longer have an image, for example
// after their circle or person was deleted. It returns the number removed.
func (s *Store) Sweep(keep map[int64]bool) (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jpg") {
			continue
		}
		idPart := strings.TrimSuffix(strings.TrimSuffix(name, ".jpg"), "_thumb")
		id, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil || keep[id] {
			continue
		}
		if os.Remove(filepath.Join(s.dir, name)) == nil {
			removed++
		}
	}
	return removed, nil
}
