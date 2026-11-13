package investigation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ckanner/flywheel/internal/childenv"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/ckanner/flywheel/internal/watchdog"
)

var experimentTempDir = os.MkdirTemp
var experimentMkdir = os.MkdirAll
var experimentWrite = os.WriteFile
var experimentRemove = os.RemoveAll
var runExperimentProcess = watchdog.Run

type actionResult struct {
	Output      string
	ContentHash string
	Truncated   bool
	Execution   *state.InvestigationExecution
	Unknown     bool
	Err         error
}

func (o Options) lookup(name string) (string, bool) {
	if o.LookupEnv != nil {
		return o.LookupEnv(name)
	}
	return os.LookupEnv(name)
}

func boundedOutput(value string, limit int) (string, bool) {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func redactExperiment(value string, secrets []string) string {
	// Replace longer overlapping values first, without ever persisting them.
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
			// A bounded stream may end partway through a credential. Remove any
			// trailing prefix as well; never persist a cut-off secret as evidence.
			for size := len(secret) - 1; size > 0; size-- {
				if strings.HasSuffix(value, secret[:size]) {
					value = value[:len(value)-size] + "[REDACTED]"
					break
				}
			}
		}
	}
	return value
}

func performAction(ctx context.Context, cwd string, action contract.InvestigationAction, limits contract.InvestigationLimits, remaining int, o Options) actionResult {
	if action.Kind == "run_command" {
		return runCommand(ctx, cwd, action, limits, remaining, o)
	}
	if action.Kind == "http_read" {
		return readHTTP(ctx, action, remaining, o)
	}
	output, contentHash, truncated, err := executeAction(ctx, cwd, action, limits, remaining)
	return actionResult{Output: output, ContentHash: contentHash, Truncated: truncated, Err: err}
}

// Copying inputs prevents accidental relative writes to the repository. The
// approved command remains a host process with OS-level capabilities: it is
// not a security sandbox and never receives an implicit shell or credentials.
func runCommand(ctx context.Context, cwd string, action contract.InvestigationAction, limits contract.InvestigationLimits, remaining int, o Options) (result actionResult) {
	e := &state.InvestigationExecution{Kind: "run_command", Cleanup: "not-created"}
	result.Execution = e
	workspace, err := experimentTempDir("", "flywheel-investigation-")
	if err != nil {
		result.Err = fmt.Errorf("create experiment copy: %w", err)
		return
	}
	e.Workspace, e.Cleanup = workspace, "retained"
	defer func() {
		if result.Unknown {
			return
		}
		if err := experimentRemove(workspace); err != nil {
			result.Unknown, result.Err = true, fmt.Errorf("experiment cleanup failed; workspace retained")
			return
		}
		e.Cleanup = "removed"
	}()
	inputs := map[string]string{}
	for _, path := range action.Command.Inputs {
		raw, directory, mode, err := readPathWithMode(ctx, cwd, path, limits.MaxFileBytes)
		if err != nil {
			result.Err = err
			return
		}
		if directory {
			result.Err = fmt.Errorf("command inputs require explicit regular files")
			return
		}
		target := filepath.Join(workspace, path)
		if err := experimentMkdir(filepath.Dir(target), 0o700); err != nil {
			result.Err = fmt.Errorf("prepare experiment input: %w", err)
			return
		}
		if err := experimentWrite(target, raw, 0o600|(mode&0o111)); err != nil {
			result.Err = fmt.Errorf("copy experiment input: %w", err)
			return
		}
		inputs[path] = fmt.Sprintf("directory=false:%s:mode=%o", digest(raw), mode)
	}
	e.InputHash = state.PreparationHash(inputs)
	secrets := []string{}
	env := childenv.GateEnv(o.lookup, action.Command.Env)
	for _, name := range action.Command.Env {
		value, ok := env[name]
		if !ok {
			result.Err = fmt.Errorf("approved command environment variable is unavailable: %s", name)
			return
		}
		secrets = append(secrets, value)
	}
	// Keep the normal child whitelist but give language runtimes private scratch
	// locations. Credentials and user profiles are not copied into this tree.
	env["HOME"], env["TMPDIR"] = workspace, workspace
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		env[key] = workspace
	}
	before := &state.PreparationState{Cwd: cwd, Contract: contract.Preparation{ReadPaths: action.Command.Inputs, Actions: []contract.InvestigationAction{action}, Limits: limits}}
	identity, err := candidate(ctx, before)
	if err != nil {
		result.Err = err
		return
	}
	if identity != e.InputHash {
		result.Err = fmt.Errorf("approved source inputs changed while copying experiment")
		return
	}
	childCtx, cancel := context.WithTimeout(ctx, time.Duration(action.Command.TimeoutMS)*time.Millisecond)
	defer cancel()
	var outputMu sync.Mutex
	bytesSeen, exceeded := 0, false
	r := runExperimentProcess(action.Command.Argv[0], action.Command.Argv[1:], watchdog.Opts{Context: childCtx, Cwd: workspace, Env: childenv.EnvSlice(env), WallTimeoutMs: action.Command.TimeoutMS, IdleTimeoutMs: action.Command.TimeoutMS, KillGraceMs: 1000, OnActivity: func(chunk, _ string) {
		outputMu.Lock()
		defer outputMu.Unlock()
		bytesSeen += len(chunk)
		if bytesSeen > remaining {
			exceeded = true
			cancel()
		}
	}})
	e.ExitCode, e.Signal, e.Cancelled = r.ExitCode, r.Signal, r.Cancelled || ctx.Err() == context.Canceled
	e.TimedOut = r.TimedOut || childCtx.Err() == context.DeadlineExceeded
	redacted := redactExperiment(r.Stdout, secrets) + redactExperiment(r.Stderr, secrets)
	result.Output, result.Truncated = boundedOutput(redacted, remaining)
	result.Truncated = result.Truncated || r.Truncated || exceeded
	result.ContentHash = digest([]byte(redacted))
	if r.CleanupError != "" {
		result.Unknown, result.Err = true, fmt.Errorf("experiment process cleanup is unknown; workspace retained")
		return
	}
	if r.SpawnError != "" || r.ExitCode == nil || *r.ExitCode != action.Command.ExpectExit || r.Signal != "" || e.TimedOut || e.Cancelled || result.Truncated {
		result.Err = fmt.Errorf("experiment did not finish with its approved physical result")
		return
	}
	after, err := candidate(ctx, before)
	if err != nil || after != identity {
		result.Err = fmt.Errorf("approved source inputs changed during experiment")
		return
	}
	return
}
