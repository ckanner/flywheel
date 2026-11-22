package prompts

import (
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/stretchr/testify/require"
)

func TestImagePromptsNameFrozenOrderAndIndependentReviewLimits(t *testing.T) {
	p := testPacket()
	p.InputImages = &imageinput.References{Version: 1, Images: []imageinput.Reference{
		{Path: "references/mobile.png", SHA256: strings.Repeat("a", 64), MediaType: "image/png"},
		{Path: "references/desktop.webp", SHA256: strings.Repeat("b", 64), MediaType: "image/webp"},
	}}
	execute := BuildExecutePrompt(p, testSpec(), "/task", "", 1)
	require.Contains(t, execute, "Native image content blocks are supplied in the order below")
	require.Less(t, strings.Index(execute, "references/mobile.png"), strings.Index(execute, "references/desktop.webp"))
	require.Contains(t, execute, strings.Repeat("a", 64))
	review := BuildReviewPrompt(ReviewArgs{Packet: p})
	require.Contains(t, review, "not native image content blocks")
	require.Contains(t, review, "Do not claim to have visually inspected an attachment")
	require.Contains(t, review, "verification-gap")
	require.Contains(t, review, "references/desktop.webp")
}
