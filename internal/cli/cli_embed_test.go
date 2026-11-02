package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ckanner/flywheel/internal/modules"
)

// ---- 错误注入用最小 fs.FS / fs.File / fs.FileInfo 桩 ----

var errWalkBoom = errors.New("walk boom")

type errOpenFS struct{ openErr error }

func (f errOpenFS) Open(string) (fs.File, error) { return nil, f.openErr }

// failReadFile Open 成功但 Read 报错。
type failReadFile struct{ name string }

func (f *failReadFile) Stat() (fs.FileInfo, error) { return fakeStat{name: f.name}, nil }
func (f *failReadFile) Read([]byte) (int, error)   { return 0, errors.New("read boom") }
func (f *failReadFile) Close() error               { return nil }

func (f *failReadFile) Name() string           { return f.name }
func (f *failReadFile) Size() int64            { return 0 }
func (f *failReadFile) Mode() fs.FileMode      { return 0o444 }
func (f *failReadFile) ModTime() (t time.Time) { return }
func (f *failReadFile) IsDir() bool            { return false }
func (f *failReadFile) Sys() any               { return nil }

// fakeStat 实现 fs.FileInfo（不含 ModTime 的真实时间值）。
type fakeStat struct{ name string }

func (s fakeStat) Name() string       { return s.name }
func (s fakeStat) Size() int64        { return 0 }
func (s fakeStat) Mode() fs.FileMode  { return 0o444 }
func (s fakeStat) ModTime() time.Time { return time.Time{} }
func (s fakeStat) IsDir() bool        { return false }
func (s fakeStat) Sys() any           { return nil }

// readFailFS Open 成功返回 failReadFile。
type readFailFS struct{}

func (readFailFS) Open(name string) (fs.File, error) {
	return &failReadFile{name: name}, nil
}

// ---- 测试 ----

func TestMaterializeFSWalkError(t *testing.T) {
	err := materializeFS(errOpenFS{openErr: errors.New("walk boom")}, ".", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk boom")
}

func TestMaterializeFSReadError(t *testing.T) {
	err := materializeFS(readFailFS{}, ".", filepath.Join(t.TempDir(), "dest"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read boom")
}

// TestInstallEmbeddedSnapshotFSHookError 注入恒错 FS → werr 分支 +
// 结果里该 skill 标记为 error，其余 skill 正常 copied。
func TestInstallEmbeddedSnapshotFSHookError(t *testing.T) {
	orig := skillFSFor
	skillFSFor = func(string) (fs.FS, error) {
		return errOpenFS{openErr: errWalkBoom}, nil
	}
	t.Cleanup(func() { skillFSFor = orig })

	sum, serr := installEmbeddedSnapshot(t.TempDir())
	require.NoError(t, serr)
	var errs, copied int
	for _, r := range sum.Results {
		if r.Action == "error" {
			errs++
			assert.Contains(t, r.Reason, "walk boom")
		} else if r.Action == "copied" {
			copied++
		}
	}
	assert.Equal(t, len(sum.Results), errs+copied)
	assert.Positive(t, errs)
}

// TestInstallEmbeddedSnapshotMkdirAllError project 指向普通文件 →
// MkdirAll ENOTDIR → 顶层错误返回。
func TestInstallEmbeddedSnapshotMkdirAllError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFileCli(t, blocker, "x")
	_, serr := installEmbeddedSnapshot(blocker)
	require.Error(t, serr)
	assert.Contains(t, serr.Error(), "not a directory")
}

// Unmanaged files are user data, even when a previous installer called them stale.
func TestInstallEmbeddedSnapshotPreservesExistingFile(t *testing.T) {
	project := t.TempDir()
	stale := filepath.Join(project, ".agents", "skills", "flywheel-run")
	writeFileCli(t, stale, "stale snapshot")

	sum, serr := installEmbeddedSnapshot(project)
	require.NoError(t, serr)
	for _, r := range sum.Results {
		if r.Name == "flywheel-run" {
			assert.Equal(t, "error", r.Action, r.Reason)
			assert.Contains(t, r.Reason, "refusing to replace")
		}
	}
	raw, err := os.ReadFile(stale)
	require.NoError(t, err)
	assert.Equal(t, "stale snapshot", string(raw))
}

func TestMaterializeFSRealFiles(t *testing.T) {
	mfs := fstest.MapFS{
		"flywheel-run/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: x\n---\n")},
	}
	dest := filepath.Join(t.TempDir(), "out")
	require.NoError(t, materializeFS(mfs, ".", dest))
	assert.FileExists(t, filepath.Join(dest, "flywheel-run", "SKILL.md"))
}

func TestInstallSkillsHomeResolutionError(t *testing.T) {
	t.Setenv("HOME", "")
	code, out := runCLI(t, "install-skills")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "$HOME is not defined")
}

func TestInstallEmbeddedSnapshotRealDirRefusal(t *testing.T) {
	project := t.TempDir()
	realDir := filepath.Join(project, ".agents", "skills", "flywheel-run")
	writeFileCli(t, filepath.Join(realDir, "user-data.txt"), "precious")

	sum, serr := installEmbeddedSnapshot(project)
	require.NoError(t, serr)
	var run modules.SkillInstallResult
	for _, r := range sum.Results {
		if r.Name == "flywheel-run" {
			run = r
		}
	}
	assert.Equal(t, "error", run.Action)
	assert.Contains(t, run.Reason, "refusing to remove")
	data, rerr := os.ReadFile(filepath.Join(realDir, "user-data.txt"))
	require.NoError(t, rerr)
	assert.Equal(t, "precious", string(data))
}

func TestResolveTaskDirEmptyArg(t *testing.T) {
	_, err := resolveTaskDir("")
	assert.Error(t, err)
	wd, _ := os.Getwd()
	got, err := resolveTaskDir(".")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, wd) || strings.HasSuffix(got, filepath.Base(wd)))
}
