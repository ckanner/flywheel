package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const feedbackOutputLimit = 2000

// gateCheckID identifies one ordered command in a gate. It is not a test-case
// identifier: shell output alone cannot prove which tests were discovered.
// Position distinguishes repeated commands; changing a command or its expected
// exit status invalidates comparisons with its previous outcome.
func gateCheckID(index int, command, expect string) string {
	return "check-" + hashFeedbackParts("check-v1", strconv.Itoa(index), command, expect)
}

// gateEnvironmentID binds the exact filtered environment variables supplied to
// every command in this gate, without persisting their values. It does not claim
// an identity for external services, ignored dependencies or the toolchain.
func gateEnvironmentID(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{"environment-v1"}
	for _, key := range keys {
		parts = append(parts, key, env[key])
	}
	return hashFeedbackParts(parts...)
}

// gateOutputDigest identifies complete captured streams before normalization or
// summary clipping. Stream boundaries are included; a digest is diagnostic
// evidence, not a fabricated reference to a persisted full log.
func gateOutputDigest(stdout, stderr string) string {
	return hashFeedbackParts("output-v1", stdout, stderr)
}

// Length framing keeps arbitrary raw output bytes and stream boundaries exact;
// JSON string encoding would replace invalid UTF-8 before computing the digest.
func hashFeedbackParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(h, "%d:", len(part))
		_, _ = h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// summarizeOutput retains setup context and the terminal failure within a
// bounded diagnostic payload. Summary clipping is separate from watchdog output
// loss: a compact summary cannot make a complete command outcome incomplete or
// turn an interrupted command into a successful gate.
func summarizeOutput(output string) (summary string, truncated bool) {
	output = strings.ToValidUTF8(output, "\uFFFD")
	if len(output) <= feedbackOutputLimit {
		return output, false
	}
	const marker = "\n... [output summary truncated] ...\n"
	remaining := feedbackOutputLimit - len(marker)
	headEnd := remaining / 2
	for !utf8.RuneStart(output[headEnd]) {
		headEnd--
	}
	tailStart := len(output) - (remaining - headEnd)
	for !utf8.RuneStart(output[tailStart]) {
		tailStart++
	}
	return output[:headEnd] + marker + output[tailStart:], true
}
