package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const profileDefinition = `{"harness":"dsh","model":"flash","channel":"account","provider":"deepseek-official","thinking":"high"}`

func TestProfilesExpandToEquivalentInlineBindings(t *testing.T) {
	profileRaw := `{
	  "_note": "annotations remain supported",
	  "profiles": {
	    "flash": {"harness":"dsh","model":"flash","channel":"account","provider":"deepseek-official","thinking":"high","_note_source":"shared"},
	    "kimi": {"harness":"kimi","model":"kimi-model","channel":"subscription"}
	  },
	  "loops": {
	    "plan":{"profile":"kimi","resident-session":true},
	    "review":{"profile":"flash","thinking":"","resident-session":false},
	    "judge":{"profile":"flash"}
	  },
	  "routes": {
	    "default":{"profile":"flash"},
	    "custom":{"profile":"flash","harness":"codex","model":"gpt","channel":"codex-subscription","provider":"","thinking":"low"}
	  },
	  "judge-panel":{"profile":"flash","model":"second-opinion","resident-session":false},
	  "channels":{"account":{"harness":"dsh","provider":"deepseek-official","env-key":null,"_note":"SDK credentials"}},
	  "worktree":{"enabled":false}, "retry":{"jitter":0}
	}`
	inlineRaw := `{
	  "loops": {
	    "plan":{"harness":"kimi","model":"kimi-model","channel":"subscription","resident-session":true},
	    "review":{"harness":"dsh","model":"flash","channel":"account","provider":"deepseek-official","thinking":"","resident-session":false},
	    "judge":{"harness":"dsh","model":"flash","channel":"account","provider":"deepseek-official","thinking":"high"}
	  },
	  "routes": {
	    "default":{"harness":"dsh","model":"flash","channel":"account","provider":"deepseek-official","thinking":"high"},
	    "custom":{"harness":"codex","model":"gpt","channel":"codex-subscription","thinking":"low"}
	  },
	  "judge-panel":{"harness":"dsh","model":"second-opinion","channel":"account","provider":"deepseek-official","thinking":"high","resident-session":false},
	  "channels":{"account":{"harness":"dsh","provider":"deepseek-official","env-key":null}},
	  "worktree":{"enabled":false}, "retry":{"jitter":0}
	}`
	profile, err := parseConfig([]byte(profileRaw), "profiles.json")
	require.NoError(t, err)
	inline, err := parseConfig([]byte(inlineRaw), "inline.json")
	require.NoError(t, err)
	assert.Equal(t, inline.Loops, profile.Loops)
	assert.Equal(t, inline.Routes, profile.Routes)
	assert.Equal(t, inline.JudgePanel, profile.JudgePanel)
	assert.Equal(t, inline.Channels, profile.Channels)
	assert.False(t, *profile.Worktree.Enabled)
	assert.Zero(t, *profile.Retry.Jitter)
	assert.Nil(t, profile.Channels["account"].EnvKey)
	assert.True(t, profile.Loops["plan"].ResidentSession)
	assert.False(t, profile.Loops["review"].ResidentSession)
	assert.Empty(t, profile.Loops["review"].Thinking, "an explicit empty effort selects the harness default")
	assert.Equal(t, "high", profile.Loops["judge"].Thinking, "an override cannot modify another reference")
	assert.Equal(t, "flash", profile.Profiles["flash"].Model, "per-stage overrides cannot mutate a definition")
	assert.Equal(t, map[string]string{
		"loops.plan": "kimi", "loops.review": "flash", "loops.judge": "flash",
		"routes.default": "flash", "routes.custom": "flash", "judge-panel": "flash",
	}, profile.ProfileRefs)

	// The effective representation is loadable without keeping a second interpreter
	// in consumers; profile provenance is deliberately separate from runtime values.
	raw, err := json.Marshal(profile)
	require.NoError(t, err)
	reloaded, err := parseConfig(raw, "effective.json")
	require.NoError(t, err)
	assert.Equal(t, profile.Loops, reloaded.Loops)
	assert.Equal(t, profile.Routes, reloaded.Routes)
	assert.Equal(t, profile.JudgePanel, reloaded.JudgePanel)
	assert.Empty(t, reloaded.ProfileRefs)
}

func TestProfilesRejectInvalidDefinitionsReferencesAndFields(t *testing.T) {
	ref := func(binding string) string {
		return `{"profiles":{"fast":` + profileDefinition + `},"loops":{"review":` + binding + `}}`
	}
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"invalid JSON", `{`, "config"},
		{"root null", `null`, "config"},
		{"root array", `[]`, "config"},
		{"top typo", `{"profiels":{}}`, "profiels"},
		{"case alias bypass", `{"Loops":{"typo":{}}}`, `unknown field "Loops"`},
		{"unrecognized annotation", `{"_notes":"typo"}`, "_notes"},
		{"profiles null", `{"profiles":null}`, "profiles"},
		{"profiles array", `{"profiles":[]}`, "profiles"},
		{"blank name", `{"profiles":{" ":` + profileDefinition + `}}`, "name must not be blank"},
		{"definition null", `{"profiles":{"fast":null}}`, "profiles.fast"},
		{"inheritance", `{"profiles":{"fast":{"profile":"base"}}}`, `unknown field "profile"`},
		{"resident policy in profile", `{"profiles":{"fast":{"resident-session":true}}}`, "resident-session"},
		{"missing harness", `{"profiles":{"fast":{"model":"m","channel":"c"}}}`, "profiles.fast.harness"},
		{"missing model", `{"profiles":{"fast":{"harness":"dsh","channel":"c"}}}`, "profiles.fast.model"},
		{"missing channel", `{"profiles":{"fast":{"harness":"dsh","model":"m"}}}`, "profiles.fast.channel"},
		{"unknown profile harness", `{"profiles":{"fast":{"harness":"typo","model":"m","channel":"c"}}}`, "unsupported harness"},
		{"loops type", `{"loops":[]}`, "loops"},
		{"routes type", `{"routes":false}`, "routes"},
		{"invalid loop", `{"loops":{"execute":{}}}`, "unknown loop role"},
		{"misplaced panel", `{"loops":{"judge-panel":{}}}`, "unknown loop role"},
		{"binding null", `{"loops":{"review":null}}`, "loops.review"},
		{"binding string", `{"loops":{"review":"fast"}}`, "loops.review"},
		{"reference typo", `{"loops":{"review":{"profille":"fast"}}}`, "profille"},
		{"unknown reference", `{"loops":{"review":{"profile":"missing"}}}`, "unknown profile"},
		{"null reference", ref(`{"profile":null}`), "loops.review.profile"},
		{"numeric reference", ref(`{"profile":5}`), "loops.review.profile"},
		{"empty reference", ref(`{"profile":" "}`), "must not be blank"},
		{"null harness", ref(`{"profile":"fast","harness":null}`), "loops.review.harness"},
		{"empty harness", ref(`{"profile":"fast","harness":""}`), "loops.review.harness"},
		{"blank model", ref(`{"profile":"fast","model":" "}`), "loops.review.model"},
		{"number model", ref(`{"profile":"fast","model":3}`), "loops.review.model"},
		{"empty channel", ref(`{"profile":"fast","channel":""}`), "loops.review.channel"},
		{"null thinking", ref(`{"profile":"fast","thinking":null}`), "loops.review.thinking"},
		{"blank thinking", ref(`{"profile":"fast","thinking":" "}`), "loops.review.thinking"},
		{"null provider", ref(`{"profile":"fast","provider":null}`), "loops.review.provider"},
		{"null resident", ref(`{"profile":"fast","resident-session":null}`), "resident-session"},
		{"string resident", ref(`{"profile":"fast","resident-session":"false"}`), "resident-session"},
		{"route session", `{"routes":{"default":{"resident-session":false}}}`, "resident-session"},
		{"bad panel", `{"judge-panel":{"profile":"missing"}}`, "judge-panel.profile"},
		{"empty panel", `{"judge-panel":{}}`, "judge-panel requires"},
		{"section type", `{"budget":[]}`, "budget"},
		{"nested typo", `{"verify":{"env-alowlist":[]}}`, "env-alowlist"},
		{"intake sources type", `{"intake":{"sources":{}}}`, "intake.sources"},
		{"intake source null", `{"intake":{"sources":[null]}}`, "intake.sources[0]"},
		{"channels type", `{"channels":[]}`, "channels"},
		{"channel null", `{"channels":{"account":null}}`, "channels.account"},
		{"channel provider null", `{"channels":{"account":{"provider":null}}}`, "channels.account.provider"},
		{"channel typo", `{"channels":{"account":{"provder":"chatgpt"}}}`, "provder"},
		{"channel provider case alias", `{"channels":{"account":{"Provider":null}}}`, `unknown field "Provider"`},
		{"drivers type", `{"drivers":[]}`, "drivers"},
		{"driver role", `{"drivers":{"execut":"real"}}`, "unknown driver role"},
		{"driver null", `{"drivers":{"execute":null}}`, "drivers.execute"},
		{"driver value", `{"drivers":{"execute":"reel"}}`, "real or mock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig([]byte(tc.raw), "invalid.json")
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, cfg, "invalid configuration cannot reach the scheduler")
		})
	}
}

func TestProviderDeclarationsRemainSeparateFromChannels(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"loop non-DSH", `{"loops":{"plan":{"harness":"kimi","provider":"chatgpt"}}}`, "loops.plan"},
		{"route non-DSH", `{"routes":{"hard":{"harness":"codex","provider":"chatgpt"}}}`, "routes.hard"},
		{"panel non-DSH", `{"judge-panel":{"harness":"codex","model":"m","channel":"c","provider":"chatgpt"}}`, "judge-panel"},
		{"unused profile non-DSH", `{"profiles":{"bad":{"harness":"kimi","model":"m","channel":"c","provider":"chatgpt"}}}`, "profiles.bad"},
		{"channel needs harness", `{"channels":{"account":{"provider":"chatgpt"}}}`, "channels.account.provider"},
		{"channel blank provider", `{"channels":{"account":{"harness":"dsh","provider":" "}}}`, "channels.account.provider"},
		{"provider conflict", `{"channels":{"account":{"harness":"dsh","provider":"chatgpt"}},"loops":{"review":{"harness":"dsh","channel":"account","provider":"other"}}}`, "conflicts with channels.account.provider"},
		{"channel provider for non-DSH binding", `{"channels":{"account":{"harness":"dsh","provider":"chatgpt"}},"routes":{"hard":{"harness":"codex","channel":"account"}}}`, "routes.hard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig([]byte(tc.raw), "invalid-provider.json")
			require.ErrorContains(t, err, tc.want)
		})
	}
	cfg, err := parseConfig([]byte(`{
	  "loops":{"review":{"harness":"dsh","model":"m","channel":"account"}},
	  "routes":{"default":{"harness":"dsh","model":"m","channel":"legacy-provider"}},
	  "channels":{"account":{"harness":"dsh","provider":"chatgpt","env-key":null}}
	}`), "providers.json")
	require.NoError(t, err)
	assert.Equal(t, "account", cfg.Loops["review"].Channel)
	assert.Empty(t, cfg.Loops["review"].Provider, "fallback resolves channel metadata; config must not erase provenance")
	assert.Equal(t, "chatgpt", cfg.Channels["account"].Provider)
	assert.Empty(t, cfg.Routes["default"].Provider, "legacy DSH channel IDs remain supported")

	programmatic := Default()
	programmatic.Profiles = map[string]ModelProfile{"bad": {Harness: "dsh"}}
	require.ErrorContains(t, programmatic.Validate(), "profiles.bad requires")
	programmatic.Profiles = nil
	programmatic.Loops = map[string]LoopBinding{"review": {Harness: "dsh", Provider: " "}}
	require.ErrorContains(t, programmatic.Validate(), "provider must not be blank")
}

func TestStrictConfigPreservesLegacyDefaultsAndAnnotations(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
	  "_note_global":"kept as documentation", "profiles":{}, "loops":{"plan":{"harness":""}},
	  "routes":null, "judge-panel":null, "drivers":null, "budget":null,
	  "watchdog":{"_note":"default"}, "credentials":{}, "verify":{"_note_policy":"empty"},
	  "channels":{"legacy":{"env-key":null,"models":{"_note_model":"m"},"_note":"OAuth"}},
	  "retry":{"jitter":0}, "worktree":{"enabled":false},
	  "intake":{"sources":[{"name":"issues","type":"command","cmd":"true","_note":"source"}]},
	  "pricing":{"_note_model":{"input":1}}, "mock-behaviors":{"_note_event":{"some":"payload"}}
	}`), "legacy.json")
	require.NoError(t, err)
	assert.Empty(t, cfg.Loops["plan"].Harness, "legacy inline default still selects pi downstream")
	assert.Nil(t, cfg.JudgePanel)
	assert.Empty(t, cfg.ProfileRefs)
	assert.Equal(t, "m", cfg.Channels["legacy"].Models["_note_model"])
	assert.Equal(t, 1.0, cfg.Pricing["_note_model"].Input)
	assert.Contains(t, cfg.MockBehaviors, "_note_event", "arbitrary payload keys are not annotation locations")
	assert.Equal(t, "true", cfg.Intake.Sources[0].Cmd)
	assert.Equal(t, "real", cfg.Drivers["execute"])
}

func TestProfilesExampleUsesRealLoadingContract(t *testing.T) {
	raw, err := templates.FS.ReadFile("flywheel.profiles.example.json")
	require.NoError(t, err)
	cfg, err := parseConfig(raw, "embedded profiles example")
	require.NoError(t, err)
	assert.Equal(t, "flash", cfg.ProfileRefs["routes.default"])
	assert.Equal(t, "deepseek-flash", cfg.Routes["default"].Model)
	assert.Equal(t, "deepseek-official", cfg.Routes["default"].Provider)
	assert.NotEqual(t, cfg.Routes["default"].Channel, cfg.Routes["default"].Provider)
	assert.Equal(t, "low", cfg.Loops["intake"].Thinking)
	assert.Equal(t, "high", cfg.Routes["default"].Thinking)
	assert.True(t, cfg.Loops["plan"].ResidentSession)
	assert.Nil(t, cfg.JudgePanel)
}

func TestLoadReadOnlyNeverMaterializesAndKeepsDiscoveryPriority(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	task := filepath.Join(project, "docs", "task")
	require.NoError(t, os.MkdirAll(task, 0o755))
	global := filepath.Join(root, "global", "flywheel.config.json")
	t.Setenv(GlobalConfigEnv, global)
	embedded, err := LoadReadOnly(task, project)
	require.NoError(t, err)
	assert.Equal(t, "(embedded legal table)", embedded.File)
	assert.NoDirExists(t, filepath.Dir(global), "inspection must not install a global config or create its parent")

	require.NoError(t, os.MkdirAll(filepath.Dir(global), 0o755))
	require.NoError(t, os.WriteFile(global, []byte(`{"profiles":{"global":`+profileDefinition+`}}`), 0o644))
	trap := filepath.Join(root, "unrelated-cwd")
	require.NoError(t, os.MkdirAll(trap, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(trap, "flywheel.config.json"), []byte(`{broken`), 0o644))
	t.Chdir(trap)
	assert.Empty(t, FindConfigFile("", ""))
	assert.Empty(t, FindConfigFile("", project), "empty task dir must not inspect the process cwd")
	cfg, err := LoadReadOnly("", project)
	require.NoError(t, err)
	assert.Equal(t, global, cfg.File)

	projectFile := filepath.Join(project, "flywheel.config.json")
	require.NoError(t, os.WriteFile(projectFile, []byte(`{"loops":{"review":{"profile":"global"}}}`), 0o644))
	_, err = LoadReadOnly("", project)
	require.ErrorContains(t, err, "unknown profile", "project files do not inherit a global profile registry")
	require.NoError(t, os.WriteFile(projectFile, []byte(`{"version":7}`), 0o644))
	cfg, err = LoadReadOnly(task, project)
	require.NoError(t, err)
	assert.Equal(t, projectFile, cfg.File)
	assert.Equal(t, 7, *cfg.Version)

	taskFile := filepath.Join(task, "flywheel.config.json")
	require.NoError(t, os.WriteFile(taskFile, []byte(`{broken`), 0o644))
	_, err = LoadReadOnly(task, project)
	require.ErrorContains(t, err, taskFile, "bad task config cannot fall back to a valid project/global file")
	require.NoError(t, os.WriteFile(taskFile, []byte(`{"version":9}`), 0o644))
	cfg, err = LoadReadOnly(task, project)
	require.NoError(t, err)
	assert.Equal(t, taskFile, cfg.File)
	assert.Equal(t, 9, *cfg.Version)
}

func TestBrokenOrUnreadablePriorityFilesDoNotFallBack(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	task := filepath.Join(root, "task")
	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, os.MkdirAll(task, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "flywheel.config.json"), []byte(`{"version":3}`), 0o644))
	global := filepath.Join(root, "global.json")
	t.Setenv(GlobalConfigEnv, global)
	missing := filepath.Join(root, "missing-target.json")
	require.NoError(t, os.Symlink(missing, global))
	_, created, err := MaterializeGlobal()
	require.NoError(t, err)
	assert.False(t, created)
	assert.NoFileExists(t, missing, "materialization cannot overwrite an existing dangling symlink")
	for _, readOnly := range []bool{false, true} {
		_, err = load("", filepath.Join(root, "missing-project"), !readOnly)
		require.ErrorContains(t, err, global)
	}

	taskFile := filepath.Join(task, "flywheel.config.json")
	require.NoError(t, os.Symlink(missing, taskFile))
	_, err = LoadReadOnly(task, project)
	require.ErrorContains(t, err, taskFile)
	require.NoError(t, os.Remove(taskFile))
	require.NoError(t, os.WriteFile(taskFile, []byte(`{"version":4}`), 0o644))
	require.NoError(t, os.Chmod(task, 0o000))
	t.Cleanup(func() { _ = os.Chmod(task, 0o755) })
	_, err = LoadReadOnly(task, project)
	require.ErrorContains(t, err, taskFile, "inaccessible higher-priority paths are not proof of absence")
}
