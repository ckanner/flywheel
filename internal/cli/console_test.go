package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/stretchr/testify/require"
)

type failingEditorReader struct{}

func (failingEditorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestControlEditorUsesBoundedStdinAndExplicitScope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FLYWHEEL_CONFIG", filepath.Join(root, "global.json"))
	call := func(action string, input io.Reader) (string, error) {
		cmd := controlCmd()
		cmd.SetArgs([]string{action, "--cwd", root})
		cmd.SetIn(input)
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := cmd.Execute()
		return out.String(), err
	}
	raw, err := call("config-edit", strings.NewReader(`{"scope":"global"}`))
	require.NoError(t, err)
	require.Contains(t, raw, `"editor"`)
	for _, reader := range []io.Reader{failingEditorReader{}, strings.NewReader(strings.Repeat("x", (1<<20)+1)), strings.NewReader(`{"scope":"global","unexpected":true}`), strings.NewReader(`{} {}`)} {
		_, err = call("config-edit", reader)
		require.Error(t, err)
	}
	// Exercise the write entry through its ordinary JSON command boundary.
	raw, err = call("config-save", strings.NewReader(`{"scope":"global","revision":"stale"}`))
	require.Error(t, err)
	require.Contains(t, raw, "changed")
}

func TestControlStepValidatesAndForwardsArguments(t *testing.T) {
	root, task := t.TempDir(), t.TempDir()
	for _, args := range [][]string{{"--only", "execute"}, {"--only", "wrong", "--expect-revision", "1"}} {
		code, _ := runControlTest(t, "step", task, root, args...)
		require.Equal(t, 1, code)
	}
	code, _ := runControlTest(t, "status", task, root, "--retry-reason", "reason")
	require.Equal(t, 1, code)
	code, _ = runControlTest(t, "status", task, root, "--only", "execute")
	require.Equal(t, 1, code)
	release, err := machine.AcquireRunLease(task)
	require.NoError(t, err)
	code, _ = runControlTest(t, "step", task, root, "--only", "execute", "--packet", "M01-T01", "--expect-revision", "1")
	require.Equal(t, 1, code)
	release()
	controlFixture(t, "completed")
	code, _ = runControlTest(t, "step", task, root, "--only", "execute", "--packet", "M01-T01", "--expect-revision", "1", "--retry-reason", "fixture retry")
	require.Zero(t, code)
	raw, err := os.ReadFile(filepath.Join(task, "fixture-args.json"))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"--expect-revision","1"`)
	code, _ = runControlTest(t, "step", task, root, "--only", "plan", "--expect-revision", "1")
	require.Zero(t, code)
	for _, args := range [][]string{{"run", task, "--retry-reason", "reason"}, {"run", task, "--expect-revision", "1"}, {"run", task, "--only", "execute", "--expect-revision", "0"}, {"run", task, "--only", "execute", "--expect-revision", "1"}} {
		code, _ := runCLI(t, args...)
		require.Equal(t, 1, code)
	}
}

func TestControlStepCannotAcknowledgeAnotherRunOwner(t *testing.T) {
	controlFixture(t, "other-owner")
	code, reply := runControlTest(t, "step", t.TempDir(), t.TempDir(), "--only", "execute", "--packet", "M01-T01", "--expect-revision", "1")
	require.Equal(t, 1, code)
	require.False(t, reply.OK)
}
