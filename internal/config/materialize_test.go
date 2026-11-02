package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func globalMaterializeHooksT(t *testing.T) {
	t.Helper()
	create, link, remove := createGlobalConfigTemp, linkGlobalConfigTemp, removeGlobalConfigTemp
	t.Cleanup(func() {
		createGlobalConfigTemp, linkGlobalConfigTemp, removeGlobalConfigTemp = create, link, remove
	})
}

type materializedResult struct {
	path    string
	created bool
	err     error
}

func materializeResult() materializedResult {
	path, created, err := MaterializeGlobal()
	return materializedResult{path, created, err}
}

func configBytesT(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

func TestMaterializeGlobalConcurrentPublicationHasOneWinner(t *testing.T) {
	globalMaterializeHooksT(t)
	path := filepath.Join(t.TempDir(), "global.json")
	t.Setenv(GlobalConfigEnv, path)
	const count = 16
	ready, release := make(chan struct{}, count), make(chan struct{})
	var once sync.Once
	var workers sync.WaitGroup
	t.Cleanup(func() { once.Do(func() { close(release) }); workers.Wait() })
	linkGlobalConfigTemp = func(source, target string) error {
		ready <- struct{}{}
		<-release
		return os.Link(source, target)
	}
	results := make(chan materializedResult, count)
	for range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- materializeResult()
		}()
	}
	for range count {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("all competing installers should reach publication while the destination is absent")
		}
	}
	assert.NoFileExists(t, path)
	once.Do(func() { close(release) })
	created := 0
	for range count {
		result := <-results
		require.NoError(t, result.err)
		assert.Equal(t, path, result.path)
		if result.created {
			created++
		}
	}
	assert.Equal(t, 1, created)
	expected, err := EmbeddedTable()
	require.NoError(t, err)
	assert.Equal(t, expected, configBytesT(t, path))
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "all losing installers remove only their own temporary file")
	assert.Equal(t, "global.json", entries[0].Name())
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

type gatedGlobalConfigFile struct {
	*os.File
	ready   chan<- string
	release <-chan struct{}
}

func (f gatedGlobalConfigFile) Write(p []byte) (int, error) {
	n, err := f.File.Write(p[:len(p)/2])
	if err != nil {
		return n, err
	}
	f.ready <- f.Name()
	<-f.release
	m, err := f.File.Write(p[n:])
	return n + m, err
}

func TestMaterializeGlobalNeverExposesPartlyWrittenJSON(t *testing.T) {
	globalMaterializeHooksT(t)
	path := filepath.Join(t.TempDir(), "global.json")
	t.Setenv(GlobalConfigEnv, path)
	ready, release := make(chan string, 1), make(chan struct{})
	var once sync.Once
	var worker sync.WaitGroup
	t.Cleanup(func() { once.Do(func() { close(release) }); worker.Wait() })
	createGlobalConfigTemp = func(dir string) (globalConfigTempFile, error) {
		f, err := os.CreateTemp(dir, ".flywheel-config-*.tmp")
		if err != nil {
			return nil, err
		}
		return gatedGlobalConfigFile{f, ready, release}, nil
	}
	result := make(chan materializedResult, 1)
	worker.Add(1)
	go func() { defer worker.Done(); result <- materializeResult() }()
	var temp string
	select {
	case temp = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the temporary file should reach the interrupted write")
	}
	expected, err := EmbeddedTable()
	require.NoError(t, err)
	assert.Equal(t, expected[:len(expected)/2], configBytesT(t, temp))
	_, err = os.Lstat(path)
	assert.ErrorIs(t, err, os.ErrNotExist, "a reader cannot observe the partially written candidate at the global path")
	once.Do(func() { close(release) })
	got := <-result
	require.NoError(t, got.err)
	assert.True(t, got.created)
	assert.Equal(t, expected, configBytesT(t, path))
	assert.NoFileExists(t, temp)
}

func TestMaterializeGlobalPreservesRacingUserFileAndDanglingSymlink(t *testing.T) {
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			globalMaterializeHooksT(t)
			path := filepath.Join(t.TempDir(), "global.json")
			t.Setenv(GlobalConfigEnv, path)
			ready, release := make(chan struct{}, 1), make(chan struct{})
			var once sync.Once
			var worker sync.WaitGroup
			t.Cleanup(func() { once.Do(func() { close(release) }); worker.Wait() })
			linkGlobalConfigTemp = func(source, target string) error {
				ready <- struct{}{}
				<-release
				return os.Link(source, target)
			}
			result := make(chan materializedResult, 1)
			worker.Add(1)
			go func() { defer worker.Done(); result <- materializeResult() }()
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("installer should pause after writing its private candidate")
			}
			user := []byte(`{"version":91,"owner":"user"}`)
			missing := filepath.Join(filepath.Dir(path), "not-created.json")
			if kind == "file" {
				require.NoError(t, os.WriteFile(path, user, 0o600))
			} else {
				require.NoError(t, os.Symlink(missing, path))
			}
			once.Do(func() { close(release) })
			got := <-result
			require.NoError(t, got.err)
			assert.False(t, got.created)
			assert.Equal(t, path, got.path)
			if kind == "file" {
				assert.Equal(t, user, configBytesT(t, path))
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			} else {
				target, err := os.Readlink(path)
				require.NoError(t, err)
				assert.Equal(t, missing, target)
				assert.NoFileExists(t, missing)
			}
			temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".flywheel-config-*.tmp"))
			require.NoError(t, err)
			assert.Empty(t, temps)
		})
	}
}

type failingGlobalConfigFile struct {
	*os.File
	failure string
	err     error
}

func (f failingGlobalConfigFile) Write(p []byte) (int, error) {
	if f.failure == "write" || f.failure == "short-write" {
		n, _ := f.File.Write(p[:1])
		if f.failure == "short-write" {
			return n, nil
		}
		return n, f.err
	}
	return f.File.Write(p)
}

func (f failingGlobalConfigFile) Close() error {
	err := f.File.Close()
	if f.failure == "close" {
		return f.err
	}
	return err
}

func TestMaterializeGlobalFailuresOnlyCleanTheirOwnTemporaryFile(t *testing.T) {
	for _, failure := range []string{"create", "write", "short-write", "close", "link", "cleanup"} {
		t.Run(failure, func(t *testing.T) {
			globalMaterializeHooksT(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "global.json")
			t.Setenv(GlobalConfigEnv, path)
			user := filepath.Join(dir, "user-notes.txt")
			require.NoError(t, os.WriteFile(user, []byte("preserve me"), 0o600))
			boom := errors.New("injected " + failure)
			createGlobalConfigTemp = func(dir string) (globalConfigTempFile, error) {
				if failure == "create" {
					return nil, boom
				}
				f, err := os.CreateTemp(dir, ".flywheel-config-*.tmp")
				if err != nil {
					return nil, err
				}
				return failingGlobalConfigFile{f, failure, boom}, nil
			}
			if failure == "link" {
				linkGlobalConfigTemp = func(string, string) error { return boom }
			}
			if failure == "cleanup" {
				removeGlobalConfigTemp = func(string) error { return boom }
			}
			_, created, err := MaterializeGlobal()
			if failure == "short-write" {
				require.ErrorIs(t, err, io.ErrShortWrite)
			} else {
				require.ErrorIs(t, err, boom)
			}
			assert.Equal(t, []byte("preserve me"), configBytesT(t, user))
			temps, err := filepath.Glob(filepath.Join(dir, ".flywheel-config-*.tmp"))
			require.NoError(t, err)
			if failure == "cleanup" {
				assert.True(t, created, "publication succeeded even though removal of its temporary alias failed")
				expected, err := EmbeddedTable()
				require.NoError(t, err)
				assert.Equal(t, expected, configBytesT(t, path))
				require.Len(t, temps, 1)
			} else {
				assert.False(t, created)
				assert.NoFileExists(t, path)
				assert.Empty(t, temps)
			}
		})
	}
}

func TestMaterializeGlobalAcceptsAnAlreadyRemovedTemporaryAlias(t *testing.T) {
	globalMaterializeHooksT(t)
	path := filepath.Join(t.TempDir(), "global.json")
	t.Setenv(GlobalConfigEnv, path)
	linkGlobalConfigTemp = func(source, target string) error {
		if err := os.Link(source, target); err != nil {
			return err
		}
		return os.Remove(source)
	}
	_, created, err := MaterializeGlobal()
	require.NoError(t, err)
	assert.True(t, created)
	expected, err := EmbeddedTable()
	require.NoError(t, err)
	assert.True(t, bytes.Equal(expected, configBytesT(t, path)))
}
