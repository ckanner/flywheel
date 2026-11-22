package imageinput

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInspectSelectedImagesProducesUsableFrozenReferencesAndSmallPreviews(t *testing.T) {
	dir := t.TempDir()
	paths := []string{}
	for _, format := range []string{"png", "jpeg", "gif", "webp"} {
		name := "selected." + format
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), raster(t, format), 0o600))
		paths = append(paths, name)
	}
	view, err := Inspect(dir, paths)
	require.NoError(t, err)
	require.NoError(t, Validate(&view.References))
	loaded, err := Load(dir, &view.References)
	require.NoError(t, err)
	for i, preview := range view.Previews {
		require.Equal(t, paths[i], preview.Path)
		require.Equal(t, view.References.Images[i].SHA256, preview.SHA256)
		require.Equal(t, len(loaded[i].Data), preview.Bytes)
		require.Equal(t, 2, preview.Width)
		require.Equal(t, 2, preview.Height)
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(preview.DataURL, "data:image/png;base64,"))
		require.NoError(t, err)
		decoded, err := png.Decode(bytes.NewReader(raw))
		require.NoError(t, err)
		require.Equal(t, image.Rect(0, 0, 2, 2), decoded.Bounds())
	}
	wide := image.NewNRGBA(image.Rect(0, 0, 800, 8))
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, wide))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "wide.png"), raw.Bytes(), 0o600))
	view, err = Inspect(dir, []string{"wide.png"})
	require.NoError(t, err)
	preview, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(view.Previews[0].DataURL, "data:image/png;base64,"))
	require.NoError(t, err)
	config, err := png.DecodeConfig(bytes.NewReader(preview))
	require.NoError(t, err)
	require.Equal(t, 160, config.Width)
	require.Equal(t, 1, config.Height)
	encoded, err := json.Marshal(view)
	require.NoError(t, err)
	require.Less(t, len(encoded), 1<<20)
	require.Equal(t, 800, view.Previews[0].Width, "source dimensions do not describe the thumbnail")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "wide.png"), raster(t, "png"), 0o600))
	_, err = Load(dir, &view.References)
	require.ErrorContains(t, err, "changed", "inspection never relaxes later frozen-input verification")
}

func TestInspectRejectsInvalidSelectionsAndPreservesRepositoryBoundary(t *testing.T) {
	dir := t.TempDir()
	for _, paths := range [][]string{nil, {"../outside.png"}, {"a", "a"}, {"a", "b", "c", "d", "e"}} {
		view, err := Inspect(dir, paths)
		require.Error(t, err)
		require.Nil(t, view)
	}
	_, err := Inspect(filepath.Join(dir, "missing-root"), []string{"a.png"})
	require.Error(t, err)
	_, err = Inspect(dir, []string{"missing.png"})
	require.Error(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "not-an-image.png"), []byte("text or SVG is not a decoded raster"), 0o600))
	_, err = Inspect(dir, []string{"not-an-image.png"})
	require.ErrorContains(t, err, "not a decodable image")
	corrupt := raster(t, "png")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corrupt.png"), corrupt[:len(corrupt)-10], 0o600))
	_, err = Inspect(dir, []string{"corrupt.png"})
	require.ErrorContains(t, err, "corrupt")
	out := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(out, "private.png"), raster(t, "png"), 0o600))
	require.NoError(t, os.Symlink(out, filepath.Join(dir, "linked")))
	_, err = Inspect(dir, []string{"linked/private.png"})
	require.ErrorContains(t, err, "symlinks")
	large := append(raster(t, "png"), make([]byte, 3<<20)...)
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), large, 0o600))
	}
	view, err := Inspect(dir, []string{"a.png", "b.png", "c.png"})
	require.ErrorContains(t, err, "combined byte limit")
	require.Nil(t, view, "partial selections are never returned as accepted")
}
