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

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

type controlResponse struct {
	Protocol      int                       `json:"protocol"`
	OK            bool                      `json:"ok"`
	Running       bool                      `json:"running"`
	PID           int                       `json:"pid"`
	TaskDir       string                    `json:"taskDir"`
	Cwd           string                    `json:"cwd,omitempty"`
	State         *state.State              `json:"state"`
	Error         string                    `json:"error,omitempty"`
	Configuration *diagnostic.Configuration `json:"configuration,omitempty"`
	Diagnostic    *diagnostic.Report        `json:"diagnostic,omitempty"`
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
	var taskDir, cwd, mode string
	var probe bool
	cmd := &cobra.Command{
		Use:   "control <launch|start|status|stop|config|doctor>",
		Short: "桌面/脚本控制接口（protocol 1 JSON）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reply := controlResponse{Protocol: 1, TaskDir: taskDir}
			action := args[0]
			var resolvedTask, resolvedCwd string
			var err error
			if cmd.Flags().Changed("mode") && action != "launch" && action != "start" {
				err = fmt.Errorf("--mode is only valid for control launch or start")
			} else if cmd.Flags().Changed("mode") && !contract.ValidWorkflowMode(mode) {
				err = fmt.Errorf("--mode only supports auto, quick, or full")
			} else if cmd.Flags().Changed("probe") && action != "doctor" {
				err = fmt.Errorf("--probe is only valid for control doctor")
			} else if taskDir != "" || (action != "config" && action != "doctor") {
				resolvedTask, err = controlDirectory(taskDir)
			}
			if err == nil {
				reply.TaskDir = resolvedTask
				resolvedCwd, err = controlDirectory(cwd)
			}
			if err == nil {
				switch action {
				case "launch", "start":
					requested := mode
					if action == "launch" && requested == "" {
						requested = "auto"
					}
					reply, err = startControlledTask(resolvedTask, resolvedCwd, requested)
				case "status":
					reply, err = controlSnapshot(resolvedTask)
				case "stop":
					_, stopErr := stopTask(resolvedTask)
					var snapshotErr error
					reply, snapshotErr = controlSnapshot(resolvedTask)
					err = errors.Join(stopErr, snapshotErr)
				case "config":
					ctx, cancel := diagnosticContext(cmd)
					defer cancel()
					reply.Configuration, err = diagnostic.InspectContext(ctx, resolvedCwd, resolvedTask)
				case "doctor":
					ctx, cancel := diagnosticContext(cmd)
					defer cancel()
					report := diagnostic.Doctor(ctx, resolvedCwd, resolvedTask, probe)
					reply.Diagnostic = &report
					if !report.OK {
						err = fmt.Errorf("preflight found blocking checks; inspect diagnostic.checks")
					}
				default:
					err = fmt.Errorf("control action must be launch, start, status, stop, config, or doctor")
				}
			}
			if resolvedCwd != "" {
				reply.Cwd = resolvedCwd
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
	cmd.Flags().StringVar(&taskDir, "task-dir", "", "任务目录的绝对路径（config/doctor 可省略）")
	cmd.Flags().StringVar(&cwd, "cwd", "", "仓库目录的绝对路径")
	cmd.Flags().BoolVar(&probe, "probe", false, "仅 doctor：探测 DSH SDK 配置与权限，不发送推理请求")
	cmd.Flags().StringVar(&mode, "mode", "", "仅 launch/start：auto|quick|full；launch 默认 auto，恢复沿用已保存策略")
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

func startControlledTask(taskDir, cwd string, modes ...string) (controlResponse, error) {
	reply, err := controlSnapshot(taskDir)
	if err != nil {
		return reply, err
	}
	if reply.Running {
		if len(modes) > 0 && modes[0] != "" && modes[0] != "auto" {
			if reply.State == nil {
				return reply, fmt.Errorf("task is already starting; refresh status before requesting a workflow change")
			}
			requested, effective := "full", "full"
			if reply.State.Workflow != nil {
				requested, effective = reply.State.Workflow.Requested, reply.State.Workflow.Effective
			}
			if modes[0] != requested && modes[0] != effective {
				return reply, fmt.Errorf("task is already running with %s workflow; stop it before requesting a workflow change", effective)
			}
		}
		return reply, nil
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
	args := []string{"run", taskDir}
	if len(modes) > 0 && modes[0] != "" {
		args = append(args, "--mode", modes[0])
	}
	child := exec.Command(executable, args...)
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
