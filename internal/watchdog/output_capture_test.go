package watchdog

import (
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type discardOutputCapture struct {
	bytes   int
	flushes int
	tail    []byte
}

func (c *discardOutputCapture) Filter(chunk []byte) []byte {
	c.bytes += len(chunk)
	return nil
}

func (c *discardOutputCapture) Flush() []byte {
	c.flushes++
	return c.tail
}

func TestOutputCaptureDoesNotChangeRawObservationIdleOrStderr(t *testing.T) {
	capture := &discardOutputCapture{tail: []byte("bounded summary\n")}
	var mu sync.Mutex
	observed := map[string]string{}
	out := Run("sh", []string{"-c", `i=0; while [ $i -lt 10 ]; do echo real-observation; sleep 0.06; i=$((i+1)); done; echo unchanged-diagnostic >&2`}, Opts{
		IdleTimeoutMs: 250, WallTimeoutMs: 5000, KillGraceMs: 500, StdoutCapture: capture,
		OnActivity: func(chunk, stream string) {
			mu.Lock()
			observed[stream] += chunk
			mu.Unlock()
		},
	})
	require.NotNil(t, out.ExitCode)
	assert.Zero(t, *out.ExitCode)
	assert.False(t, out.TimedOut, "uncaptured real output still refreshes idle")
	assert.False(t, out.Truncated)
	assert.Equal(t, "bounded summary\n", out.Stdout)
	assert.Equal(t, "unchanged-diagnostic\n", out.Stderr)
	assert.Equal(t, 10, strings.Count(observed["stdout"], "real-observation"))
	assert.Equal(t, out.Stderr, observed["stderr"])
	assert.Equal(t, len(observed["stdout"]), capture.bytes)
	assert.Equal(t, 1, capture.flushes)
}

func TestOutputCaptureFiltersBeforeRetainedLimitAndFlushStillObeysIt(t *testing.T) {
	previous := MaxBuffer
	MaxBuffer = 64
	t.Cleanup(func() { MaxBuffer = previous })
	for _, tail := range []string{"", "receipt", strings.Repeat("x", 65)} {
		t.Run("tail-bytes-"+strconv.Itoa(len(tail)), func(t *testing.T) {
			capture := &discardOutputCapture{tail: []byte(tail)}
			out := Run("sh", []string{"-c", "head -c 32768 /dev/zero"}, Opts{
				IdleTimeoutMs: 5000, WallTimeoutMs: 10000, StdoutCapture: capture,
			})
			require.NotNil(t, out.ExitCode)
			assert.Zero(t, *out.ExitCode)
			assert.Equal(t, 32768, capture.bytes)
			assert.Equal(t, 1, capture.flushes)
			assert.Equal(t, len(tail) > int(MaxBuffer), out.Truncated)
			assert.Equal(t, tail[:min(len(tail), int(MaxBuffer))], out.Stdout)
		})
	}
}
