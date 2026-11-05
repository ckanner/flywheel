package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const legacyMigrationConfig = `{
  "version": 1,
  "_note_user": "Preserve my setup and acceptance policy",
  "loops": {
    "_note_team": "Keep this explanation",
    "plan": {"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription","thinking":"max","resident-session":false,"_note_plan":"Keep short sessions"},
    "review": {"profile":"old-review","thinking":"max"},
    "judge": {"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription","thinking":"max"},
    "intake": {"harness":"zcode","model":"zai/glm-5.3-flash","channel":"zai-coding-plan","thinking":"max"},
    "garden": {"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription","thinking":"max"}
  },
  "routes": {
    "default": {"harness":"zcode","model":"zai/glm-5.3","channel":"zai-coding-plan","thinking":"max"},
    "frontend": {"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription","thinking":"max"},
    "hard": {"harness":"kimi","model":"kimi-code/k3","channel":"kimi-subscription","thinking":"max"},
    "multimodal": {"harness":"zcode","model":"zai/glm-5.3-flash","channel":"zai-coding-plan","thinking":"max"}
  },
  "profiles": {"old-review":{"harness":"codex","model":"gpt-6-astra","channel":"chatgpt-subscription","thinking":"max"}},
  "channels": {
    "_note_user":"Preserve channel notes",
    "kimi-subscription":{"env-key":null,"fallback":"zai-coding-plan","_note_user":"Existing Kimi login"},
    "chatgpt-subscription":{"env-key":null,"fallback":"zai-coding-plan"},
    "chatgpt":{"env-key":"OLD_API_KEY","harness":"dsh","provider":"old-adapter","fallback":"zai-coding-plan","models":{"review":"zai/glm-5.3"}},
    "zai-coding-plan":{"env-key":null,"harness":"zcode","models":{"execute.default":"zai/glm-5.3"}}
  },
  "judge-panel":{"harness":"codex","model":"gpt-6-astra","channel":"chatgpt-subscription","thinking":"max"},
  "permissions":{"mode":"role-based","_note_user":"Explicit task policy"},
  "budget":{"enforce":"hard","daily-token-limit":9007199254740993},
  "watchdog":{"idle-timeout-ms":1234},
  "retry":{"jitter":0},
  "credentials":{"poll-interval-ms":1010},
  "worktree":{"enabled":false},
  "verify":{"command-timeout-ms":70000,"env-allowlist":["TEST_NAME"]},
  "pricing":{"old-model":{"input":0.2,"output":0.6,"cache-read":0.02}}
}
`

func migrationFixture(t *testing.T, raw string) (MigrationOptions, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "global", "flywheel.config.json")
	t.Setenv(GlobalConfigEnv, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	if raw != "" {
		require.NoError(t, os.WriteFile(path, []byte(raw), 0o640))
	}
	return MigrationOptions{Preset: "subscriptions", Cwd: root}, path
}

func migrationHooks(t *testing.T) {
	t.Helper()
	create, rename, link, remove, read, abs, embedded := createMigrationTemp, renameMigrationFile, linkMigrationFile, removeMigrationFile, readMigrationFile, absMigrationPath, embeddedTable
	t.Cleanup(func() {
		createMigrationTemp, renameMigrationFile, linkMigrationFile, removeMigrationFile, readMigrationFile, absMigrationPath, embeddedTable = create, rename, link, remove, read, abs, embedded
	})
}

func TestMigrateLegacySubscriptionsPreservesUserPolicyAndHasNoReachableZAI(t *testing.T) {
	options, path := migrationFixture(t, legacyMigrationConfig)
	before, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	preview, err := Migrate(options)
	require.NoError(t, err)
	require.Empty(t, preview.Blockers)
	assert.True(t, preview.Changed)
	assert.False(t, preview.Applied)
	assert.Empty(t, preview.Backup)
	assert.Equal(t, "global", preview.Scope)
	assert.Equal(t, path, preview.Source)
	assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, path))
	after, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, before, after, "preview must not create a lock, backup, or temporary file")
	assert.Contains(t, preview.Notes, "停用旧 multimodal 路由；不会把未经验证的附件任务改派给普通 Flash，仍引用该路由的任务需先重新规划。")
	assert.Contains(t, preview.Changes, MigrationChange{Path: "routes.multimodal", Action: "remove"})
	assert.Contains(t, preview.Changes, MigrationChange{Path: "channels.chatgpt.models", Action: "remove"})
	assert.Contains(t, preview.Changes, MigrationChange{Path: "loops.review.profile", Action: "remove"})
	assert.Contains(t, preview.Changes, MigrationChange{Path: "permissions.mode", Action: "set"})
	options.Apply, options.ExpectSHA256 = true, preview.SHA256
	applied, err := Migrate(options)
	require.NoError(t, err)
	require.True(t, applied.Applied)
	assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, applied.Backup))
	backupInfo, err := os.Stat(applied.Backup)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), backupInfo.Mode().Perm())
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	loaded, err := LoadReadOnly("", options.Cwd)
	require.NoError(t, err)
	assert.Equal(t, "full-access", loaded.Permissions.Mode)
	assert.False(t, loaded.Loops["plan"].ResidentSession)
	assert.Equal(t, "kimi-code/k3.1-pro", loaded.Loops["plan"].Model)
	assert.Equal(t, "kimi-code/k3.1-pro", loaded.Loops["garden"].Model)
	assert.Equal(t, "kimi-code/k3.1-pro", loaded.Routes["frontend"].Model)
	assert.Equal(t, "low", loaded.Loops["intake"].Thinking)
	assert.Equal(t, "deepseek-flash", loaded.Loops["intake"].Model)
	assert.Equal(t, "deepseek-flash", loaded.Routes["default"].Model)
	assert.Equal(t, "gpt-6.1-sol", loaded.Loops["review"].Model)
	assert.Equal(t, "gpt-6-astra", loaded.Loops["judge"].Model)
	assert.Equal(t, "codex-pro", loaded.Routes["hard"].Harness)
	assert.Equal(t, "gpt-6.1-sol", loaded.Routes["hard"].Model)
	assert.Nil(t, loaded.JudgePanel)
	assert.NotContains(t, loaded.Routes, "multimodal")
	for role, binding := range loaded.Loops {
		if role != "intake" {
			assert.Equal(t, "high", binding.Thinking)
		}
		assert.Contains(t, []string{"kimi-subscription", "chatgpt", "deepseek-account"}, binding.Channel)
		assert.Empty(t, loaded.Channels[binding.Channel].Fallback)
		assert.Empty(t, loaded.Channels[binding.Channel].Models)
		assert.Nil(t, loaded.Channels[binding.Channel].EnvKey)
	}
	for _, binding := range loaded.Routes {
		assert.Equal(t, "high", binding.Thinking)
		assert.Contains(t, []string{"kimi-subscription", "chatgpt-subscription", "deepseek-account"}, binding.Channel)
		assert.Empty(t, loaded.Channels[binding.Channel].Fallback)
	}
	var original, migrated map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(legacyMigrationConfig), &original))
	require.NoError(t, json.Unmarshal(configBytesT(t, path), &migrated))
	for _, key := range []string{"version", "_note_user", "profiles", "budget", "watchdog", "retry", "credentials", "worktree", "verify", "pricing"} {
		assert.JSONEq(t, string(original[key]), string(migrated[key]), "unrelated setting %s", key)
	}
	assert.Contains(t, string(configBytesT(t, path)), "9007199254740993", "do not round large numeric policy values through float64")
	assert.Contains(t, string(configBytesT(t, path)), "Keep short sessions")
	assert.Contains(t, string(configBytesT(t, path)), "Existing Kimi login")
	assert.Contains(t, string(configBytesT(t, path)), "Explicit task policy")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 2, "one active file and one complete backup")
	bytesAfter := configBytesT(t, path)
	options.ExpectSHA256 = ""
	repeated, err := Migrate(options)
	require.NoError(t, err)
	assert.False(t, repeated.Changed)
	assert.False(t, repeated.Applied)
	assert.Empty(t, repeated.Backup)
	assert.Empty(t, repeated.Changes)
	assert.Equal(t, migrationHash(bytesAfter), repeated.CandidateSHA256)
	assert.Equal(t, bytesAfter, configBytesT(t, path))
	entriesAfter, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, entries, entriesAfter)
}

func TestMigrateV0282APIConfigUsesAccountOnlyAfterExplicitApply(t *testing.T) {
	// Frozen release template from ee65ed812b76eee77e4a001aed506008b618a058.
	// Loading/upgrading must keep the user's existing API route; adoption of the
	// corrected subscription preset is a previewable, explicit migration.
	original, err := os.ReadFile(filepath.Join("testdata", "subscriptions-v0.28.2.json"))
	require.NoError(t, err)
	options, path := migrationFixture(t, string(original))
	before, err := LoadReadOnly("", options.Cwd)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-official", before.Loops["intake"].Channel)
	assert.Equal(t, "deepseek-official", before.Routes["default"].Channel)
	_, created, err := MaterializeGlobal()
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, original, configBytesT(t, path))

	preview, err := Migrate(options)
	require.NoError(t, err)
	assert.True(t, preview.Changed)
	assert.False(t, preview.Applied)
	assert.Empty(t, preview.Backup)
	assert.Equal(t, original, configBytesT(t, path))
	for _, field := range []string{"loops.intake.channel", "loops.intake.provider", "routes.default.channel", "routes.default.provider", "channels.deepseek-account.provider"} {
		assert.Contains(t, preview.Changes, MigrationChange{Path: field, Action: "set"})
	}
	assert.Equal(t, "deepseek-official", preview.Original.Routes["default"].Channel)
	assert.Equal(t, "deepseek-account", preview.Candidate.Routes["default"].Provider)
	assert.Equal(t, "deepseek-account", preview.Candidate.Loops["intake"].Provider)

	options.Apply, options.ExpectSHA256 = true, preview.SHA256
	applied, err := Migrate(options)
	require.NoError(t, err)
	require.True(t, applied.Applied)
	assert.Equal(t, original, configBytesT(t, applied.Backup))
	after, err := LoadReadOnly("", options.Cwd)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-account", after.Loops["intake"].Channel)
	assert.Equal(t, "deepseek-account", after.Loops["intake"].Provider)
	assert.Equal(t, "low", after.Loops["intake"].Thinking)
	assert.Equal(t, "deepseek-account", after.Routes["default"].Channel)
	assert.Equal(t, "deepseek-account", after.Routes["default"].Provider)
	assert.Equal(t, "high", after.Routes["default"].Thinking)
	assert.Equal(t, "deepseek-flash", after.Routes["default"].Model)
	account := after.Channels["deepseek-account"]
	assert.Equal(t, "dsh", account.Harness)
	assert.Equal(t, "deepseek-account", account.Provider)
	assert.Nil(t, account.EnvKey)
	assert.Empty(t, account.Fallback, "account failure must never switch to API billing")
	assert.Empty(t, account.Models)
	// Existing API channels remain available for an explicit future choice, but
	// no active binding or fallback in the new preset can reach them.
	assert.Equal(t, before.Channels["deepseek-official"], after.Channels["deepseek-official"])
	for role, binding := range after.Loops {
		assert.NotEqual(t, "deepseek-official", binding.Channel)
		assert.Empty(t, after.Channels[binding.Channel].Fallback)
		if role != "intake" {
			assert.Equal(t, before.Loops[role], binding)
		}
	}
	for route, binding := range after.Routes {
		assert.NotEqual(t, "deepseek-official", binding.Channel)
		assert.Empty(t, after.Channels[binding.Channel].Fallback)
		if route != "default" {
			assert.Equal(t, before.Routes[route], binding)
		}
	}
	assert.Equal(t, before.Permissions, after.Permissions)
	assert.Equal(t, before.Budget, after.Budget)
	assert.Equal(t, before.Verify, after.Verify)
	assert.Equal(t, before.Worktree, after.Worktree)
	options.ExpectSHA256 = ""
	repeated, err := Migrate(options)
	require.NoError(t, err)
	assert.False(t, repeated.Changed)
	assert.Empty(t, repeated.Backup)
}

func TestMigrationDiscoverySelectsOneCompleteFileAndCreatesOnlyGlobalWhenEmbedded(t *testing.T) {
	options, global := migrationFixture(t, "")
	preview, err := Migrate(options)
	require.NoError(t, err)
	assert.Equal(t, "embedded", preview.Scope)
	assert.Equal(t, global, preview.Target)
	assert.NoFileExists(t, global)
	options.Apply = true
	created, err := Migrate(options)
	require.NoError(t, err)
	assert.True(t, created.Applied)
	assert.Empty(t, created.Backup)
	assert.FileExists(t, global)
	assert.NoFileExists(t, filepath.Join(options.Cwd, "flywheel.config.json"))
	info, err := os.Stat(global)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	options.TaskDir = filepath.Join(options.Cwd, "docs", "task")
	require.NoError(t, os.MkdirAll(options.TaskDir, 0o755))
	project := filepath.Join(options.Cwd, "flywheel.config.json")
	task := filepath.Join(options.TaskDir, "flywheel.config.json")
	for _, selection := range []struct{ path, scope string }{{global, "global"}, {project, "project"}, {task, "task"}} {
		preserved := map[string][]byte{}
		for _, other := range []string{global, project, task} {
			if other != selection.path {
				if raw, readErr := os.ReadFile(other); readErr == nil {
					preserved[other] = raw
				}
			}
		}
		require.NoError(t, os.WriteFile(selection.path, []byte(legacyMigrationConfig), 0o600))
		result, err := Migrate(options)
		require.NoError(t, err)
		assert.Equal(t, selection.path, result.Source)
		assert.Equal(t, selection.scope, result.Scope)
		assert.True(t, result.Applied)
		for other, original := range preserved {
			assert.Equal(t, original, configBytesT(t, other), "must not also migrate %s", other)
		}
	}
}

func TestMigrationCustomBindingsAreBlockersAndNeverGuessed(t *testing.T) {
	options, path := migrationFixture(t, `{"loops":{"new-role":{"harness":"kimi"}},"routes":{"custom":{"harness":"zcode"}}}`)
	for _, apply := range []bool{false, true} {
		options.Apply = apply
		report, err := Migrate(options)
		require.NoError(t, err)
		require.Len(t, report.Blockers, 2)
		assert.Contains(t, report.Blockers[0], "loops.new-role")
		assert.Contains(t, report.Blockers[1], "routes.custom")
		assert.Nil(t, report.Candidate)
		assert.False(t, report.Applied)
		assert.Empty(t, report.Backup)
	}
	assert.Equal(t, `{"loops":{"new-role":{"harness":"kimi"}},"routes":{"custom":{"harness":"zcode"}}}`, string(configBytesT(t, path)))
}

func TestMigrationRejectsStalePreviewsAndInvalidInputsWithoutLeakingContents(t *testing.T) {
	options, path := migrationFixture(t, legacyMigrationConfig)
	preview, err := Migrate(options)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(`{"_note":"concurrent user edit"}`), 0o640))
	options.Apply, options.ExpectSHA256 = true, preview.SHA256
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "changed since preview")
	assert.Equal(t, `{"_note":"concurrent user edit"}`, string(configBytesT(t, path)))
	options.ExpectSHA256 = "not-a-digest"
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "SHA-256 hexadecimal")
	options.ExpectSHA256 = "aa"
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "SHA-256 hexadecimal")
	options.ExpectSHA256 = ""
	options.Preset = "guess-best"
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "--preset must be")
	options.Preset = "subscriptions"
	for _, raw := range []string{"", "null", " null ", "[]", "{", `{"routes":[]}`, `{"loops":{"plan":[]}}`, `{"secret-mistake":"do-not-echo-this"}`} {
		require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
		report, err := Migrate(options)
		require.Error(t, err, raw)
		assert.NotContains(t, err.Error(), "do-not-echo-this")
		assert.False(t, report.Applied)
		assert.Equal(t, raw, string(configBytesT(t, path)))
	}
}

func TestMigrationFilesystemBoundariesPreserveTheSource(t *testing.T) {
	options, path := migrationFixture(t, legacyMigrationConfig)
	options.Apply = true
	require.NoError(t, os.Chmod(path, 0o400))
	_, err := Migrate(options)
	assert.ErrorContains(t, err, "read-only")
	require.NoError(t, os.Chmod(path, 0))
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "not readable")
	require.NoError(t, os.Chmod(path, 0o600))
	assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, path))
	alias := filepath.Join(options.Cwd, "linked.json")
	require.NoError(t, os.Symlink(path, alias))
	t.Setenv(GlobalConfigEnv, alias)
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "symbolic link")
	t.Setenv(GlobalConfigEnv, filepath.Dir(path))
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "regular configuration")
	t.Setenv(GlobalConfigEnv, filepath.Join(path, "impossible.json"))
	_, err = Migrate(options)
	assert.Error(t, err)
	t.Setenv(GlobalConfigEnv, path)
	require.NoError(t, os.WriteFile(path+".migrate.lock", []byte("another migration"), 0o600))
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "acquire migration lock")
	assert.Equal(t, "another migration", string(configBytesT(t, path+".migrate.lock")))
	assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, path))
}

func TestMigrationDetectsExternalEditsWhilePreparingFiles(t *testing.T) {
	for _, change := range []string{"content", "identity", "permission", "delete", "lock"} {
		t.Run(change, func(t *testing.T) {
			migrationHooks(t)
			options, path := migrationFixture(t, legacyMigrationConfig)
			options.Apply = true
			createMigrationTemp = func(dir, pattern string) (migrationFile, error) {
				if strings.HasPrefix(pattern, ".flywheel-migrate-") {
					switch change {
					case "content":
						require.NoError(t, os.WriteFile(path, []byte(`{"_note":"new user bytes"}`), 0o640))
					case "identity":
						require.NoError(t, os.Rename(path, path+".user-backup"))
						require.NoError(t, os.WriteFile(path, []byte(legacyMigrationConfig), 0o640))
					case "permission":
						require.NoError(t, os.Chmod(path, 0o400))
					case "delete":
						require.NoError(t, os.Remove(path))
					case "lock":
						require.NoError(t, os.Rename(path+".migrate.lock", path+".old-lock"))
						require.NoError(t, os.WriteFile(path+".migrate.lock", []byte("new owner"), 0o600))
					}
				}
				return os.CreateTemp(dir, pattern)
			}
			report, err := Migrate(options)
			require.Error(t, err)
			assert.False(t, report.Applied)
			assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, report.Backup))
			if change == "content" {
				assert.Equal(t, `{"_note":"new user bytes"}`, string(configBytesT(t, path)))
			} else if change == "delete" {
				assert.NoFileExists(t, path)
			} else {
				assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, path))
			}
			if change == "lock" {
				assert.Equal(t, "new owner", string(configBytesT(t, path+".migrate.lock")))
			} else {
				assert.NoFileExists(t, path+".migrate.lock")
			}
		})
	}
}

type failedMigrationFile struct {
	*os.File
	failure string
	closed  bool
}

func (f *failedMigrationFile) Write(p []byte) (int, error) {
	if f.failure == "write" {
		return 0, errors.New("disk full")
	}
	return f.File.Write(p)
}
func (f *failedMigrationFile) Chmod(mode os.FileMode) error {
	if f.failure == "chmod" {
		return errors.New("chmod denied")
	}
	return f.File.Chmod(mode)
}
func (f *failedMigrationFile) Sync() error {
	if f.failure == "sync" {
		return errors.New("sync failed")
	}
	return f.File.Sync()
}
func (f *failedMigrationFile) Close() error {
	if f.failure == "close" && !f.closed {
		f.closed = true
		_ = f.File.Close()
		return errors.New("close failed")
	}
	return f.File.Close()
}

func TestMigrationDiskFailuresNeverReplaceTheSource(t *testing.T) {
	for _, operation := range []string{"create", "chmod", "write", "sync", "close", "publish"} {
		for _, stage := range []string{"backup", "candidate"} {
			t.Run(stage+"/"+operation, func(t *testing.T) {
				migrationHooks(t)
				options, path := migrationFixture(t, legacyMigrationConfig)
				options.Apply = true
				createMigrationTemp = func(dir, pattern string) (migrationFile, error) {
					failHere := (stage == "backup") == strings.Contains(pattern, ".backup-")
					if failHere && operation == "create" {
						return nil, errors.New("temporary file denied")
					}
					file, err := os.CreateTemp(dir, pattern)
					require.NoError(t, err)
					failure := ""
					if failHere {
						failure = operation
					}
					return &failedMigrationFile{File: file, failure: failure}, nil
				}
				renameMigrationFile = func(_, _ string) error { return errors.New("rename denied") }
				report, err := Migrate(options)
				require.Error(t, err)
				assert.False(t, report.Applied)
				assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, path))
				assert.NoFileExists(t, path+".migrate.lock")
				entries, err := os.ReadDir(filepath.Dir(path))
				require.NoError(t, err)
				for _, entry := range entries {
					assert.False(t, strings.HasSuffix(entry.Name(), ".tmp"))
				}
				if report.Backup != "" {
					assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, report.Backup))
				}
			})
		}
	}
}

func TestMigrationPublicationNeverReplacesARacingNewGlobal(t *testing.T) {
	migrationHooks(t)
	options, path := migrationFixture(t, "")
	options.Apply = true
	linkMigrationFile = func(source, target string) error {
		require.NoError(t, os.WriteFile(target, []byte(`{"_note":"new owner"}`), 0o600))
		return os.Link(source, target)
	}
	report, err := Migrate(options)
	require.ErrorContains(t, err, "publish migration")
	assert.False(t, report.Applied)
	assert.Empty(t, report.Backup)
	assert.Equal(t, `{"_note":"new owner"}`, string(configBytesT(t, path)))
}

func TestMigrationAllowsAbsentOrNullLegacyCollections(t *testing.T) {
	for _, raw := range []string{`{}`, `{"loops":null,"routes":null,"channels":null}`} {
		options, path := migrationFixture(t, raw)
		options.Apply = true
		report, err := Migrate(options)
		require.NoError(t, err)
		assert.True(t, report.Applied)
		assert.True(t, report.Candidate.Loops["plan"].ResidentSession)
		assert.Equal(t, raw, string(configBytesT(t, report.Backup)))
		assert.NotEqual(t, raw, string(configBytesT(t, path)))
	}
}

func TestMigrationIOAndEmbeddedFailuresAreExplicit(t *testing.T) {
	migrationHooks(t)
	options, path := migrationFixture(t, legacyMigrationConfig)
	readMigrationFile = func(string) ([]byte, error) { return nil, errors.New("read denied") }
	_, err := Migrate(options)
	assert.ErrorContains(t, err, "read denied")
	readMigrationFile = os.ReadFile
	for _, raw := range []string{"{", `{}`, `{"loops":[]}`, `{"loops":{"plan":[]}}`} {
		embeddedTable = func() ([]byte, error) { return []byte(raw), nil }
		_, err = Migrate(options)
		assert.Error(t, err)
	}
	embeddedTable = func() ([]byte, error) { return nil, errors.New("embedded table unavailable") }
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "embedded table unavailable")
	require.NoError(t, os.Remove(path))
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "embedded table unavailable")
}

func TestMigrationCandidateValidationPreservesSource(t *testing.T) {
	migrationHooks(t)
	options, path := migrationFixture(t, legacyMigrationConfig)
	raw, err := EmbeddedTable()
	require.NoError(t, err)
	corrupt := bytes.Replace(raw, []byte(`"full-access"`), []byte(`"unsupported"`), 1)
	require.NotEqual(t, raw, corrupt)
	embeddedTable = func() ([]byte, error) { return corrupt, nil }
	options.Apply = true
	_, err = Migrate(options)
	assert.ErrorContains(t, err, "candidate is invalid")
	assert.Equal(t, []byte(legacyMigrationConfig), configBytesT(t, path))
}

func TestMigrationDestinationErrorsAndEarlyConcurrentEdits(t *testing.T) {
	t.Run("no home for global destination", func(t *testing.T) {
		options, _ := migrationFixture(t, "")
		t.Setenv(GlobalConfigEnv, "")
		t.Setenv("HOME", "")
		_, err := Migrate(options)
		assert.ErrorContains(t, err, "resolve global migration destination")
	})
	t.Run("relative global path with unavailable process directory", func(t *testing.T) {
		migrationHooks(t)
		options, path := migrationFixture(t, "")
		t.Setenv(GlobalConfigEnv, "relative.json")
		// Removing cwd does not reliably make Getwd fail on macOS. Exercise
		// the same path-resolution failure without depending on OS semantics.
		absMigrationPath = func(string) (string, error) { return "", errors.New("working directory unavailable") }
		report, err := Migrate(options)
		assert.ErrorContains(t, err, "working directory unavailable")
		assert.False(t, report.Applied)
		assert.Empty(t, report.Backup)
		entries, readErr := os.ReadDir(filepath.Dir(path))
		require.NoError(t, readErr)
		assert.Empty(t, entries, "path resolution failure must not create configuration or backup files")
	})
	for _, mode := range []string{"read failure", "early content edit", "parent replaced"} {
		t.Run(mode, func(t *testing.T) {
			migrationHooks(t)
			options, path := migrationFixture(t, legacyMigrationConfig)
			options.Apply = true
			embeddedTable = func() ([]byte, error) {
				switch mode {
				case "read failure":
					readMigrationFile = func(string) ([]byte, error) { return nil, errors.New("file became unreadable") }
				case "early content edit":
					require.NoError(t, os.WriteFile(path, []byte(`{"_note":"edited during preview"}`), 0o640))
				case "parent replaced":
					require.NoError(t, os.Remove(path))
					require.NoError(t, os.Remove(filepath.Dir(path)))
					require.NoError(t, os.WriteFile(filepath.Dir(path), []byte("user replaced directory"), 0o600))
				}
				return EmbeddedTable()
			}
			report, err := Migrate(options)
			require.Error(t, err)
			assert.False(t, report.Applied)
			assert.Empty(t, report.Backup)
			if mode == "early content edit" {
				assert.Equal(t, `{"_note":"edited during preview"}`, string(configBytesT(t, path)))
			}
		})
	}
}

func TestMigrationCleanupErrorsDoNotHideAnAppliedConfiguration(t *testing.T) {
	for _, target := range []string{"lock", "temporary"} {
		t.Run(target, func(t *testing.T) {
			migrationHooks(t)
			options, path := migrationFixture(t, "")
			options.Apply = true
			removeMigrationFile = func(name string) error {
				if (target == "lock" && strings.HasSuffix(name, ".migrate.lock")) || (target == "temporary" && strings.HasSuffix(name, ".tmp")) {
					return errors.New("cleanup denied")
				}
				return os.Remove(name)
			}
			report, err := Migrate(options)
			require.ErrorContains(t, err, "cleanup denied")
			assert.True(t, report.Applied, "report must not imply an applied change did not occur")
			assert.Equal(t, report.CandidateSHA256, migrationHash(configBytesT(t, path)))
		})
	}
}
