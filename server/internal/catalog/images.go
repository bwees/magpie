package catalog

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"magpie/internal/catalog/api"
	"magpie/internal/platform/config"
)

const (
	imagesDir      = "images"
	imageDirPerm   = 0o750
	imageTempName  = ".download-*"
	variantMarker  = "-w"
	formatJPEG     = "jpeg"
	formatPNG      = "png"
	jpegQuality    = 85
	defaultImgExt  = ".jpg"
	globAnyChars   = "*"
	maxImageMemory = 64 << 20
)

// downloadSizes are the TMDB sizes cached per image kind: large enough to resize down for any screen.
var downloadSizes = map[string]string{
	api.ImageKindPoster:   "w780",
	api.ImageKindBackdrop: "w1280",
	api.ImageKindLogo:     "w500",
}

// VariantWidths are the widths images are resized to; a requested width rounds up to the next one.
var VariantWidths = []int{92, 154, 185, 342, 500, 780, 1280}

var ErrImageTooLarge = errors.New("catalog: image is too large")

// ImageCache keeps TMDB images under the data dir, one folder per Title, plus resized variants beside them.
type ImageCache struct {
	root string
	tmdb TMDB
}

func NewImageCache(env config.Env, tmdb TMDB) *ImageCache {
	return &ImageCache{root: filepath.Join(env.DataDir, imagesDir), tmdb: tmdb}
}

// Download fetches one image and replaces the cached copy, dropping its old resized variants.
func (c *ImageCache) Download(ctx context.Context, title string, kind string, tmdbPath string) (string, error) {
	body, err := c.tmdb.Image(ctx, tmdbPath, downloadSizes[kind])
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()

	dir := filepath.Join(c.root, title)
	if err := os.MkdirAll(dir, imageDirPerm); err != nil {
		return "", err
	}
	ext := filepath.Ext(tmdbPath)
	if ext == "" {
		ext = defaultImgExt
	}
	base := strings.ToLower(kind)
	target := filepath.Join(dir, base+ext)

	tmp, err := os.CreateTemp(dir, imageTempName)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.Copy(tmp, io.LimitReader(body, maxImageMemory)); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := c.removeFiles(dir, base); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return "", err
	}
	return target, nil
}

// Remove deletes a Title's cached images and variants.
func (c *ImageCache) Remove(title string) error {
	return os.RemoveAll(filepath.Join(c.root, title))
}

// RemoveKind deletes one image and its variants.
func (c *ImageCache) RemoveKind(title string, kind string) error {
	return c.removeFiles(filepath.Join(c.root, title), strings.ToLower(kind))
}

func (c *ImageCache) removeFiles(dir, base string) error {
	matches, err := filepath.Glob(filepath.Join(dir, base+globAnyChars))
	if err != nil {
		return err
	}
	var errs []error
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Variant returns a copy of the image at path no wider than width, rounded up to a VariantWidth.
// It returns path itself when no smaller variant applies or the format can't be resized.
func Variant(path string, width int) (string, error) {
	target, ok := variantWidth(width)
	if !ok {
		return path, nil
	}
	ext := filepath.Ext(path)
	variant := strings.TrimSuffix(path, ext) + variantMarker + strconv.Itoa(target) + ext
	if fresh, err := newerThan(variant, path); err != nil || fresh {
		return variant, err
	}

	src, format, err := decodeFile(path)
	if err != nil {
		return "", err
	}
	if src.Bounds().Dx() <= target || (format != formatJPEG && format != formatPNG) {
		return path, nil
	}
	if err := writeImage(variant, downscale(src, target), format); err != nil {
		return "", err
	}
	return variant, nil
}

func variantWidth(width int) (int, bool) {
	if width <= 0 {
		return 0, false
	}
	i := slices.IndexFunc(VariantWidths, func(w int) bool { return w >= width })
	if i < 0 {
		return 0, false
	}
	return VariantWidths[i], true
}

func newerThan(variant, original string) (bool, error) {
	v, err := os.Stat(variant)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	o, err := os.Stat(original)
	if err != nil {
		return false, err
	}
	return !v.ModTime().Before(o.ModTime()), nil
}

func decodeFile(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, "", fmt.Errorf("decoding %s: %w", path, err)
	}
	if cfg.Width*cfg.Height*4 > maxImageMemory {
		return nil, "", ErrImageTooLarge
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", fmt.Errorf("decoding %s: %w", path, err)
	}
	return img, format, nil
}

func writeImage(path string, img image.Image, format string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), imageTempName)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if format == formatPNG {
		err = png.Encode(tmp, img)
	} else {
		err = jpeg.Encode(tmp, img, &jpeg.Options{Quality: jpegQuality})
	}
	if err := errors.Join(err, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// downscale averages every source pixel that falls in each target pixel, which keeps detail and
// transparency when shrinking.
func downscale(src image.Image, width int) image.Image {
	b := src.Bounds()
	height := max(1, b.Dy()*width/b.Dx())
	dst := image.NewRGBA64(image.Rect(0, 0, width, height))
	for y := range height {
		y0, y1 := b.Min.Y+y*b.Dy()/height, b.Min.Y+(y+1)*b.Dy()/height
		for x := range width {
			x0, x1 := b.Min.X+x*b.Dx()/width, b.Min.X+(x+1)*b.Dx()/width
			xEnd, yEnd := max(x1, x0+1), max(y1, y0+1)
			n := uint64((xEnd - x0) * (yEnd - y0))
			var r, g, bl, a uint64
			for sy := y0; sy < yEnd; sy++ {
				for sx := x0; sx < xEnd; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, bl, a = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca)
				}
			}
			dst.SetRGBA64(x, y, color.RGBA64{R: uint16(r / n), G: uint16(g / n), B: uint16(bl / n), A: uint16(a / n)})
		}
	}
	return dst
}
