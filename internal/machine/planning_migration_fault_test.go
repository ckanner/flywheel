package machine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/planning"
	"github.com/stretchr/testify/require"
)

func TestPlanningFenceReportsUnrepresentableFrontmatter(t *testing.T) {
	_, err := planningFence([]byte("---\nvalue: .nan\n---\nPreserve the body.\n"), planning.ActivationRef{})
	require.ErrorContains(t, err, "json: unsupported value", "parseable YAML may contain non-finite numbers")
}

func TestPlanningMigrationFactoryRechecksSourcesAfterValidation(t *testing.T) {
	for _, scenario := range []string{"task-open", "source-lost", "frontmatter-changed"} {
		t.Run(scenario, func(t *testing.T) {
			fx, opts, st, command := migrationFixture(t)
			open := openPlanningTask
			t.Cleanup(func() { openPlanningTask = open })
			openPlanningTask = func(path string) (*os.Root, error) {
				switch scenario {
				case "task-open":
					return nil, os.ErrPermission
				case "source-lost":
					require.NoError(t, os.Remove(filepath.Join(fx.taskDir, "task-packet-M01-T01.md")))
				case "frontmatter-changed":
					writeFile(t, filepath.Join(fx.taskDir, "spec.md"), "external replacement after validation")
				}
				return open(path)
			}
			_, err := newPlanningMigration(opts, st, command)
			require.Error(t, err)
			after := loadFixtureState(t, fx)
			require.Nil(t, after.Planning)
			require.Equal(t, st.Budget, after.Budget)
			require.NoFileExists(t, filepath.Join(fx.taskDir, ".control", "planning", "migration.json"))
		})
	}
}

func TestPlanningMigrationPersistenceFailuresPreserveLegacyAuthority(t *testing.T) {
	for _, scenario := range []string{"oversized-source", "intent-write", "unreadable-intent", "projection-root"} {
		t.Run(scenario, func(t *testing.T) {
			fx, opts, st, command := migrationFixture(t)
			write, open := writePlanningBytes, openPlanningTask
			t.Cleanup(func() { writePlanningBytes, openPlanningTask = write, open })
			switch scenario {
			case "oversized-source":
				// Each individual source remains readable and schema-valid, but
				// preserving original + fenced bytes exceeds the intent bound.
				writeFile(t, filepath.Join(fx.taskDir, "spec.md"), fixtureSpec+"\n"+strings.Repeat("x", 1<<20))
			case "intent-write":
				writePlanningBytes = func(root *os.Root, name string, raw []byte) error {
					if name == "migration.json" {
						return os.ErrPermission
					}
					return write(root, name, raw)
				}
			case "unreadable-intent":
				require.NoError(t, os.MkdirAll(filepath.Join(fx.taskDir, ".control", "planning", "migration.json"), 0700))
			case "projection-root":
				intent, err := newPlanningMigration(opts, st, command)
				require.NoError(t, err)
				raw, err := json.Marshal(intent)
				require.NoError(t, err)
				writeFile(t, filepath.Join(fx.taskDir, ".control", "planning", "migration.json"), string(raw))
				calls := 0
				openPlanningTask = func(path string) (*os.Root, error) {
					calls++
					if calls == 2 { // the readable intent root precedes the projection root
						return nil, os.ErrPermission
					}
					return open(path)
				}
			}
			before := mustReadFile(t, filepath.Join(fx.taskDir, "spec.md"))
			_, _, err := migratePlanning(opts, command)
			require.Error(t, err)
			if scenario == "oversized-source" {
				require.ErrorContains(t, err, "source snapshot exceeds 2 MiB")
			}
			if scenario == "intent-write" || scenario == "projection-root" {
				require.ErrorIs(t, err, os.ErrPermission)
			}
			require.Equal(t, before, mustReadFile(t, filepath.Join(fx.taskDir, "spec.md")), "a failed source transaction cannot publish a partial downgrade fence")
			after := loadFixtureState(t, fx)
			require.Nil(t, after.Planning)
			require.Equal(t, st.Revision, after.Revision)
			require.Equal(t, st.Budget, after.Budget)
		})
	}
}
