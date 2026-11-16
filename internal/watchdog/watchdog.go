// Package watchdog — 双计时看门狗（docs/design.md §6）：流 idle 超时（任何输出活动
// 即重置——长任务不误杀，#1911 类静默挂起 5 分钟内发现）+ 整体 wall-clock 兜底。
// kill 面向整个进程组，且必须等到真正退出（quiescence）才返回给调用者重试
// ——不给孤儿进程写坏工作区的机会。
//
// 在飞的子进程登记在册：编排器收到 SIGTERM（flywheel stop）时可杀光所有
// 在飞进程，同样等 quiescence 再自行退出。
package watchdog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// MaxBuffer 单流累计上限（超出后丢弃后续并置 truncated）。
var MaxBuffer = int64(16 * 1024 * 1024) // 16MB per stream

// Defaults 对齐 Node 版缺省。
const (
	DefaultIdleTimeoutMs = 300_000   // 流空闲超时
	DefaultWallTimeoutMs = 3_600_000 // 整体兜底
	DefaultKillGraceMs   = 5_000     // SIGTERM → SIGKILL 升级宽限
)

// Narrow clock/timer seams let tests deliver an already-queued idle callback
// after fresh output without depending on scheduler timing.
var (
	idleNow      = time.Now
	newIdleTimer = time.AfterFunc
)

// Result runWithWatchdog 结果。ExitCode 与 Signal 正交记录："超时被 trap 成
// exit 0"不得读成绿灯。
type Result struct {
	ExitCode     *int   `json:"exitCode"`
	Signal       string `json:"signal"`
	TimedOut     bool   `json:"timedOut"`
	IdleTimedOut bool   `json:"idleTimedOut"`
	WallTimedOut bool   `json:"wallTimedOut"`
	Cancelled    bool   `json:"cancelled"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	Truncated    bool   `json:"truncated"`
	DurationMS   int64  `json:"durationMs"`
	SpawnError   string `json:"spawnError,omitempty"`
	CleanupError string `json:"cleanupError,omitempty"`
}

// Opts Run 选项；零值字段取默认。
type Opts struct {
	Context       context.Context // nil = Background；取消与超时同样清理整个进程组
	Cwd           string
	Env           []string
	IdleTimeoutMs int64
	WallTimeoutMs int64
	KillGraceMs   int64
	// DisableIdleTimeout is for transports whose stdout/stderr cannot expose
	// ongoing work. Wall timeout, explicit cancellation and group cleanup stay
	// mandatory; silence alone cannot establish a hang on these transports.
	DisableIdleTimeout bool
	// OnActivity 每收到一段输出即回调（chunk, "stdout"/"stderr"）。
	OnActivity func(chunk string, stream string)
	// InteractStdin 可选交互钩子（协议驱动用，如 zcode app-server）：Start 后
	// 获得子进程 stdin 写入器与 finish 函数——driver 写请求帧、经 OnActivity
	// 解析事件流，会话结束后调用 finish() 走与超时同一条清理路径（SIGTERM →
	// grace → SIGKILL → quiescence；kill 开关契约不变）。finish 幂等，调用后
	// 不得再写 stdin。钩子在独立 goroutine 运行，Run 返回前不等待其退出。
	InteractStdin func(stdin io.WriteCloser, finish func())
}

// ---- 在飞子进程登记表（SIGTERM 全杀的依据）----

type activeChild struct {
	pid       int
	kill      func(sig syscall.Signal)
	killGrace time.Duration
	exited    chan struct{}
}

var (
	regMu    sync.Mutex
	registry = map[int]*activeChild{}
	// Real Start remains replaceable so tests can hold the parent after a
	// subprocess begins executing but before its registry entry is published.
	startCommand = (*exec.Cmd).Start
)

// A subprocess may execute before Cmd.Start returns. Hold the snapshot lock
// across startup and publication so KillActiveChildren cannot miss an already
// running child. Start does not wait for the output-copy callbacks, and the
// child kill callback is prepared before entering this critical section.
func startRegistered(cmd *exec.Cmd, child *activeChild) error {
	regMu.Lock()
	defer regMu.Unlock()
	if err := startCommand(cmd); err != nil {
		return err
	}
	child.pid = cmd.Process.Pid
	registry[child.pid] = child
	return nil
}

func unregister(pid int) {
	regMu.Lock()
	delete(registry, pid)
	regMu.Unlock()
}

// KillActiveChildren 杀光所有在飞子进程组并等待真退出（quiescence）。
// 编排器的 SIGTERM handler 在自行退出前调用。先全组 SIGTERM，再按每个
// 子进程自己的清理宽限与调用方宽限的较大值升级，不能截断 adapter 收尾。
func KillActiveChildren(grace time.Duration) {
	regMu.Lock()
	children := make([]*activeChild, 0, len(registry))
	for _, c := range registry {
		children = append(children, c)
	}
	regMu.Unlock()
	if len(children) == 0 {
		return
	}
	for _, c := range children {
		c.kill(syscall.SIGTERM)
	}
	timers := make([]*time.Timer, 0, len(children))
	for _, c := range children {
		// 只升级本次取消的进程；宽限期间启动的新调用不在快照中。
		// child.kill 在进程已收尾后为 no-op，避免向复用的 pid 发信号。
		timers = append(timers, time.AfterFunc(max(grace, c.killGrace), func() {
			c.kill(syscall.SIGKILL)
		}))
	}
	for _, c := range children {
		<-c.exited // Promise.allSettled 等价
	}
	for _, timer := range timers {
		timer.Stop()
	}
}

// killGroup 向负 pid 发信号（child 是组长，负 pid = 整个组）；组已消失则退回
// 直接 kill 单个进程。
func killGroup(pid int, sig syscall.Signal) {
	if err := syscall.Kill(-pid, sig); err != nil {
		_ = syscall.Kill(pid, sig) // already dead → ESRCH ignored
	}
}

// groupSignal 是清理失败的注入缝；生产使用真实进程组信号，调用者仍持有
// registry 条目直到清理结束。失败必须显式报告，不能将未知存活状态当成功。
var groupSignal = syscall.Kill

func cleanProcessGroup(pid int, grace time.Duration) error {
	if err := groupSignal(-pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("kill remaining process group: %w", err)
	}
	deadline := time.Now().Add(grace)
	for {
		err := groupSignal(-pid, 0)
		if err == syscall.ESRCH {
			return nil
		}
		// Darwin's group probe excludes zombies, so a killed group awaiting
		// reaping can return EPERM. Retry within the original grace without
		// treating permission errors as proof of quiescence: only ESRCH is.
		// https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/kern_sig.c#L1675-L1721
		expired := !time.Now().Before(deadline)
		if err != nil && (err != syscall.EPERM || expired) {
			return fmt.Errorf("inspect remaining process group: %w", err)
		}
		if expired {
			return fmt.Errorf("process group %d did not become quiescent within cleanup grace", pid)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// cappedBuffer 带上限的缓冲：达限后丢弃后续并置 truncated 标志。
type cappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	cap       int64
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if int64(b.buf.Len()) < b.cap {
		room := b.cap - int64(b.buf.Len())
		if int64(len(p)) <= room {
			b.buf.Write(p)
		} else {
			b.buf.Write(p[:room])
			b.truncated = true
		}
	} else {
		b.truncated = true
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *cappedBuffer) isTruncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// streamWriter io.Writer 适配：把每段输出路由到 cappedBuffer、重置 idle 计时
// 器并触发活动回调。由 exec 的内部复制 goroutine 调用（cmd.Stdout/Stderr），
// Wait() 保证复制完成——Node 'close' 语义由标准库托管复制保证。
type streamWriter struct {
	mu           *sync.Mutex
	buf          *cappedBuffer
	stream       string
	idleTimer    *time.Timer
	idleTimeout  time.Duration
	idleDeadline *time.Time
	idleTimedOut *bool
	readError    *error
	onActivity   func(chunk, stream string)
}

// ReadFrom 保留管道排空错误。exec.Cmd.Wait 对非零退出优先返回 ExitError，
// 会掩盖随后发生的 ErrWaitDelay；因此在复制端记录错误，不能把“exit 1 +
// 孙进程占住管道直到截断”当作完整且符合 expect exit 1 的验收。
func (w *streamWriter) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.Copy(struct{ io.Writer }{w}, r)
	if err != nil {
		w.mu.Lock()
		*w.readError = err
		w.mu.Unlock()
	}
	return n, err
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	if w.idleTimer != nil && !*w.idleTimedOut {
		*w.idleDeadline = idleNow().Add(w.idleTimeout)
		w.idleTimer.Reset(w.idleTimeout)
	}
	w.mu.Unlock()
	if w.onActivity != nil {
		w.onActivity(string(p), w.stream)
	}
	return len(p), nil
}

// Run 以双计时看门狗运行 command。阻塞到进程退出且 stdio 冲刷完毕
// （Node 'close' 语义：'exit' 可能在缓冲输出未排干时触发，'close' 保证流已
// 冲洗——尾部的 agent_settled/errorMessage 事件不会丢）。实现经 cmd.Stdout/
// Stderr 托管复制：Wait() 返回即复制完成，无手动排干竞态（review #3）。
func Run(command string, args []string, opts Opts) Result {
	started := time.Now()
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return Result{Cancelled: true}
	}
	idleTimeout := msDur(opts.IdleTimeoutMs, DefaultIdleTimeoutMs)
	wallTimeout := msDur(opts.WallTimeoutMs, DefaultWallTimeoutMs)
	killGrace := msDur(opts.KillGraceMs, DefaultKillGraceMs)

	res := Result{}

	cmd := exec.Command(command, args...)
	cmd.Dir = opts.Cwd
	cmd.Env = opts.Env
	cmd.Stdin = nil
	var stdinPipe io.WriteCloser
	if opts.InteractStdin != nil {
		// 协议驱动的双向通道；Start 前调用 StdinPipe 恒成功（exec 契约），
		// 失败分支不存在（覆盖率纪律：不可达防御分支不写）。
		stdinPipe, _ = cmd.StdinPipe()
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // 自成进程组 → 可整树 kill
	// 组长已退出而后代仍持有 stdio 时，不能无限等 exec 的复制 goroutine。
	// 超过宽限即关闭管道，随后清扫进程组；明确失败，不冒充完整输出。
	cmd.WaitDelay = killGrace

	var mu sync.Mutex // 保护 idle/kill 状态位
	var outputReadError error
	idleTimedOut, wallTimedOut := false, false
	cancelled, finished := false, false

	stdoutBuf := &cappedBuffer{cap: MaxBuffer}
	stderrBuf := &cappedBuffer{cap: MaxBuffer}
	stdoutW := &streamWriter{mu: &mu, buf: stdoutBuf, stream: "stdout",
		idleTimeout: idleTimeout, idleTimedOut: &idleTimedOut, readError: &outputReadError, onActivity: opts.OnActivity}
	stderrW := &streamWriter{mu: &mu, buf: stderrBuf, stream: "stderr",
		idleTimeout: idleTimeout, idleTimedOut: &idleTimedOut, readError: &outputReadError, onActivity: opts.OnActivity}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	child := &activeChild{
		killGrace: killGrace,
		exited:    make(chan struct{}),
	}
	sendSignal := func(sig syscall.Signal) {
		mu.Lock()
		if !finished {
			killGroup(child.pid, sig)
		}
		mu.Unlock()
	}
	markCancelled := func() {
		mu.Lock()
		if !finished {
			cancelled = true
		}
		mu.Unlock()
	}
	child.kill = func(sig syscall.Signal) {
		markCancelled()
		sendSignal(sig)
	}
	if err := startRegistered(cmd, child); err != nil {
		res.SpawnError = err.Error()
		res.DurationMS = time.Since(started).Milliseconds()
		return res
	}

	var stopOnce sync.Once
	beginKill := func() {
		stopOnce.Do(func() {
			sendSignal(syscall.SIGTERM)
			time.AfterFunc(killGrace, func() { sendSignal(syscall.SIGKILL) })
		})
	}

	var idleTimer *time.Timer
	if !opts.DisableIdleTimeout {
		// Reset cannot retract an AfterFunc callback already queued by the
		// runtime. Decide against the latest deadline under the same lock as
		// output, and rearm stale callbacks instead of killing renewed work.
		mu.Lock()
		idleDeadline := idleNow().Add(idleTimeout)
		idleTimer = newIdleTimer(idleTimeout, func() {
			mu.Lock()
			if finished || idleTimedOut {
				mu.Unlock()
				return
			}
			if remaining := idleDeadline.Sub(idleNow()); remaining > 0 {
				idleTimer.Reset(remaining)
				mu.Unlock()
				return
			}
			idleTimedOut = true
			mu.Unlock()
			beginKill()
		})
		stdoutW.idleTimer, stderrW.idleTimer = idleTimer, idleTimer
		stdoutW.idleDeadline, stderrW.idleDeadline = &idleDeadline, &idleDeadline
		mu.Unlock()
	}
	wallTimer := time.AfterFunc(wallTimeout, func() {
		mu.Lock()
		if !finished {
			wallTimedOut = true
		}
		mu.Unlock()
		beginKill()
	})
	go func() {
		select {
		case <-ctx.Done():
			markCancelled()
			beginKill()
		case <-child.exited:
		}
	}()

	if opts.InteractStdin != nil {
		// finish 与超时共用 beginKill 清理路径（kill 开关契约不变）；幂等。
		var once sync.Once
		finish := func() { once.Do(beginKill) }
		go opts.InteractStdin(stdinPipe, finish)
	}

	// Wait 返回 = 进程退出 + exec 内部复制 goroutine 排干 —— Node 'close'。
	if err := cmd.Wait(); errors.Is(err, exec.ErrWaitDelay) {
		res.CleanupError = "child output did not close within cleanup grace"
	}

	mu.Lock()
	finished = true
	idleFlag, wallFlag := idleTimedOut, wallTimedOut
	res.Cancelled = cancelled
	if outputReadError != nil {
		res.CleanupError = "child output did not close within cleanup grace: " + outputReadError.Error()
	}
	res.Stdout, res.Stderr = stdoutBuf.String(), stderrBuf.String()
	truncFlag := stdoutBuf.isTruncated() || stderrBuf.isTruncated()
	mu.Unlock()
	// 无论组长正常退出还是被取消，都不能遗留本次调用拥有的后台写入者。
	if err := cleanProcessGroup(child.pid, killGrace); err != nil {
		res.CleanupError = strings.TrimPrefix(res.CleanupError+"; "+err.Error(), "; ")
	}
	unregister(child.pid)
	if idleTimer != nil {
		idleTimer.Stop()
	}
	wallTimer.Stop()
	close(child.exited)

	res.ExitCode = exitCodeOf(cmd)
	res.Signal = signalOf(cmd)
	res.TimedOut = idleFlag || wallFlag
	res.IdleTimedOut = idleFlag
	res.WallTimedOut = wallFlag
	res.Truncated = truncFlag || res.CleanupError != ""
	res.DurationMS = time.Since(started).Milliseconds()
	return res
}

func msDur(v, def int64) time.Duration {
	if v <= 0 {
		return time.Duration(def) * time.Millisecond
	}
	return time.Duration(v) * time.Millisecond
}

func exitCodeOf(cmd *exec.Cmd) *int {
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if ok && ws.Signaled() {
		return nil // Node: 被 signal 杀死时 exitCode=null
	}
	code := cmd.ProcessState.ExitCode()
	return &code
}

func signalOf(cmd *exec.Cmd) string {
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if ok && ws.Signaled() {
		return fmt.Sprint(ws.Signal())
	}
	return ""
}
