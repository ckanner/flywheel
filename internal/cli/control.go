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

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/diagnostic"
	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/metrics"
	"github.com/ckanner/flywheel/internal/modelcatalog"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/spf13/cobra"
)

type controlResponse struct {
	ModelCatalog    *modelcatalog.Report            `json:"modelCatalog,omitempty"`
	InputImages     *imageinput.Inspection          `json:"inputImages,omitempty"`
	PlanningControl *machine.PlanningControlReceipt `json:"planning-control,omitempty"`
	SessionControl  *SessionControlReceipt          `json:"session-control,omitempty"`
	PacketControl   *PacketControlReceipt           `json:"packet-control,omitempty"`
	TaskTitle       string                          `json:"taskTitle,omitempty"`
	Preparation     *state.PreparationState         `json:"preparation,omitempty"`
	Projection      string                          `json:"projection,omitempty"`
	Node            *machine.NodeDetail             `json:"node,omitempty"`
	Delivery        *machine.DeliverySnapshot       `json:"delivery,omitempty"`
	History         *machine.SessionHistory         `json:"history,omitempty"`
	Metrics         *metrics.Report                 `json:"metrics,omitempty"`
	Runs            []machine.RunRecord             `json:"runs"`
	Protocol        int                             `json:"protocol"`
	OK              bool                            `json:"ok"`
	Running         bool                            `json:"running"`
	PID             int                             `json:"pid"`
	TaskDir         string                          `json:"taskDir"`
	Cwd             string                          `json:"cwd,omitempty"`
	State           *state.State                    `json:"state"`
	Sessions        []sessionSummary                `json:"sessions,omitempty"`
	Error           string                          `json:"error,omitempty"`
	Configuration   *diagnostic.Configuration       `json:"configuration,omitempty"`
	Diagnostic      *diagnostic.Report              `json:"diagnostic,omitempty"`
	Editor          *config.EditorDocument          `json:"editor,omitempty"`
}

// SessionControlReceipt makes an asynchronous session command's semantic
// outcome explicit. The control envelope may still be ok:true; applied=false
// means no requested transition was confirmed. Message distinguishes a refusal
// or unchanged record from an operation whose effect could not be verified.
type SessionControlReceipt struct {
	Key        string `json:"key"`
	Action     string `json:"action"`
	Applied    bool   `json:"applied"`
	Revision   int64  `json:"revision"`
	Status     string `json:"status,omitempty"`
	Invocation string `json:"invocation,omitempty"`
	Message    string `json:"message,omitempty"`
}

func sessionControlReceipt(st, before *state.State, request *machine.SessionCommand) *SessionControlReceipt {
	receipt := &SessionControlReceipt{Key: request.Key, Action: request.Action}
	if st == nil {
		receipt.Message = "task state is unavailable"
		return receipt
	}
	receipt.Revision = st.Revision
	if st.Sessions == nil || st.Sessions.Records[request.Key] == nil || st.Sessions.Records[request.Key].Last == nil {
		receipt.Message = "session key not found"
		return receipt
	}
	record := st.Sessions.Records[request.Key]
	receipt.Status, receipt.Invocation = record.Last.Status, record.Last.ID
	var beforeRecord *state.SessionRecord
	if before != nil && before.Sessions != nil {
		beforeRecord = before.Sessions.Records[request.Key]
	}
	if beforeRecord == nil || beforeRecord.Last == nil || beforeRecord.Last.ID == "" {
		// The child may have executed even when the earlier snapshot failed.
		// Preserve the observed after-state, but do not infer this command's
		// success from a postcondition that could already have been true.
		receipt.Message = "the session command's effect could not be verified because the pre-command invocation is unavailable; it may already have executed; refresh status before another mutation"
		return receipt
	}
	beforeInvocation, beforeStatus, beforeHandoff := beforeRecord.Last.ID, beforeRecord.Last.Status, ""
	if before.Handoff != nil {
		beforeHandoff = before.Handoff.Key
	}
	switch request.Action {
	case "abandon":
		receipt.Applied = record.Last.Status == "abandoned" && beforeStatus != "abandoned" &&
			record.Last.Abandon != nil && record.Last.Abandon.Reason == request.Reason
		if !receipt.Applied {
			receipt.Message = record.Reason
			if receipt.Message == "" {
				receipt.Message = "the unresolved invocation was not retired"
			}
		}
	case "reconcile":
		// Local cancellation may consume a no-dispatch receipt while leaving the
		// manual handoff pending. Reconcile then settles the handoff without
		// changing the invocation's already-terminal status.
		pendingCleared := before.Handoff != nil && beforeHandoff == request.Key && before.Handoff.Pending &&
			st.Handoff != nil && st.Handoff.Key == request.Key && st.Handoff.Thread == before.Handoff.Thread && !st.Handoff.Pending &&
			record.Last.ID == beforeInvocation
		receipt.Applied = record.Last.Status == "consumed" && record.Last.CleanupConfirmed && (beforeStatus != "consumed" || pendingCleared)
		receipt.Message = record.Reason
	case "claim":
		receipt.Applied = st.Handoff != nil && st.Handoff.Key == request.Key && beforeHandoff != request.Key
		if !receipt.Applied {
			receipt.Message = "the session was not newly claimed"
		}
	case "release":
		receipt.Applied = beforeHandoff == request.Key && st.Handoff == nil
		if !receipt.Applied {
			receipt.Message = "the session was not under this handoff or remains controlled"
		}
	case "send":
		receipt.Applied = record.Last.ID != beforeInvocation
		if !receipt.Applied {
			receipt.Message = "the session command has not started a new invocation yet; refresh status"
		}
	case "return":
		receipt.Applied = record.Last.Status == "consumed" && record.Last.CleanupConfirmed &&
			beforeHandoff == request.Key && st.Handoff == nil
		receipt.Message = record.Reason
	default:
		receipt.Message = "unsupported session action"
	}
	return receipt
}

func controlEditor(cmd *cobra.Command, cwd, action string) (*config.EditorDocument, error) {
	var request config.EditorRequest
	raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, fmt.Errorf("configuration request exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || !json.Valid(raw) {
		return nil, fmt.Errorf("invalid configuration editor request")
	}
	root := machine.ProjectRoot(cwd)
	if action == "config-edit" {
		return config.ReadEditor(root, request.Scope)
	}
	return config.SaveEditor(root, request)
}

var (
	controlExecutable   = os.Executable
	controlStat         = os.Stat
	controlStartWait    = 5 * time.Second
	controlPollInterval = 20 * time.Millisecond
	// Injection seam for the wait-for-session-exit path; production reads the
	// same snapshot used by status.
	controlSnapshotFn = controlSnapshot
)

// control is an argv-only, single-JSON boundary for the DSH plugin. Parse/flag
// failures are normalized by runRoot too; legacy commands keep their output.
func controlCmd() *cobra.Command {
	var taskDir, cwd, mode, only, packet, retryReason, sessionKey, runID string
	var revision int64
	var probe, compact bool
	cmd := &cobra.Command{
		Use:   "control <planning|launch|start|step|status|stop|investigate|node|runs|metrics|history|session|packet|delivery|deliver|config|config-edit|config-save|doctor|images|models>",
		Short: "桌面/脚本控制接口（protocol 1 JSON）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reply := controlResponse{Protocol: 1, TaskDir: taskDir}
			action := args[0]
			var resolvedTask, resolvedCwd string
			var err error
			if cmd.Flags().Changed("run-id") && action != "metrics" {
				err = fmt.Errorf("--run-id requires metrics")
			} else if cmd.Flags().Changed("session-key") && action != "history" {
				err = fmt.Errorf("--session-key requires history")
			} else if cmd.Flags().Changed("retry-reason") && (action != "step" || only != "execute" || strings.TrimSpace(retryReason) == "" || len(retryReason) > 1000) {
				err = fmt.Errorf("retry reason requires Execute and 1–1000 bytes of explanation")
			} else if action == "step" && (!contains(onlySteps, only) || revision < 1) {
				err = fmt.Errorf("step requires --only plan|execute|review|verify|integrate|final and a positive --expect-revision")
			} else if action != "step" && (cmd.Flags().Changed("only") || (cmd.Flags().Changed("packet") && action != "node") || cmd.Flags().Changed("expect-revision")) {
				err = fmt.Errorf("--only, --packet and --expect-revision require control step")
			} else if cmd.Flags().Changed("mode") && action != "launch" && action != "start" {
				err = fmt.Errorf("--mode is only valid for control launch or start")
			} else if cmd.Flags().Changed("mode") && !contract.ValidWorkflowMode(mode) {
				err = fmt.Errorf("--mode only supports auto, quick, or full")
			} else if cmd.Flags().Changed("probe") && action != "doctor" {
				err = fmt.Errorf("--probe is only valid for control doctor")
			} else if taskDir != "" || (action != "config" && action != "doctor" && action != "config-edit" && action != "config-save" && action != "runs" && action != "images" && action != "models") {
				if action == "planning" || action == "investigate" {
					resolvedTask, err = planningControlDirectory(taskDir)
				} else {
					resolvedTask, err = controlDirectory(taskDir)
				}
			}
			if err == nil {
				reply.TaskDir = resolvedTask
				resolvedCwd, err = controlDirectory(cwd)
			}
			if err == nil {
				switch action {
				case "models":
					reply.ModelCatalog, err = controlModels(cmd, resolvedCwd)
				case "images":
					reply.InputImages, err = controlImages(cmd, resolvedCwd)
				case "planning":
					reply, err = controlPlanning(cmd, resolvedTask, resolvedCwd)
				case "investigate":
					reply, err = controlInvestigation(cmd, resolvedTask, resolvedCwd)
				case "delivery", "deliver":
					var request machine.DeliveryRequest
					raw, readErr := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 131073))
					if readErr != nil {
						err = readErr
						break
					}
					decoder := json.NewDecoder(strings.NewReader(string(raw)))
					decoder.DisallowUnknownFields()
					if len(raw) > 131072 || !json.Valid(raw) {
						err = fmt.Errorf("invalid delivery request")
						break
					}
					if err = decoder.Decode(&request); err != nil {
						break
					}
					ctx, cancel := diagnosticContext(cmd)
					defer cancel()
					if action == "delivery" && request.Action != "inspect" || action == "deliver" && request.Action == "inspect" {
						err = fmt.Errorf("delivery is read-only; use deliver for a confirmed write")
						break
					}
					if request.Action == "inspect" {
						reply.Delivery, err = inspectDeliveryFn(ctx, resolvedCwd, request.HeadBranch)
						if err == nil {
							reply.Delivery.Endpoint = deliveryEndpoint(resolvedTask, resolvedCwd)
						}
					} else {
						cfg, configErr := config.Load(resolvedTask, resolvedCwd)
						if configErr != nil {
							err = configErr
							break
						}
						reply.Delivery, err = machine.Deliver(machine.RunOpts{TaskDir: resolvedTask, Cwd: resolvedCwd, Config: cfg, Context: ctx, Log: logf(cmd)}, request)
						delivery := reply.Delivery
						snapshot, snapshotErr := controlSnapshot(resolvedTask)
						reply = snapshot
						reply.Delivery = delivery
						err = errors.Join(err, snapshotErr)
					}
				case "session":
					reply, err = startSessionControl(cmd, resolvedTask, resolvedCwd)
				case "packet":
					reply, err = startPacketControl(cmd, resolvedTask, resolvedCwd)
				case "history":
					reply.History, err = machine.ReadSessionHistory(resolvedTask, sessionKey)
				case "metrics":
					reply.Metrics, err = metrics.ReadReport(filepath.Join(resolvedTask, "metrics.jsonl"), runID)
				case "runs":
					directory, indexErr := machine.RunIndexDirectory()
					if indexErr != nil {
						err = indexErr
					} else {
						reply.Runs, err = machine.ListRuns(directory)
					}
				case "step":
					extra := []string{"--only", only, "--expect-revision", fmt.Sprint(revision)}
					if packet != "" {
						extra = append(extra, "--packet", packet)
					}
					if retryReason != "" {
						extra = append(extra, "--retry-reason", retryReason)
					}
					reply, err = startControlledRun(resolvedTask, resolvedCwd, "", extra)
				case "config-edit", "config-save":
					reply.Editor, err = controlEditor(cmd, resolvedCwd, action)
				case "launch", "start":
					requested := mode
					if action == "launch" && requested == "" {
						requested = "auto"
					}
					reply, err = startControlledTask(resolvedTask, resolvedCwd, requested)
				case "node":
					reply, err = controlSnapshot(resolvedTask)
					if err == nil {
						reply.Node, err = machine.ReadNode(reply.State, packet, resolvedTask, resolvedCwd)
					}
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
					err = fmt.Errorf("control action must be planning, launch, start, step, status, stop, investigate, node, runs, metrics, history, session, packet, delivery, deliver, config, config-edit, config-save, or doctor")
				}
			}
			if resolvedCwd != "" {
				reply.Cwd = resolvedCwd
			}
			if compact && reply.State != nil {
				var projectionErr error
				reply.State, projectionErr = machine.ConsoleProjection(reply.State, reply.Node)
				err = errors.Join(err, projectionErr)
				reply.Projection = "console-excerpt"
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
	cmd.Flags().BoolVar(&compact, "compact", false, "有界只读画布投影；完整诊断可用 node 或省略此选项读取")
	cmd.Flags().StringVar(&runID, "run-id", "", "仅 metrics：按准确的引擎 run ID 筛选")
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "仅 history：准确的角色／作用域键")
	cmd.Flags().StringVar(&taskDir, "task-dir", "", "任务目录的绝对路径（config/doctor 可省略）")
	cmd.Flags().StringVar(&cwd, "cwd", "", "仓库目录的绝对路径")
	cmd.Flags().BoolVar(&probe, "probe", false, "仅 doctor：探测 DSH SDK 配置与权限，不发送推理请求")
	cmd.Flags().StringVar(&mode, "mode", "", "仅 launch/start：auto|quick|full；launch 默认 auto，恢复沿用已保存策略")
	cmd.Flags().StringVar(&only, "only", "", "仅 step：运行 plan|execute|review|verify|integrate|final")
	cmd.Flags().StringVar(&packet, "packet", "", "仅 step：packet id")
	cmd.Flags().Int64Var(&revision, "expect-revision", 0, "仅 step：已查看的状态 revision")
	cmd.Flags().StringVar(&retryReason, "retry-reason", "", "仅 step execute：人工继续一次的原因")
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
	// Optional display metadata; an invalid or missing Spec must not hide
	// lifecycle/cleanup evidence or prevent stopping a damaged task.
	if spec, specErr := contract.LoadSpec(taskDir); specErr == nil {
		reply.TaskTitle = spec.Title
	}
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
		reply.Sessions = sessionSummaries(reply.State)
		if reply.State != nil && reply.State.Planning != nil {
			reply.TaskTitle = reply.State.Planning.Brief.Goal
		}
		if preparation, prepErr := state.LoadPreparation(taskDir); prepErr == nil {
			reply.Preparation = preparation
		}
		return reply, err
	}
	if preparation, preparationErr := state.LoadPreparation(taskDir); !os.IsNotExist(preparationErr) {
		reply.Preparation = preparation
		return reply, preparationErr
	}
	return reply, nil
}

func startControlledTask(taskDir, cwd string, modes ...string) (controlResponse, error) {
	mode := ""
	if len(modes) > 0 {
		mode = modes[0]
	}
	return startControlledRun(taskDir, cwd, mode, nil)
}

func startControlledRun(taskDir, cwd, mode string, extra []string) (controlResponse, error) {
	return startControlledRunWait(taskDir, cwd, mode, extra, false)
}

func startControlledRunWait(taskDir, cwd, mode string, extra []string, waitForExit bool) (controlResponse, error) {
	reply, err := controlSnapshotFn(taskDir)
	if err != nil {
		return reply, err
	}
	if reply.Running {
		if len(extra) > 0 {
			return reply, fmt.Errorf("task is already running; wait for its terminal result before starting a stage")
		}
		if mode != "" && mode != "auto" {
			if reply.State == nil {
				return reply, fmt.Errorf("task is already starting; refresh status before requesting a workflow change")
			}
			requested, effective := "full", "full"
			if reply.State.Workflow != nil {
				requested, effective = reply.State.Workflow.Requested, reply.State.Workflow.Effective
			}
			if mode != requested && mode != effective {
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
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	args = append(args, extra...)
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
			reply, snapshotErr := controlSnapshotFn(taskDir)
			if snapshotErr != nil {
				return reply, snapshotErr
			}
			if exitErr != nil && (!reply.Running || len(extra) > 0) {
				var semanticExit *exec.ExitError
				if errors.As(exitErr, &semanticExit) && (semanticExit.ExitCode() == 3 || semanticExit.ExitCode() == 143) && (reply.State != nil || reply.Preparation != nil) && (len(extra) == 0 || !reply.Running || reply.PID == child.Process.Pid) {
					return reply, nil // a started task may finish escalated or interrupted
				}
				return reply, fmt.Errorf("orchestrator exited before readiness: %w; inspect orchestrator.log", exitErr)
			}
			return reply, nil
		case <-ticker.C:
			reply, err = controlSnapshotFn(taskDir)
			if err != nil {
				return reply, err
			}
			if reply.Running && reply.PID > 0 {
				if len(extra) > 0 && reply.PID != child.Process.Pid {
					continue // Another window owns the run; await this child's admission result.
				}
				if waitForExit {
					continue // Session abandon/reconcile/claim/release must settle before its receipt.
				}
				return reply, nil
			}
		case <-timer.C:
			// Start admission is uncertain, not cancelled: the detached engine may
			// still finish loading its contract. Never launch another run here.
			reply.Running, reply.PID = true, child.Process.Pid
			if waitForExit {
				return reply, fmt.Errorf("orchestrator pid %d started but did not finish the session command; refresh status before another mutation", child.Process.Pid)
			}
			return reply, fmt.Errorf("orchestrator pid %d started but readiness was not confirmed; inspect orchestrator.log and refresh status", child.Process.Pid)
		}
	}
}

var inspectDeliveryFn = machine.InspectDelivery

func deliveryEndpoint(taskDir, cwd string) string {
	cfg, err := config.LoadReadOnly(taskDir, cwd)
	if err != nil {
		return "local"
	}
	return cfg.Delivery.Endpoint
}
