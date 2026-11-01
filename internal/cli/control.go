package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

type controlResponse struct {
	Protocol int          `json:"protocol"`
	OK       bool         `json:"ok"`
	Running  bool         `json:"running"`
	PID      int          `json:"pid"`
	TaskDir  string       `json:"taskDir"`
	State    *state.State `json:"state"`
	Error    string       `json:"error,omitempty"`
}

var (
	controlExecutable   = os.Executable
	controlStat         = os.Stat
	controlStartWait    = 5 * time.Second
	controlPollInterval = 20 * time.Millisecond
)

// control is an argv-only, single-JSON boundary for the DSH plugin. Parse/flag
// failures are normalized by runRoot too; legacy commands keep their output.
func controlCmd() *cobra.Command {
	var taskDir, cwd string
	cmd := &cobra.Command{
		Use:   "control <start|status|stop>",
		Short: "桌面/脚本控制接口（protocol 1 JSON）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reply := controlResponse{Protocol: 1, TaskDir: taskDir}
			resolvedTask, err := controlDirectory(taskDir)
			var resolvedCwd string
			if err == nil {
				reply.TaskDir = resolvedTask
				resolvedCwd, err = controlDirectory(cwd)
			}
			if err == nil {
				switch args[0] {
				case "start":
					reply, err = startControlledTask(resolvedTask, resolvedCwd)
				case "status":
					reply, err = controlSnapshot(resolvedTask)
				case "stop":
					_, stopErr := stopTask(resolvedTask)
					var snapshotErr error
					reply, snapshotErr = controlSnapshot(resolvedTask)
					err = errors.Join(stopErr, snapshotErr)
				default:
					err = fmt.Errorf("control action must be start, status, or stop")
				}
			}
			reply.OK = err == nil
			if err != nil {
				reply.Error = err.Error()
			}
			writeControlJSON(cmd.OutOrStdout(), reply)
			if err != nil {
				return &exitError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&taskDir, "task-dir", "", "任务目录的绝对路径")
	cmd.Flags().StringVar(&cwd, "cwd", "", "仓库目录的绝对路径")
	return cmd
}

func writeControlJSON(out io.Writer, response controlResponse) {
	// Fixed state types contain no unsupported JSON values; stdout errors follow
	// the same CLI boundary as printJSON (the caller has already gone away).
	raw, _ := json.Marshal(response)
	fmt.Fprintf(out, "%s\n", raw)
}

func controlTaskArg(args []string) string {
	value := ""
	for i, arg := range args {
		if strings.HasPrefix(arg, "--task-dir=") {
			value = strings.TrimPrefix(arg, "--task-dir=")
		} else if arg == "--task-dir" && i+1 < len(args) {
			value = args[i+1]
		}
	}
	return value
}

func controlDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("control paths must be absolute: %q", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := controlStat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("control path is not a directory: %s", resolved)
	}
	return resolved, nil
}

// controlSnapshot never creates locks or state. The lifetime lease determines
// running; a stale or reused PID cannot make another process look like this task.
func controlSnapshot(taskDir string) (controlResponse, error) {
	reply := controlResponse{Protocol: 1, TaskDir: taskDir}
	held, err := machine.RunLeaseHeld(taskDir)
	if err != nil {
		return reply, err
	}
	reply.Running = held
	if held {
		control, readErr := machine.ReadRunControl(taskDir)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return reply, readErr
		}
		reply.PID = control.PID // zero while the lease owner is still starting
	}
	if state.Exists(taskDir) {
		reply.State, err = state.Load(taskDir)
		return reply, err
	}
	return reply, nil
}

func startControlledTask(taskDir, cwd string) (controlResponse, error) {
	reply, err := controlSnapshot(taskDir)
	if err != nil || reply.Running {
		return reply, err
	}
	executable, err := controlExecutable()
	if err != nil {
		return reply, err
	}
	log, err := os.OpenFile(filepath.Join(taskDir, "orchestrator.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return reply, err
	}
	defer log.Close()
	child := exec.Command(executable, "run", taskDir)
	child.Dir = cwd
	child.Stdin = nil
	child.Stdout, child.Stderr = log, log
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return reply, err
	}
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	timer := time.NewTimer(controlStartWait)
	defer timer.Stop()
	ticker := time.NewTicker(controlPollInterval)
	defer ticker.Stop()
	for {
		select {
		case exitErr := <-exited:
			reply, snapshotErr := controlSnapshot(taskDir)
			if snapshotErr != nil {
				return reply, snapshotErr
			}
			if exitErr != nil && !reply.Running {
				var semanticExit *exec.ExitError
				if errors.As(exitErr, &semanticExit) && (semanticExit.ExitCode() == 3 || semanticExit.ExitCode() == 143) && reply.State != nil {
					return reply, nil // a started task may finish escalated or interrupted
				}
				return reply, fmt.Errorf("orchestrator exited before readiness: %w; inspect orchestrator.log", exitErr)
			}
			return reply, nil
		case <-ticker.C:
			reply, err = controlSnapshot(taskDir)
			if err != nil {
				return reply, err
			}
			if reply.Running && reply.PID > 0 {
				return reply, nil
			}
		case <-timer.C:
			// Start admission is uncertain, not cancelled: the detached engine may
			// still finish loading its contract. Never launch another run here.
			reply.Running, reply.PID = true, child.Process.Pid
			return reply, fmt.Errorf("orchestrator pid %d started but readiness was not confirmed; inspect orchestrator.log and refresh status", child.Process.Pid)
		}
	}
}
