package drivers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/require"
)

func TestLegacyCodexSilenceUsesWallBudgetAndPreservesCancellation(t *testing.T) {
	disableCodexDesktop(t)
	bin, cwd := fakeBin(t, "codex", codexNDJSON)
	setPath(t, bin)
	command := filepath.Join(bin, "codex")
	raw, err := os.ReadFile(command)
	require.NoError(t, err)
	// codex exec JSONL cannot expose continuous thinking. A valid eventual
	// result after a quiet interval longer than idle must remain deliverable.
	require.NoError(t, os.WriteFile(command, append([]byte("#!/bin/sh\nsleep 0.12\n"), raw...), 0700))
	a := CodexArgs{Cwd: cwd, Prompt: "quiet reasoning", Watchdog: watchdog.Opts{IdleTimeoutMs: 20, WallTimeoutMs: 2000, KillGraceMs: 20}}
	result := RunCodex(a)
	require.True(t, result.Settled, "%+v", result)
	require.False(t, result.IdleTimedOut)
	a.Watchdog.WallTimeoutMs = 25
	result = RunCodex(a)
	require.True(t, result.WallTimedOut)
	require.False(t, result.Cancelled)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Watchdog.Context = ctx
	result = RunCodex(a)
	require.True(t, result.Cancelled)
	require.False(t, result.TimedOut)
}
