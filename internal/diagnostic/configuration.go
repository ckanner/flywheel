// Package diagnostic inspects the same effective configuration as execution,
// without creating task state, materializing global config, or calling models.
package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/fallback"
)

// Binding is a resolved stage, not a promise that an account can serve its model.
type Binding struct {
	ID              string        `json:"id"`
	Role            string        `json:"role"`
	Route           string        `json:"route"`
	Profile         string        `json:"profile"`
	Harness         string        `json:"harness"`
	Provider        string        `json:"provider"`
	Channel         string        `json:"channel"`
	Model           string        `json:"model"`
	Thinking        string        `json:"thinking"`
	ResidentSession bool          `json:"residentSession"`
	PermissionMode  string        `json:"permissionMode"`
	InScope         bool          `json:"inScope"`
	Session         SessionPolicy `json:"session"`
}

type Configuration struct {
	Scheduler config.SchedulerConfig `json:"scheduler"`
	Source    string                 `json:"source"`
	Scope     string                 `json:"scope"`
	Cwd       string                 `json:"cwd"`
	TaskDir   string                 `json:"taskDir"`
	Bindings  []Binding              `json:"bindings"`
	Sessions  config.SessionsConfig  `json:"sessions"`
}

var (
	directoryAbs  = filepath.Abs
	directoryStat = os.Stat
)

func resolveDirectory(path string) (string, error) {
	abs, err := directoryAbs(path)
	if err != nil {
		return "", err
	}
	physical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := directoryStat(physical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", physical)
	}
	return physical, nil
}

// Inspect uses read-only discovery. A bad higher-priority file is an error,
// never a reason to silently select a different account or model definition.
func Inspect(cwd, taskDir string) (*Configuration, error) {
	return InspectContext(context.Background(), cwd, taskDir)
}

// InspectContext lets CLI/control signal handlers cancel the bounded read-only
// discovery and await its subprocess cleanup before returning a response.
func InspectContext(parent context.Context, cwd, taskDir string) (*Configuration, error) {
	ctx, cancel := context.WithTimeout(parent, doctorBudget)
	defer cancel()
	view, _, err := inspect(ctx, cwd, taskDir)
	return view, err
}

func inspect(ctx context.Context, cwd, taskDir string) (*Configuration, *config.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	resolvedCwd, err := resolveDirectory(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve working directory: %w", err)
	}
	if taskDir != "" {
		if !filepath.IsAbs(taskDir) {
			taskDir = filepath.Join(resolvedCwd, taskDir)
		}
		taskDir, err = resolveDirectory(taskDir)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve task directory: %w", err)
		}
	}
	resolvedCwd, err = projectRoot(ctx, resolvedCwd)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.LoadReadOnly(taskDir, resolvedCwd)
	if err != nil {
		// Config validators can quote user-provided values. A malformed file may
		// contain a misplaced credential; never echo its contents to diagnostics.
		return nil, nil, errors.New("effective configuration is invalid or unreadable; check JSON fields, profile references and harness/provider bindings in the selected task/project/global file")
	}
	return configurationView(cfg, resolvedCwd, taskDir), cfg, nil
}

// configurationView is shared by read-only inspection and an explicit migration
// preview so both show the exact model defaults, providers and permission policy.
func configurationView(cfg *config.Config, resolvedCwd, taskDir string) *Configuration {
	scope := "global"
	switch {
	case cfg.File == "(embedded legal table)":
		scope = "embedded"
	case taskDir != "" && cfg.File == filepath.Join(taskDir, "flywheel.config.json"):
		scope = "task"
	case cfg.File == filepath.Join(resolvedCwd, "flywheel.config.json"):
		scope = "project"
	}
	view := &Configuration{Scheduler: cfg.Scheduler, Source: cfg.File, Scope: scope, Cwd: resolvedCwd, TaskDir: taskDir, Bindings: []Binding{}, Sessions: cfg.Sessions}
	add := func(id, role, route string, b config.LoopBinding) {
		harness := b.Harness
		if harness == "" {
			harness = "pi" // same legacy default as InvokeHarness
		}
		model, thinking := b.Model, b.Thinking
		if model == "" {
			switch harness {
			case "kimi":
				model = drivers.KimiDefaultModel
			case "codex":
				model = drivers.CodexDefaultModel
			case "zcode":
				model = drivers.ZcodeDefaultModel
			}
		}
		if thinking == "" {
			thinking = "high"
			if harness == "pi" || harness == "omp" || harness == "zcode" {
				thinking = "max"
			}
		}
		permission := cfg.PermissionMode(role, harness, id)
		view.Bindings = append(view.Bindings, Binding{
			ID: id, Role: role, Route: route, Profile: cfg.ProfileRefs[id], Harness: harness,
			Provider: fallback.ResolveProvider(cfg, harness, b.Provider, b.Channel), Channel: b.Channel,
			Model: model, Thinking: thinking, ResidentSession: b.ResidentSession && role == "plan" && harness == "kimi",
			PermissionMode: permission, InScope: true,
			Session: sessionPolicy(cfg, role, harness, b),
		})
	}
	for role, b := range cfg.Loops {
		add("loops."+role, role, "", b)
	}
	for route, b := range cfg.Routes {
		add("routes."+route, "execute", route, config.LoopBinding{
			Harness: b.Harness, Provider: b.Provider, Channel: b.Channel, Model: b.Model, Thinking: b.Thinking,
		})
	}
	if cfg.JudgePanel != nil {
		add("judge-panel", "judge-panel", "", *cfg.JudgePanel)
	}
	sort.Slice(view.Bindings, func(i, j int) bool { return strings.Compare(view.Bindings[i].ID, view.Bindings[j].ID) < 0 })
	return view
}

// projectRoot shares execution's Git-root discovery without an unbounded
// exec.Command outside the diagnostic deadline. A non-Git directory remains
// inspectable; a timeout, cancellation or incomplete process is not a fallback.
func projectRoot(ctx context.Context, cwd string) (string, error) {
	result := runCommand("git", []string{"-C", cwd, "rev-parse", "--show-toplevel"}, commandOptions(ctx, cwd))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if result.Cancelled || result.TimedOut || result.Truncated || result.Signal != "" || result.CleanupError != "" {
		return "", errors.New("working-directory discovery was interrupted or did not finish cleanly")
	}
	if !cleanCommand(result) {
		return cwd, nil
	}
	root := strings.TrimSpace(result.Stdout)
	if !filepath.IsAbs(root) {
		return "", errors.New("Git returned an invalid project directory")
	}
	return resolveDirectory(root)
}
