package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEditorScopesValidationAndConflicts(t *testing.T) {
	root := t.TempDir()
	t.Setenv(GlobalConfigEnv, filepath.Join(root, "global.json"))
	doc, err := ReadEditor(root, "global")
	require.NoError(t, err)
	require.NotContains(t, doc.Values, "intake")
	req := EditorRequest{Scope: "global", Revision: doc.Revision, Values: doc.Values}
	saved, err := SaveEditor(root, req)
	require.NoError(t, err)
	require.True(t, saved.Applied)
	_, err = SaveEditor(root, req)
	require.ErrorContains(t, err, "changed")
	loaded, err := LoadReadOnly("", root)
	require.NoError(t, err)
	require.Equal(t, "scoped", loaded.Sessions.Mode)
	doc, err = ReadEditor(root, "global")
	require.NoError(t, err)
	_, err = SaveEditor(root, EditorRequest{Scope: "global", Revision: doc.Revision, Values: map[string]json.RawMessage{"intake": json.RawMessage(`{}`)}})
	require.ErrorContains(t, err, "section")
	_, err = SaveEditor(root, EditorRequest{Scope: "global", Revision: doc.Revision, Values: map[string]json.RawMessage{"sessions": json.RawMessage(`{"mode":"wrong","policy-version":1}`)}})
	require.Error(t, err)
	req = EditorRequest{Scope: "global", Revision: doc.Revision, Reset: true}
	saved, err = SaveEditor(root, req)
	require.NoError(t, err)
	require.True(t, saved.Applied)
	project, err := ReadEditor(root, "project")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "flywheel.config.json"), project.Target)
	_, err = SaveEditor(root, EditorRequest{Scope: "project", Revision: project.Revision, Values: project.Values})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(project.Target, []byte(`{"sessions":null}`), 0600))
	_, err = ReadEditor(root, "project")
	require.Error(t, err)
	_, err = ReadEditor(root, "task")
	require.Error(t, err)
	_, err = SaveEditor(root, EditorRequest{Scope: "task"})
	require.Error(t, err)
	require.NoError(t, os.Remove(project.Target))
	require.NoError(t, os.Mkdir(project.Target, 0700))
	_, err = ReadEditor(root, "project")
	require.Error(t, err)
	t.Setenv(GlobalConfigEnv, "")
	t.Setenv("HOME", "")
	_, err = ReadEditor(root, "global")
	require.Error(t, err)
}

func TestEditorIOFailuresAndPublicationReceipt(t *testing.T) {
	root := t.TempDir()
	t.Setenv(GlobalConfigEnv, filepath.Join(root, "global.json"))
	originalAbs, originalEmbed, originalRename, originalRead := absMigrationPath, embeddedTable, renameMigrationFile, readMigrationFile
	t.Cleanup(func() {
		absMigrationPath = originalAbs
		embeddedTable = originalEmbed
		renameMigrationFile = originalRename
		readMigrationFile = originalRead
	})
	fail := errors.New("injected")
	absMigrationPath = func(string) (string, error) { return "", fail }
	_, err := ReadEditor(root, "global")
	require.ErrorIs(t, err, fail)
	absMigrationPath = originalAbs
	embeddedTable = func() ([]byte, error) { return nil, fail }
	_, err = ReadEditor(root, "global")
	require.ErrorIs(t, err, fail)
	embeddedTable = func() ([]byte, error) { return []byte(`null`), nil }
	_, err = ReadEditor(root, "global")
	require.Error(t, err)
	embeddedTable = originalEmbed
	doc, err := ReadEditor(root, "global")
	require.NoError(t, err)
	calls := 0
	embeddedTable = func() ([]byte, error) {
		calls++
		if calls == 2 {
			return nil, fail
		}
		return originalEmbed()
	}
	_, err = SaveEditor(root, EditorRequest{Scope: "global", Revision: doc.Revision})
	require.ErrorIs(t, err, fail)
	embeddedTable = originalEmbed
	saved, err := SaveEditor(root, EditorRequest{Scope: "global", Revision: doc.Revision, Values: doc.Values})
	require.NoError(t, err)
	renameMigrationFile = func(string, string) error { return fail }
	_, err = SaveEditor(root, EditorRequest{Scope: "global", Revision: saved.Revision})
	require.ErrorIs(t, err, fail)
	renameMigrationFile = originalRename
	calls = 0
	readMigrationFile = func(p string) ([]byte, error) {
		calls++
		if calls == 4 {
			return nil, fail
		}
		return originalRead(p)
	}
	result, err := SaveEditor(root, EditorRequest{Scope: "global", Revision: saved.Revision})
	require.ErrorIs(t, err, fail)
	require.True(t, result.Applied)
}
