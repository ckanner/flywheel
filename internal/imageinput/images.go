// Package imageinput validates frozen image references and bounded native
// attachments. Paths authorize reads; a MIME label alone never proves an image.
package imageinput

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"

	_ "golang.org/x/image/webp"
)

const (
	MaxImages     = 4
	MaxImageBytes = 4 << 20
	MaxTotalBytes = 8 << 20
	MaxDimension  = 8192
	MaxPixels     = 16_000_000
)

var mediaTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

type Reference struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	MediaType string `json:"media-type"`
}

type References struct {
	Version int         `json:"version"`
	Images  []Reference `json:"images"`
}

// Image is the public machine attachment. encoding/json encodes Data as
// canonical base64; no path or data-URL string substitutes for actual bytes.
type Image struct {
	MIMEType string `json:"mimeType"`
	Data     []byte `json:"data"`
}

type ImageInfo struct {
	MIMEType string `json:"mimeType"`
	SHA256   string `json:"sha256"`
	Bytes    int    `json:"bytes"`
}

// Manifest is safe to retain in invocation state and native receipts. Original
// image bytes remain in the source file and the native client's own history.
type Manifest struct {
	Version int         `json:"version"`
	Images  []ImageInfo `json:"images"`
}

func Validate(refs *References) error {
	if refs == nil {
		return nil
	}
	if refs.Version != 1 || len(refs.Images) == 0 || len(refs.Images) > MaxImages {
		return fmt.Errorf("input-images requires version 1 and one to four images")
	}
	seen := map[string]bool{}
	for _, ref := range refs.Images {
		if ref.Path == "" || ref.Path == "." || ref.Path == ".." || len(ref.Path) > 512 || !utf8.ValidString(ref.Path) ||
			path.IsAbs(ref.Path) || path.Clean(ref.Path) != ref.Path || strings.HasPrefix(ref.Path, "../") ||
			strings.ContainsAny(ref.Path, "\\:") || strings.IndexFunc(ref.Path, unicode.IsControl) >= 0 ||
			slices.Contains(strings.Split(ref.Path, "/"), ".git") {
			return fmt.Errorf("input image path must be a clean repository-relative file path: %q", ref.Path)
		}
		if seen[ref.Path] {
			return fmt.Errorf("input image path is repeated: %q", ref.Path)
		}
		seen[ref.Path] = true
		digest, err := hex.DecodeString(ref.SHA256)
		if err != nil || len(digest) != sha256.Size || strings.ToLower(ref.SHA256) != ref.SHA256 || !slices.Contains(mediaTypes, ref.MediaType) {
			return fmt.Errorf("input image %q requires a lowercase SHA-256 and PNG/JPEG/GIF/WebP MIME type", ref.Path)
		}
	}
	return nil
}

func Describe(images []Image) *Manifest {
	if len(images) == 0 {
		return nil
	}
	m := &Manifest{Version: 1, Images: make([]ImageInfo, 0, len(images))}
	for _, img := range images {
		m.Images = append(m.Images, ImageInfo{MIMEType: img.MIMEType, SHA256: fmt.Sprintf("%x", sha256.Sum256(img.Data)), Bytes: len(img.Data)})
	}
	return m
}

func ValidateManifest(m *Manifest) error {
	if m == nil {
		return nil
	}
	if m.Version != 1 || len(m.Images) == 0 || len(m.Images) > MaxImages {
		return fmt.Errorf("input image manifest requires version 1 and one to four images")
	}
	total := 0
	for _, img := range m.Images {
		digest, err := hex.DecodeString(img.SHA256)
		if err != nil || len(digest) != sha256.Size || strings.ToLower(img.SHA256) != img.SHA256 ||
			!slices.Contains(mediaTypes, img.MIMEType) || img.Bytes < 1 || img.Bytes > MaxImageBytes {
			return fmt.Errorf("input image manifest has an invalid type, SHA-256 or size (maximum 4 MiB per image)")
		}
		total += img.Bytes
	}
	if total > MaxTotalBytes {
		return fmt.Errorf("input images exceed the 8 MiB combined byte limit")
	}
	return nil
}

// Load resolves references in the task's repository checkout, independently of
// any packet worktree. Every use revalidates the frozen bytes before dispatch.
func Load(directory string, refs *References) ([]Image, error) {
	if err := Validate(refs); err != nil {
		return nil, err
	}
	if refs == nil {
		return nil, nil
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	images := make([]Image, 0, len(refs.Images))
	total := 0
	for _, ref := range refs.Images {
		raw, err := readImage(root, ref.Path)
		if err != nil {
			return nil, fmt.Errorf("input image %q: %w", ref.Path, err)
		}
		total += len(raw)
		if total > MaxTotalBytes {
			return nil, fmt.Errorf("input images exceed the 8 MiB combined byte limit")
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != ref.SHA256 {
			return nil, fmt.Errorf("input image %q changed: SHA-256 does not match the frozen packet", ref.Path)
		}
		if err := validateRaster(raw, ref.MediaType); err != nil {
			return nil, fmt.Errorf("input image %q: %w", ref.Path, err)
		}
		images = append(images, Image{MIMEType: ref.MediaType, Data: raw})
	}
	return images, nil
}

type imageFile interface {
	io.ReadCloser
	Stat() (os.FileInfo, error)
}

// Narrow file seam covers actual open/stat/read failures without relaxing path
// confinement or replacing the image decoder in production.
var openImageFile = func(root *os.Root, name string) (imageFile, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func readImage(root *os.Root, name string) ([]byte, error) {
	var expected os.FileInfo
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlinks are not allowed in input image paths")
		}
		expected = info
	}
	if !expected.Mode().IsRegular() || expected.Size() < 1 || expected.Size() > MaxImageBytes {
		return nil, fmt.Errorf("input image must be a nonempty regular file of at most 4 MiB")
	}
	f, err := openImageFile(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(expected, actual) {
		return nil, fmt.Errorf("input image path changed before opening")
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || len(raw) > MaxImageBytes {
		return nil, fmt.Errorf("input image changed size while reading (maximum 4 MiB)")
	}
	return raw, nil
}

func validateRaster(raw []byte, mediaType string) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || "image/"+format != mediaType {
		return fmt.Errorf("input bytes are not a decodable image matching the declared MIME type")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > MaxDimension || config.Height > MaxDimension || int64(config.Width)*int64(config.Height) > MaxPixels {
		return fmt.Errorf("input image exceeds the 8192-pixel dimension or 16-million-pixel limit")
	}
	if _, _, err := image.Decode(bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("input image content is corrupt: %w", err)
	}
	return nil
}
