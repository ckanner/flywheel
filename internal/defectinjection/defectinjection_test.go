package defectinjection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePatchFiles(t *testing.T) {
	patch := `diff --git a/src/new.ts b/src/new.ts
new file mode 100644
--- /dev/null
+++ b/src/new.ts
@@ -0,0 +1 @@
+ok
diff --git a/src/old.ts b/src/old.ts
--- a/src/old.ts
+++ b/src/old.ts
@@ -1 +1 @@
-x
+y
diff --git a/src/gone.ts b/src/gone.ts
deleted file mode 100644
--- a/src/gone.ts
+++ /dev/null
@@ -1 +0,0 @@
-bye
`
	created, modified := ParsePatchFiles(patch)
	assert.Equal(t, []string{"src/new.ts"}, created)
	assert.Equal(t, []string{"src/old.ts", "src/gone.ts"}, modified)

	// 重复条目去重。
	dup := "--- /dev/null\n+++ a/dup.txt\n@@\n+1\n--- /dev/null\n+++ a/dup.txt\n@@\n+2\n"
	c2, m2 := ParsePatchFiles(dup)
	// 裸路径（无 a/ b/ 前缀）原样保留。
	c3, m3 := ParsePatchFiles("--- /dev/null\n+++ bare.txt\n@@\n+1\n--- old2.txt\n+++ /dev/null\n@@\n-x\n")
	assert.Equal(t, []string{"bare.txt"}, c3)
	assert.Equal(t, []string{"old2.txt"}, m3)
	assert.Equal(t, []string{"dup.txt"}, c2)
	assert.Empty(t, m2)
}

func TestShouldSample(t *testing.T) {
	tests := []struct {
		name       string
		completed  int
		everyN     int
		boost      float64
		ratio      float64
		wantSample bool
	}{
		{"every unit when N=1 even first", 0, 1, 0.5, 0, true},
		{"N=3 second unit not sampled", 1, 3, 0.5, 0, false},
		{"N=3 third unit sampled", 2, 3, 0.5, 0, true},
		{"test-heavy diff boosts from the second completed unit on", 1, 5, 0.5, 0.6, true},
		{"boost does not fire before any completion", 0, 5, 0.5, 1.0, false},
		{"ratio below boost falls through to modulo", 4, 5, 0.5, 0.2, true}, // (4+1)%5==0
		{"nothing hits", 1, 5, 0.5, 0.4, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldSample(tc.completed, tc.everyN, tc.boost, tc.ratio)
			assert.Equal(t, tc.wantSample, got)
		})
	}
}

// writePatch 落一个可逆补丁：新建 poison 文件。
func writePatch(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "poison.patch")
	require.NoError(t, os.WriteFile(p, []byte("--- /dev/null\n+++ b/poison.txt\n@@ -0,0 +1 @@\n+injected defect\n"), 0o644))
	return p
}

func TestRunDefectInjectionPass(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	outcome := RunDefectInjection(Args{
		PatchFile: writePatch(t, dir),
		TestCmd:   "suite",
		Cwd:       dir,
		Run: func(cmd string, args []string, cwd string, timeoutMs int64, envAllowlist []string) RunResult {
			calls = append(calls, cmd+" "+strings.Join(args, " "))
			if cmd == "git" {
				return RunResult{ExitCode: 0} // apply 与 apply -R 都成功
			}
			if len(calls) == 3 { // 第三次 spawn = 注入后的测试，变红
				return RunResult{ExitCode: 1}
			}
			return RunResult{ExitCode: 0}
		},
	})
	assert.Equal(t, "pass", outcome.Outcome)
	assert.True(t, outcome.Pass)
	assert.True(t, outcome.Injected)
	assert.True(t, outcome.TestsWentRed)
	assert.True(t, outcome.Restored)
	require.NotNil(t, outcome.BaselineGreen)
	assert.True(t, *outcome.BaselineGreen)
	assert.Nil(t, outcome.Detail)
	// 顺序：基线 → git apply → 测试 → git apply -R。
	assert.Len(t, calls, 4)
	assert.Contains(t, calls[0], "/bin/sh")
	assert.Contains(t, calls[1], "apply "+filepath.Join(dir, "poison.patch"))
	assert.Contains(t, calls[3], "apply -R")
}

func TestRunDefectInjectionFakeTests(t *testing.T) {
	dir := t.TempDir()
	outcome := RunDefectInjection(Args{
		PatchFile: writePatch(t, dir),
		TestCmd:   "suite",
		Cwd:       dir,
		Run: func(cmd string, _ []string, _ string, _ int64, _ []string) RunResult {
			return RunResult{ExitCode: 0} // 测试始终绿 → 假测试
		},
	})
	assert.Equal(t, "fake-tests", outcome.Outcome)
	assert.False(t, outcome.Pass)
	assert.True(t, outcome.Injected)
	assert.True(t, outcome.TestsWentRed == false)
	assert.True(t, outcome.Restored)
	require.NotNil(t, outcome.Detail)
	assert.Contains(t, *outcome.Detail, "stayed green")
}

func TestRunDefectInjectionRestoreFailed(t *testing.T) {
	dir := t.TempDir()
	outcome := RunDefectInjection(Args{
		PatchFile: writePatch(t, dir),
		TestCmd:   "suite",
		Cwd:       dir,
		Run: func(cmd string, args []string, _ string, _ int64, _ []string) RunResult {
			if cmd == "git" && len(args) > 1 && args[1] == "-R" {
				return RunResult{ExitCode: 128, Stderr: "  patch does not reverse  "}
			}
			return RunResult{ExitCode: 0}
		},
	})
	assert.Equal(t, "restore-failed", outcome.Outcome)
	assert.False(t, outcome.Pass)
	assert.True(t, outcome.Injected)
	assert.False(t, outcome.Restored)
	require.NotNil(t, outcome.Detail)
	assert.Equal(t, "git apply -R failed: patch does not reverse", *outcome.Detail)
}

func TestRunDefectInjectionUninjectablePaths(t *testing.T) {
	t.Run("patch unreadable", func(t *testing.T) {
		o := RunDefectInjection(Args{PatchFile: filepath.Join(t.TempDir(), "nope.patch"), Cwd: "."})
		assert.Equal(t, "uninjectable", o.Outcome)
		assert.Nil(t, o.BaselineGreen)
	})

	t.Run("forbidden ledger path refused before any spawn", func(t *testing.T) {
		dir := t.TempDir()
		spawned := false
		o := RunDefectInjection(Args{
			PatchFile:   writePatch(t, dir), // 触碰 poison.txt——在禁止清单里
			Cwd:         dir,
			ForbidPaths: []string{"poison.txt"},
			Run: func(string, []string, string, int64, []string) RunResult {
				spawned = true
				return RunResult{ExitCode: 0}
			},
		})
		assert.Equal(t, "uninjectable", o.Outcome)
		assert.False(t, spawned)
		require.NotNil(t, o.Detail)
		assert.Contains(t, *o.Detail, "poison.txt")
	})

	t.Run("ledger prefix single-source matches directory contents (v0.19)", func(t *testing.T) {
		dir := t.TempDir()
		patch := filepath.Join(dir, "p.patch")
		require.NoError(t, os.WriteFile(patch, []byte("--- /dev/null\n+++ b/docs/task/reports/y.done.json\n@@ -0,0 +1 @@\n+j\n"), 0o644))
		spawned := false
		o := RunDefectInjection(Args{
			PatchFile:        patch,
			Cwd:              dir,
			LedgerTaskPrefix: "docs/task/",
			Run: func(string, []string, string, int64, []string) RunResult {
				spawned = true
				return RunResult{ExitCode: 0}
			},
		})
		assert.Equal(t, "uninjectable", o.Outcome)
		assert.False(t, spawned)
	})
	t.Run("directory prefix entry catches files inside (C3 回归)", func(t *testing.T) {
		dir := t.TempDir()
		// 补丁触碰 docs/task/reports/x.done.json——账本目录条目 "reports/"
		// 必须按前缀命中（原精确相等实现会放行）。
		patch := filepath.Join(dir, "ledger.patch")
		require.NoError(t, os.WriteFile(patch, []byte("--- /dev/null\n+++ b/docs/task/reports/x.done.json\n@@ -0,0 +1 @@\n+j\n"), 0o644))
		spawned := false
		o := RunDefectInjection(Args{
			PatchFile: patch,
			Cwd:       dir,
			ForbidPaths: []string{
				"docs/task/state.json",
				"docs/task/reports/",
			},
			Run: func(string, []string, string, int64, []string) RunResult {
				spawned = true
				return RunResult{ExitCode: 0}
			},
		})
		assert.Equal(t, "uninjectable", o.Outcome)
		assert.False(t, spawned)
	})

	t.Run("git apply fails -> stale patch, baseline recorded green", func(t *testing.T) {
		dir := t.TempDir()
		o := RunDefectInjection(Args{
			PatchFile: writePatch(t, dir),
			TestCmd:   "suite",
			Cwd:       dir,
			Run: func(cmd string, _ []string, _ string, _ int64, _ []string) RunResult {
				if cmd == "git" {
					return RunResult{ExitCode: 1, Stderr: "error: corrupt patch\n"}
				}
				return RunResult{ExitCode: 0} // 基线绿
			},
		})
		assert.Equal(t, "uninjectable", o.Outcome)
		require.NotNil(t, o.BaselineGreen)
		assert.True(t, *o.BaselineGreen)
		require.NotNil(t, o.Detail)
		assert.Equal(t, "git apply failed: error: corrupt patch", *o.Detail)
	})
}

func TestRunDefectInjectionBaselineRed(t *testing.T) {
	dir := t.TempDir()
	spawnCount := 0
	o := RunDefectInjection(Args{
		PatchFile: writePatch(t, dir),
		TestCmd:   "suite",
		Cwd:       dir,
		Run: func(string, []string, string, int64, []string) RunResult {
			spawnCount++
			return RunResult{ExitCode: 1} // 基线就红
		},
	})
	assert.Equal(t, "baseline-red", o.Outcome)
	assert.False(t, o.Pass)
	assert.False(t, o.Injected)
	assert.Nil(t, o.BaselineGreen)
	assert.Equal(t, 1, spawnCount) // 基线失败后绝不注入
}

func TestRunDefectInjectionRelativePatchPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "poison.patch"), []byte("--- /dev/null\n+++ b/poison.txt\n@@ -0,0 +1 @@\n+x\n"), 0o644))
	shCalls := 0
	o := RunDefectInjection(Args{
		PatchFile: "poison.patch", // 相对路径 → 相对 cwd 解析
		TestCmd:   "suite",
		Cwd:       dir,
		Run: func(cmd string, _ []string, _ string, _ int64, _ []string) RunResult {
			if cmd == "git" {
				return RunResult{ExitCode: 0}
			}
			shCalls++
			if shCalls == 1 {
				return RunResult{ExitCode: 0} // 基线绿
			}
			return RunResult{ExitCode: 1} // 注入后红
		},
	})
	assert.Equal(t, "pass", o.Outcome)
}

func TestDefaultRunCapturesOutputAndExitCode(t *testing.T) {
	r := DefaultRun("/bin/sh", []string{"-c", "echo out; echo err >&2; exit 7"}, ".", 10_000, nil)
	assert.Equal(t, 7, r.ExitCode)
	assert.Equal(t, "out\n", r.Stdout)
	assert.Equal(t, "err\n", r.Stderr)
}

func TestDefaultRunTimeoutHasNoNormalExit(t *testing.T) {
	r := DefaultRun("/bin/sh", []string{"-c", "sleep 5"}, ".", 80, nil)
	assert.Equal(t, -1, r.ExitCode)
	assert.True(t, r.TimedOut)
}

func TestDefaultRunSpawnErrorHasNoNormalExit(t *testing.T) {
	r := DefaultRun("/definitely/not/a/binary", nil, ".", 1000, nil)
	assert.Equal(t, -1, r.ExitCode)
	assert.NotEmpty(t, r.SpawnError)
}

func TestDefaultRunEnvAllowlist(t *testing.T) {
	t.Setenv("FLYWHEEL_DI_TEST_KEY", "v")
	r := DefaultRun("/bin/sh", []string{"-c", "echo ${FLYWHEEL_DI_TEST_KEY:-missing}"}, ".", 5000, []string{"FLYWHEEL_DI_TEST_KEY"})
	assert.Equal(t, "v", strings.TrimSpace(r.Stdout))
}

func TestDefaultRunMaxBufferOverflow(t *testing.T) {
	old := maxBuffer
	maxBuffer = 8
	defer func() { maxBuffer = old }()
	r := DefaultRun("/bin/sh", []string{"-c", "printf 'aaaaaaaaaaaaaaaa'; printf 'bbbbbbbbbbbbbbbb' >&2"}, ".", 5000, nil)
	assert.Equal(t, 0, r.ExitCode, "保留真实退出码，截断另外记录")
	assert.True(t, r.Truncated)
	assert.Equal(t, "aaaaaaaa", r.Stdout)
	assert.Equal(t, "bbbbbbbb", r.Stderr)
}
