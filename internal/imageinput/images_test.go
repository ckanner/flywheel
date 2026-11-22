package imageinput

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func raster(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 220, G: 30, B: 40, A: 255})
	var out bytes.Buffer
	switch format {
	case "png":
		require.NoError(t, png.Encode(&out, img))
	case "jpeg":
		require.NoError(t, jpeg.Encode(&out, img, nil))
	case "gif":
		require.NoError(t, gif.Encode(&out, img, nil))
	case "webp":
		// Generated two-by-two solid RGB fixture; no downloaded image content.
		data, err := base64.StdEncoding.DecodeString("UklGRh4AAABXRUJQVlA4TBEAAAAvAUAAAAdQj3IXpf+BiOh/AAA=")
		require.NoError(t, err)
		return data
	}
	return out.Bytes()
}

func source(name, mediaType string, raw []byte) Reference {
	return Reference{Path: name, SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), MediaType: mediaType}
}

func TestFrozenReferenceValidation(t *testing.T) {
	require.NoError(t, Validate(nil))
	valid := References{Version: 1, Images: []Reference{source("design/页面 image.png", "image/png", []byte("frozen"))}}
	require.NoError(t, Validate(&valid))
	for _, edit := range []func(*References){
		func(r *References) { r.Version = 2 },
		func(r *References) { r.Images = nil },
		func(r *References) { r.Images = make([]Reference, 5) },
		func(r *References) { r.Images = append(r.Images, r.Images[0]) },
		func(r *References) { r.Images[0].SHA256 = "invalid" },
		func(r *References) { r.Images[0].SHA256 = strings.Repeat("a", 62) },
		func(r *References) { r.Images[0].SHA256 = strings.Repeat("A", 64) },
		func(r *References) { r.Images[0].MediaType = "image/svg+xml" },
	} {
		candidate := References{Version: 1, Images: append([]Reference{}, valid.Images...)}
		edit(&candidate)
		require.Error(t, Validate(&candidate))
	}
	for _, path := range []string{"", ".", "..", "../outside.png", "a/../b.png", "/absolute.png", "C:/file.png", "https://example.test/file.png", "a//b.png", "a\\b.png", "a/", "a\x00b", "a\nb", "a\tb", "a\u0085b", ".git/config", "nested/.git/HEAD", string([]byte{0xff}), strings.Repeat("a", 513)} {
		r := References{Version: 1, Images: []Reference{source(path, "image/png", []byte("frozen"))}}
		require.Error(t, Validate(&r), path)
	}
}

func TestTypedImagesEncodeActualBytesAndBoundMetadata(t *testing.T) {
	require.Nil(t, Describe(nil))
	require.NoError(t, ValidateManifest(nil))
	img := Image{MIMEType: "image/png", Data: raster(t, "png")}
	manifest := Describe([]Image{img})
	require.NoError(t, ValidateManifest(manifest))
	require.Equal(t, source("ignored", "image/png", img.Data).SHA256, manifest.Images[0].SHA256)
	require.Equal(t, len(img.Data), manifest.Images[0].Bytes)
	raw, err := json.Marshal(img)
	require.NoError(t, err)
	var wire map[string]string
	require.NoError(t, json.Unmarshal(raw, &wire))
	require.Equal(t, base64.StdEncoding.EncodeToString(img.Data), wire["data"])
	require.Equal(t, "image/png", wire["mimeType"])
	for _, edit := range []func(*Manifest){
		func(m *Manifest) { m.Version = 2 },
		func(m *Manifest) { m.Images = nil },
		func(m *Manifest) { m.Images = make([]ImageInfo, 5) },
		func(m *Manifest) { m.Images[0].Bytes = 0 },
		func(m *Manifest) { m.Images[0].Bytes = MaxImageBytes + 1 },
		func(m *Manifest) { m.Images[0].MIMEType = "text/plain" },
		func(m *Manifest) { m.Images[0].SHA256 = "no" },
		func(m *Manifest) { m.Images[0].SHA256 = strings.Repeat("A", 64) },
		func(m *Manifest) { m.Images[0].SHA256 = strings.Repeat("a", 62) },
		func(m *Manifest) {
			m.Images[0].Bytes = 3 << 20
			m.Images = []ImageInfo{m.Images[0], m.Images[0], m.Images[0]}
		},
	} {
		candidate := Describe([]Image{img})
		edit(candidate)
		require.Error(t, ValidateManifest(candidate))
	}
}

func TestLoadVerifiesFourRasterFormatsAndPreservesOriginalBytes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "inputs"), 0o700))
	refs := &References{Version: 1}
	var want []Image
	for _, format := range []string{"png", "jpeg", "gif", "webp"} {
		raw := raster(t, format)
		name := "inputs/reference." + format
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), raw, 0o600))
		refs.Images = append(refs.Images, source(name, "image/"+format, raw))
		want = append(want, Image{MIMEType: "image/" + format, Data: raw})
	}
	got, err := Load(dir, refs)
	require.NoError(t, err)
	require.Equal(t, want, got)
	none, err := Load(filepath.Join(dir, "does-not-exist"), nil)
	require.NoError(t, err)
	require.Nil(t, none)
	_, err = Load(dir, &References{Version: 2})
	require.Error(t, err)
	_, err = Load(filepath.Join(dir, "does-not-exist"), refs)
	require.Error(t, err)
	refs.Images[0].SHA256 = strings.Repeat("0", 64)
	_, err = Load(dir, refs)
	require.ErrorContains(t, err, "SHA-256")
}

func TestLoadRejectsNonImagesMismatchesCorruptionAndPixelBombs(t *testing.T) {
	valid := raster(t, "png")
	tooWide := append([]byte{}, valid...)
	binary.BigEndian.PutUint32(tooWide[16:20], MaxDimension+1)
	binary.BigEndian.PutUint32(tooWide[29:33], crc32.ChecksumIEEE(tooWide[12:29]))
	tooMany := append([]byte{}, valid...)
	binary.BigEndian.PutUint32(tooMany[16:20], 5000)
	binary.BigEndian.PutUint32(tooMany[20:24], 5000)
	binary.BigEndian.PutUint32(tooMany[29:33], crc32.ChecksumIEEE(tooMany[12:29]))
	for _, tc := range []struct {
		name, mime, message string
		data                []byte
	}{
		{"not an image", "image/png", "decodable image", []byte("a text file with an image extension")},
		{"MIME mismatch", "image/jpeg", "MIME", valid},
		{"corrupt image content", "image/png", "content is corrupt", valid[:33]},
		{"wide image", "image/png", "pixel", tooWide},
		{"pixel count", "image/png", "pixel", tooMany},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "input.png"), tc.data, 0o600))
			_, err := Load(dir, &References{Version: 1, Images: []Reference{source("input.png", tc.mime, tc.data)}})
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestLoadEnforcesFileAndCombinedByteLimits(t *testing.T) {
	dir := t.TempDir()
	base := raster(t, "png")
	padded := append(append([]byte{}, base...), make([]byte, MaxImageBytes-len(base))...)
	refs := &References{Version: 1}
	for _, name := range []string{"first.png", "second.png"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), padded, 0o600))
		refs.Images = append(refs.Images, source(name, "image/png", padded))
	}
	images, err := Load(dir, refs)
	require.NoError(t, err)
	require.Equal(t, MaxTotalBytes, len(images[0].Data)+len(images[1].Data))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "third.png"), base, 0o600))
	refs.Images = append(refs.Images, source("third.png", "image/png", base))
	_, err = Load(dir, refs)
	require.ErrorContains(t, err, "8 MiB")
	tooLarge := append(padded, 0)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.png"), tooLarge, 0o600))
	_, err = Load(dir, &References{Version: 1, Images: []Reference{source("large.png", "image/png", tooLarge)}})
	require.ErrorContains(t, err, "4 MiB")
}

func TestLoadRejectsSymlinksDirectoriesSpecialAndMissingFiles(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	raw := raster(t, "png")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image.png"), raw, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "image.png"), raw, 0o600))
	require.NoError(t, os.Symlink("image.png", filepath.Join(dir, "inside.png")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "image.png"), filepath.Join(dir, "outside.png")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "parent")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.png"), nil, 0o600))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600))
	for _, name := range []string{"missing.png", "inside.png", "outside.png", "parent/image.png", "directory", "empty.png", "pipe", "image.png/child.png"} {
		_, err := Load(dir, &References{Version: 1, Images: []Reference{source(name, "image/png", raw)}})
		require.Error(t, err, name)
	}
}

type faultImageFile struct {
	io.Reader
	info    os.FileInfo
	statErr error
	closed  bool
}

func (f *faultImageFile) Stat() (os.FileInfo, error) { return f.info, f.statErr }
func (f *faultImageFile) Close() error               { f.closed = true; return nil }

type imageReadFailure struct{}

func (imageReadFailure) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestImageReadFailuresDoNotBypassIdentityOrBoundedRead(t *testing.T) {
	dir := t.TempDir()
	raw := raster(t, "png")
	file := filepath.Join(dir, "image.png")
	require.NoError(t, os.WriteFile(file, raw, 0o600))
	info, err := os.Stat(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.png"), raw, 0o600))
	other, err := os.Stat(filepath.Join(dir, "other.png"))
	require.NoError(t, err)
	original := openImageFile
	t.Cleanup(func() { openImageFile = original })
	refs := &References{Version: 1, Images: []Reference{source("image.png", "image/png", raw)}}
	openImageFile = func(*os.Root, string) (imageFile, error) { return nil, errors.New("open failed") }
	_, err = Load(dir, refs)
	require.ErrorContains(t, err, "open failed")
	for _, tc := range []struct {
		name string
		file *faultImageFile
	}{
		{"stat failed", &faultImageFile{Reader: bytes.NewReader(raw), statErr: errors.New("stat failed")}},
		{"path changed", &faultImageFile{Reader: bytes.NewReader(raw), info: other}},
		{"read failed", &faultImageFile{Reader: imageReadFailure{}, info: info}},
		{"changed size", &faultImageFile{Reader: bytes.NewReader(make([]byte, MaxImageBytes+2)), info: info}},
		{"changed size", &faultImageFile{Reader: bytes.NewReader(nil), info: info}},
	} {
		openImageFile = func(*os.Root, string) (imageFile, error) { return tc.file, nil }
		_, err := Load(dir, refs)
		require.ErrorContains(t, err, tc.name)
		require.True(t, tc.file.closed, "owned file closes on every error path")
	}
}
