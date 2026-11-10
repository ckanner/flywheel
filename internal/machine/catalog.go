package machine

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/fallback"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/verify"
	"github.com/ckanner/flywheel/internal/watchdog"
)

// RunRecord is a discovery index, never an authority for liveness or completion.
// One file per invocation avoids lost updates between unrelated task leases.
type RunQuality struct {
	Units                   int `json:"units"`
	NewCompletions          int `json:"newCompletions"`
	NewAttempts             int `json:"newAttempts"`
	NewFailedAttempts       int `json:"newFailedAttempts"`
	FirstAttemptCompletions int `json:"firstAttemptCompletions"`
}
type RunRecord struct {
	HostSessionID string                         `json:"hostSessionId,omitempty"`
	Quality       RunQuality                     `json:"quality"`
	Head          string                         `json:"head,omitempty"`
	Branch        string                         `json:"branch,omitempty"`
	ElapsedMs     *int64                         `json:"elapsedMs,omitempty"`
	Kind          string                         `json:"kind"`
	Version       int                            `json:"version"`
	ID            string                         `json:"id"`
	TaskDir       string                         `json:"taskDir"`
	Cwd           string                         `json:"cwd"`
	StartedAt     string                         `json:"startedAt"`
	FinishedAt    string                         `json:"finishedAt,omitempty"`
	Conditions    string                         `json:"conditions"`
	ConfigHash    string                         `json:"configHash"`
	ContractHash  string                         `json:"contractHash"`
	Workflow      string                         `json:"workflow"`
	Bindings      map[string]config.LoopBinding  `json:"bindings"`
	Routes        map[string]config.RouteBinding `json:"routes"`
	Sessions      config.SessionsConfig          `json:"sessions"`
	Permissions   config.PermissionsConfig       `json:"permissions"`
	Summary       TaskSummary                    `json:"summary"`
	Error         string                         `json:"error,omitempty"`
}

func RunIndexDirectory() (string, error) {
	if value := os.Getenv("FLYWHEEL_RUNS_DIR"); value != "" {
		if !filepath.IsAbs(value) {
			return "", fmt.Errorf("FLYWHEEL_RUNS_DIR must be absolute")
		}
		return value, nil
	}
	path, err := config.GlobalConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "runs"), nil
}

var catalogLstat = os.Lstat
var catalogRead = os.ReadFile
var catalogTemp = os.CreateTemp
var catalogEntries = os.ReadDir
var catalogWrite = (*os.File).Write
var catalogSync = (*os.File).Sync
var catalogRename = os.Rename

func writeRunRecord(directory string, record *RunRecord) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	info, err := catalogLstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("run index must be a real directory")
	}
	file, err := catalogTemp(directory, ".run-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	raw, _ := json.Marshal(record) // fixed, typed, finite engine counters
	if _, err := catalogWrite(file, append(raw, '\n')); err != nil {
		return err
	}
	if err := catalogSync(file); err != nil {
		return err
	}
	return catalogRename(file.Name(), filepath.Join(directory, record.ID+".json"))
}

// ListRuns reads only explicit registrations. It never walks projects or homes.
func ListRuns(directory string) ([]RunRecord, error) {
	entries, err := catalogEntries(directory)
	if os.IsNotExist(err) {
		return []RunRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, fmt.Errorf("run index exceeds 10000 entries; archive older records")
	}
	records := []RunRecord{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil, fmt.Errorf("invalid run index entry %s", entry.Name())
		}
		raw, err := catalogRead(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record RunRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, err
		}
		if record.Version != 1 || record.ID+".json" != entry.Name() || !filepath.IsAbs(record.TaskDir) || !filepath.IsAbs(record.Cwd) {
			return nil, fmt.Errorf("invalid run identity in %s", entry.Name())
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].StartedAt > records[j].StartedAt })
	return records[:min(len(records), 100)], nil
}

var hostSessionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)

func (c *Context) beginRunRecord(directory string, kinds ...string) (func(*error), error) {
	if directory == "" {
		return func(*error) {}, nil
	}
	c.runID = rand.Text()
	started := c.Now()
	baseline := map[string]state.PacketState{}
	for id, packet := range c.State.Packets {
		baseline[id] = *packet
	}

	hostSession, _ := c.EnvLookup("DSH_SESSION_ID")
	if !hostSessionPattern.MatchString(hostSession) {
		hostSession = ""
	}
	record := &RunRecord{HostSessionID: hostSession, Version: 1, ID: c.runID, TaskDir: c.TaskDir, Cwd: c.Cwd, StartedAt: c.nowISO(),
		ConfigHash: sessionHash(c.Config), ContractHash: sessionHash(c.State.ContractFingerprints),
		Bindings: c.Config.Loops, Routes: c.Config.Routes, Sessions: c.Config.Sessions, Permissions: c.Config.Permissions,
		Workflow: c.State.Workflow.Effective, Summary: c.summary()}
	if len(kinds) > 0 {
		record.Kind = kinds[0]
	}
	record.Conditions = sessionHash([]string{record.ConfigHash, record.ContractHash, record.Workflow, record.Kind})
	record.Head, _ = verify.GetBaseRef(c.Cwd)
	branch := watchdog.Run("git", []string{"symbolic-ref", "--quiet", "--short", "HEAD"}, watchdog.Opts{Context: c.RunContext, Cwd: c.Cwd, Env: childenv.EnvSlice(childenv.GateEnv(c.EnvLookup, nil)), WallTimeoutMs: 5000, IdleTimeoutMs: 5000})
	if branch.ExitCode != nil && *branch.ExitCode == 0 && !branch.TimedOut && !branch.Cancelled && !branch.Truncated && branch.Signal == "" && branch.CleanupError == "" {
		record.Branch = strings.TrimSpace(branch.Stdout)
	}
	if err := writeRunRecord(directory, record); err != nil {
		return nil, fmt.Errorf("register run: %w", err)
	}
	if c.State.Project == nil {
		c.State.Project = &state.ProjectIdentity{Cwd: c.Cwd}
		if origin, err := deliveryRun(c.RunContext, c.Cwd, "git", "remote", "get-url", "origin"); err == nil {
			repo, url, err := githubRemote(origin)
			if err == nil {
				c.State.Project.Repo, c.State.Project.URL = repo, url
			}
		}
	}
	if c.State.Sessions == nil {
		c.State.Sessions = &state.SessionLedger{Version: 2, Records: map[string]*state.SessionRecord{}}
	}
	c.State.Sessions.Version = 2
	c.State.Run = &state.RunIdentity{Bindings: c.runBindings(), Drivers: c.Config.Drivers, ID: record.ID, Kind: record.Kind, Head: record.Head, Branch: record.Branch, Conditions: record.Conditions, StartedAt: record.StartedAt}
	c.save()
	if err := c.err(); err != nil {
		return nil, err
	}
	return func(runErr *error) {
		ended := c.Now()
		if !ended.Before(started) {
			value := ended.Sub(started).Milliseconds()
			record.ElapsedMs = &value
		}
		record.Quality = c.runQuality(baseline)
		record.FinishedAt, record.Summary = c.nowISO(), c.summary()
		if *runErr != nil {
			record.Error = (*runErr).Error()
		}
		*runErr = errors.Join(*runErr, writeRunRecord(directory, record))
	}, nil
}

func (c *Context) runQuality(before map[string]state.PacketState) RunQuality {
	quality := RunQuality{Units: len(c.PacketList)}
	for _, packet := range c.PacketList {
		after := c.State.Packets[packet.PacketID]
		prior := before[packet.PacketID]
		quality.NewAttempts += max(0, after.Attempts-prior.Attempts)
		// Plan may reset the retry threshold after a contract revision. The audit
		// history is append-only, so count newly recorded business failures instead.
		for _, event := range after.RetryHistory[min(len(prior.RetryHistory), len(after.RetryHistory)):] {
			if row, ok := event.(map[string]any); ok && row["scope"] == "attempt" && row["policy"] == "requeue" {
				quality.NewFailedAttempts++
			}
		}
		if after.Status == "completed" && prior.Status != "completed" {
			quality.NewCompletions++
			if after.Attempts == 1 && after.FailedAttempts == 0 {
				quality.FirstAttemptCompletions++
			}
		}
	}
	return quality
}

func (c *Context) runBindings() map[string]state.SessionConfiguration {
	result := map[string]state.SessionConfiguration{}
	add := func(id, role string, binding config.LoopBinding) {
		result[id] = state.SessionConfiguration{Harness: binding.Harness, Model: binding.Model, Provider: fallback.ResolveProvider(c.Config, binding.Harness, binding.Provider, binding.Channel), Channel: binding.Channel, Thinking: binding.Thinking, Permission: c.Config.PermissionMode(role, binding.Harness, id), SessionMode: c.Config.Sessions.ModeForRole(role)}
	}
	for role, binding := range c.Config.Loops {
		add("loops."+role, role, binding)
	}
	for route, binding := range c.Config.Routes {
		add("routes."+route, "execute", config.LoopBinding{Harness: binding.Harness, Model: binding.Model, Provider: binding.Provider, Channel: binding.Channel, Thinking: binding.Thinking})
	}
	if c.Config.JudgePanel != nil {
		add("judge-panel", "judge-panel", *c.Config.JudgePanel)
	}
	return result
}
