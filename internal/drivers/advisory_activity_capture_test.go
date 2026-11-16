package drivers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectActivityCapture(c watchdog.OutputCapture, input []byte, chunkSize int) string {
	var output bytes.Buffer
	for len(input) != 0 {
		n := min(chunkSize, len(input))
		output.Write(c.Filter(input[:n]))
		input = input[n:]
	}
	output.Write(c.Flush())
	return output.String()
}

func TestAdvisoryCaptureKeepsOneObservationAndAllAuthoritativeFrames(t *testing.T) {
	r := codexProRequest(t)
	var source strings.Builder
	progress := dshJSONL(map[string]any{"protocol": r.Protocol, "type": "progress", "requestId": r.RequestID, "text": "正常展示"})
	for sequence := int64(1); sequence <= 200; sequence++ {
		source.WriteString(dshJSONL(activityFixture("codex-pro", r, sequence)))
		if sequence == 100 {
			source.WriteString(progress)
		}
	}
	terminal := dshJSONL(codexProFrame(r, codexProFixture(r)))
	source.WriteString(terminal)
	for _, chunkSize := range []int{1, 137, source.Len()} {
		captured := collectActivityCapture(newProActivityCapture("codex-pro", r), []byte(source.String()), chunkSize)
		assert.Equal(t, progress+dshJSONL(activityFixture("codex-pro", r, 200))+terminal, captured)
		parsed := parseCodexProStream(captured, r)
		require.True(t, parsed.Settled, parsed.Error)
	}
}

func TestAdvisoryCaptureCannotWashOutInvalidOrRepeatedObservations(t *testing.T) {
	r := codexProRequest(t)
	one := dshJSONL(activityFixture("codex-pro", r, 1))
	two := dshJSONL(activityFixture("codex-pro", r, 2))
	foreign := activityFixture("codex-pro", r, 2)
	foreign["requestId"] = "foreign-request"
	terminal := dshJSONL(codexProFrame(r, codexProFixture(r)))
	for name, input := range map[string]string{
		"repeat omitted first sequence": one + one + terminal,
		"earlier sequence after newest": one + two + one + terminal,
		"foreign request":               one + dshJSONL(foreign) + terminal,
		"post terminal":                 one + terminal + two,
		"post terminal progress":        one + terminal + "{\"type\":\"progress\"}\n",
		"malformed line":                one + "{invalid}\n" + terminal,
		"valid JSON without newline":    one + strings.TrimSuffix(terminal, "\n"),
		"partial tail":                  one + terminal + "{\"type\":\"activity\"",
	} {
		t.Run(name, func(t *testing.T) {
			captured := collectActivityCapture(newProActivityCapture("codex-pro", r), []byte(input), 13)
			assert.Contains(t, captured, advisoryCaptureRejected)
			assert.Contains(t, captured, one)
			parsed := parseCodexProStream(captured, r)
			assert.False(t, parsed.Settled)
			assert.False(t, parsed.CleanupConfirmed)
		})
	}
}

func TestAdvisoryCapturePreservesPartialOversizedAndUnclassifiedFrames(t *testing.T) {
	r := codexProRequest(t)
	activity := dshJSONL(activityFixture("codex-pro", r, 1))
	for _, input := range []string{strings.Repeat("x", 100), strings.Repeat("x", 100) + "\n{\"type\":\"result\"}\n"} {
		c := newProActivityCapture("codex-pro", r).(*advisoryActivityCapture)
		c.frameLimit = 50
		captured := collectActivityCapture(c, []byte(input), 30)
		assert.Equal(t, advisoryCaptureRejected+input, captured)
		assert.Empty(t, c.Flush(), "flush is idempotent")
	}
	for _, classify := range []func(json.RawMessage) (bool, error){
		func(json.RawMessage) (bool, error) { return false, nil },
		func(json.RawMessage) (bool, error) { return true, fmt.Errorf("untrusted source") },
	} {
		captured := collectActivityCapture(newAdvisoryActivityCapture(classify), []byte(activity), 17)
		assert.Equal(t, advisoryCaptureRejected+activity, captured)
	}
	wrongClassification := newAdvisoryActivityCapture(func(json.RawMessage) (bool, error) { return true, nil })
	assert.Equal(t, advisoryCaptureRejected+"{\"type\":\"result\"}\n", collectActivityCapture(wrongClassification, []byte("{\"type\":\"result\"}\n"), 17))
	wrongLifecycle := newAdvisoryActivityCapture(func(json.RawMessage) (bool, error) { return true, nil })
	assert.Equal(t, advisoryCaptureRejected+"{\"type\":\"lifecycle\"}\n", collectActivityCapture(wrongLifecycle, []byte("{\"type\":\"lifecycle\"}\n"), 17))
	observedEvent := newAdvisoryActivityCapture(func(raw json.RawMessage) (bool, error) { return bytes.Contains(raw, []byte(`"type":"event"`)), nil })
	assert.Equal(t, "{\"type\":\"event\",\"sequence\":2}\n{\"type\":\"result\"}\n", collectActivityCapture(observedEvent,
		[]byte("{\"type\":\"event\",\"sequence\":1}\n{\"type\":\"event\",\"sequence\":2}\n{\"type\":\"result\"}\n"), 7))
	c := newProActivityCapture("codex-pro", r)
	assert.Equal(t, "\n "+activity+" \t", collectActivityCapture(c, []byte("\n "+activity+" \t"), 29))
	// A classifier sees non-activity frames before advisory data and can bind
	// the fresh SDK session. Alternate terminal types receive the same fence.
	ready := false
	c = newAdvisoryActivityCapture(func(raw json.RawMessage) (bool, error) {
		if bytes.Contains(raw, []byte(`"type":"ready"`)) {
			ready = true
		}
		if bytes.Contains(raw, []byte(`"type":"activity"`)) {
			if !ready {
				return false, fmt.Errorf("missing ready")
			}
			return true, nil
		}
		return false, nil
	}, "warm-result")
	input := "{\"type\":\"ready\"}\n" + activity + "{\"type\":\"warm-result\"}\n" + activity
	assert.Contains(t, collectActivityCapture(c, []byte(input), 11), advisoryCaptureRejected)
	assert.True(t, ready)
}

// Exercise capture through the public Go driver with a real worker process.
// This peer emits valid typed frames; no provider account or model is used.
func TestProActivityCaptureProcessFixture(t *testing.T) {
	harness := os.Getenv("FLYWHEEL_TEST_PRO_CAPTURE_CHILD")
	if harness == "" {
		return
	}
	var request codexProStart
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(91)
	}
	var written int64
	for sequence := int64(1); written <= watchdog.MaxBuffer+1024*1024; sequence++ {
		frame, _ := json.Marshal(activityFixture(harness, request, sequence))
		n, err := os.Stdout.Write(append(frame, '\n'))
		if err != nil {
			os.Exit(92)
		}
		written += int64(n)
	}
	var result map[string]any
	if harness == "codex-pro" {
		result = codexProFrame(request, codexProFixture(request))
	} else {
		claude := claudeCodeProStart(request)
		result = claudeCodeProFrame(claude, claudeCodeProFixture(claude))
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
	os.Exit(0)
}

func TestPublicProDriversRetainFinalReceiptBeyondRawActivityCaptureLimit(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		t.Run(harness, func(t *testing.T) {
			command := kimiScript(t, "FLYWHEEL_TEST_PRO_CAPTURE_CHILD="+harness+" exec "+shQuote(os.Args[0])+" -test.run '^TestProActivityCaptureProcessFixture$'\n")
			var observedBytes int
			onEvent := func(line string) { observedBytes += len(line) + 1 }
			limits := watchdog.Opts{IdleTimeoutMs: 5000, WallTimeoutMs: 20000}
			var result Result
			if harness == "codex-pro" {
				result = RunCodexPro(CodexProArgs{Command: command, Cwd: t.TempDir(), RequestID: "capture-codex", Prompt: "offline capture",
					Model: "gpt-test", PermissionMode: "read-only", OnEvent: onEvent, Watchdog: limits})
			} else {
				result = RunClaudeCodePro(ClaudeCodeProArgs{Command: command, Cwd: t.TempDir(), RequestID: "capture-claude", Prompt: "offline capture",
					Model: "claude-test", ReasoningEffort: "high", PermissionMode: "danger-full-access", OnEvent: onEvent, Watchdog: limits})
			}
			require.True(t, result.Settled, "%+v", result)
			assert.False(t, result.Truncated)
			assert.Greater(t, observedBytes, int(watchdog.MaxBuffer), "raw OnEvent keeps every observed frame")
			assert.Less(t, len(result.Stdout), 16*1024, "capture holds one diagnostic observation plus the complete receipt")
			assert.Equal(t, 1, strings.Count(result.Stdout, `"type":"activity"`))
		})
	}
}
