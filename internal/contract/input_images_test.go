package contract

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacketInputImagesAreTypedOptionalAndStrict(t *testing.T) {
	base, err := ParsePacket([]byte(validPacket), "packet.md")
	require.NoError(t, err)
	require.Nil(t, base.InputImages)
	digest := strings.Repeat("a", 64)
	declaration := fmt.Sprintf(`{version: 1, images: [{path: references/layout.png, sha256: %s, media-type: image/png}]}`, digest)
	parse := func(value string) (*Packet, error) {
		return ParsePacket([]byte(strings.Replace(validPacket, "packet-id:", "input-images: "+value+"\npacket-id:", 1)), "packet.md")
	}
	got, err := parse(declaration)
	require.NoError(t, err)
	require.Equal(t, 1, got.InputImages.Version)
	require.Equal(t, "references/layout.png", got.InputImages.Images[0].Path)
	require.Equal(t, digest, got.InputImages.Images[0].SHA256)
	require.Equal(t, "image/png", got.InputImages.Images[0].MediaType)
	for _, invalid := range []string{
		"null", "{}", "{version: 2, images: []}", "{version: 1, images: []}",
		strings.Replace(declaration, "image/png", "text/plain", 1),
		strings.Replace(declaration, digest, strings.ToUpper(digest), 1),
		strings.Replace(declaration, "references/layout.png", "../secret.png", 1),
		strings.Replace(declaration, "references/layout.png", "..", 1),
		strings.Replace(declaration, "references/layout.png", "references/../layout.png", 1),
		strings.Replace(declaration, "references/layout.png", ".git/private.png", 1),
		strings.Replace(declaration, "version: 1", "version: 1, extra: true", 1),
		strings.Replace(declaration, "path:", "data: aGVsbG8=, path:", 1),
		fmt.Sprintf(`{version: 1, images: [{path: reference.png, sha256: %s, media-type: image/png}, {path: reference.png, sha256: %s, media-type: image/jpeg}]}`, digest, strings.Repeat("b", 64)),
	} {
		_, err := parse(invalid)
		require.Error(t, err, invalid)
	}
}
