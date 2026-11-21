package state

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/stretchr/testify/require"
)

func TestSessionImageManifestRoundTripAndBounds(t *testing.T) {
	dir := t.TempDir()
	st := sessionTestState(t, dir)
	manifest := imageinput.Describe([]imageinput.Image{{MIMEType: "image/png", Data: []byte("original-image-bytes")}})
	cfg := &SessionConfiguration{Harness: "claude-code-pro", Model: "sonnet", Channel: "claude-native", Thinking: "high", Permission: "danger-full-access", SessionMode: "scoped", InputImages: manifest}
	st.Sessions.Records["execute/M01-T01"].Configuration = cfg
	_, err := Save(dir, st, nil)
	require.NoError(t, err)
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, manifest, loaded.Sessions.Records["execute/M01-T01"].Configuration.InputImages)
	raw, err := json.Marshal(loaded.Sessions)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "original-image-bytes")
	for _, invalid := range []*imageinput.Manifest{
		{Version: 2, Images: manifest.Images},
		{Version: 1, Images: []imageinput.ImageInfo{{MIMEType: "image/png", SHA256: strings.Repeat("a", 64), Bytes: imageinput.MaxImageBytes + 1}}},
		{Version: 1, Images: []imageinput.ImageInfo{
			{MIMEType: "image/png", SHA256: strings.Repeat("a", 64), Bytes: imageinput.MaxImageBytes},
			{MIMEType: "image/png", SHA256: strings.Repeat("b", 64), Bytes: imageinput.MaxImageBytes},
			{MIMEType: "image/png", SHA256: strings.Repeat("c", 64), Bytes: 1},
		}},
	} {
		cfg.InputImages = invalid
		raw, err := json.Marshal(st.Sessions)
		require.NoError(t, err)
		require.Error(t, validateSessions(raw))
		_, err = Save(dir, st, nil)
		require.Error(t, err)
	}
	cfg.InputImages, cfg.Harness = manifest, "codex-pro"
	_, err = Save(dir, st, nil)
	require.Error(t, err, "image metadata cannot claim support in another harness")
}
