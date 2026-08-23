// Package watchdog — 双计时看门狗（DESIGN.md §6）：流 idle 超时（任何输出活动
// 即重置——长任务不误杀，#1911 类静默挂起 5 分钟内发现）+ 整体 wall-clock 兜底。
// kill 面向整个进程组，且必须等到真正退出（quiescence）才返回给调用者重试
// ——不给孤儿进程写坏工作区的机会。
//
// 在飞的子进程登记在册：编排器收到 SIGTERM（flywheel stop）时可杀光所有
// 在飞进程，同样等 quiescence 再自行退出。
package watchdog

import (
	"fmt"
	"io"
	"os/exec"
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

// Result runWithWatchdog 结果。ExitCode 与 Signal 正交记录："超时被 trap 成
// exit 0"不得读成绿灯。
type Result struct {
	ExitCode     *int   `json:"exitCode"`
	Signal       string `json:"signal"`
	TimedOut     bool   `json:"timedOut"`
	IdleTimedOut bool   `json:"idleTimedOut"`
	WallTimedOut bool   `json:"wallTimedOut"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	Truncated    bool   `json:"truncated"`
	DurationMS   int64  `json:"durationMs"`
	SpawnError   string `json:"spawnError,omitempty"`
}

// Opts Run 选项；零值字段取默认。
type Opts struct {
	Cwd           string
	Env           []string
	IdleTimeoutMs int64
	WallTimeoutMs int64
	KillGraceMs   int64
	// OnActivity 每收到一段输出即回调（chunk, "stdout"/"stderr"）。
	OnActivity func(chunk string, stream string)
}

// ---- 在飞子进程登记表（SIGTERM 全杀的依据）----

type activeChild struct {
	pid    int
	kill   func(sig syscall.Signal)
	exited chan struct{}
}

var (
	regMu    sync.Mutex
	registry = map[int]*activeChild{}
)

func register(c *activeChild) {
	regMu.Lock()
	registry[c.pid] = c
	regMu.Unlock()
}

func unregister(pid int) {
	regMu.Lock()
	delete(registry, pid)
	regMu.Unlock()
}

// KillActiveChildren 杀光所有在飞子进程组并等待真退出（quiescence）。
// 编排器的 SIGTERM handler 在自行退出前调用。先全组 SIGTERM，grace 后对仍
// 未退出者升级 SIGKILL，最后等到全部退出。
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
	timer := time.AfterFunc(grace, func() {
		// 对 trap 了 SIGTERM 的进程的最后手段；registry 已空时循环零次。
		regMu.Lock()
		live := make([]*activeChild, 0, len(registry))
		for _, c := range registry {
			live = append(live, c)
		}
		regMu.Unlock()
		for _, c := range live {
			c.kill(syscall.SIGKILL)
		}
	})
	for _, c := range children {
		<-c.exited // Promise.allSettled 等价
	}
	timer.Stop()
}

// killGroup 向负 pid 发信号（child 是组长，负 pid = 整个组）；组已消失则退回
// 直接 kill 单个进程。
func killGroup(pid int, sig syscall.Signal) {
	if err := syscall.Kill(-pid, sig); err != nil {
		_ = syscall.Kill(pid, sig) // already dead → ESRCH ignored
	}
}

// Run 以双计时看门狗运行 command。阻塞到进程退出且 stdio 冲刷完毕
// （Node 'close' 语义：'exit' 可能在缓冲输出未排干时触发，'close' 保证流已
// 冲洗——尾部的 agent_settled/errorMessage 事件不会丢）。
func Run(command string, args []string, opts Opts) Result {
	started := time.Now()
	idleTimeout := msDur(opts.IdleTimeoutMs, DefaultIdleTimeoutMs)
	wallTimeout := msDur(opts.WallTimeoutMs, DefaultWallTimeoutMs)
	killGrace := msDur(opts.KillGraceMs, DefaultKillGraceMs)

	res := Result{}

	cmd := exec.Command(command, args...)
	cmd.Dir = opts.Cwd
	cmd.Env = opts.Env
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // 自成进程组 → 可整树 kill

	// StdoutPipe/StderrPipe 在 Start 前调用且各一次，错误不可达（exec 内部
	// 仅在重复设置或 Start 后调用时失败）——忽略返回的 error。
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		res.SpawnError = err.Error()
		res.DurationMS = time.Since(started).Milliseconds()
		return res
	}

	var mu sync.Mutex // 保护 res.Stdout/Stderr/Truncated 与 idle 定时器状态
	idleTimedOut, wallTimedOut := false, false
	killSent := false

	child := &activeChild{
		pid:    cmd.Process.Pid,
		exited: make(chan struct{}),
	}
	child.kill = func(sig syscall.Signal) {
		mu.Lock()
		killSent = true
		mu.Unlock()
		killGroup(child.pid, sig)
	}
	register(child)

	beginKill := func() {
		child.kill(syscall.SIGTERM)
		time.AfterFunc(killGrace, func() {
			select {
			case <-child.exited:
			default:
				child.kill(syscall.SIGKILL)
			}
		})
	}

	idleTimer := time.AfterFunc(idleTimeout, func() {
		mu.Lock()
		idleTimedOut = true
		mu.Unlock()
		beginKill()
	})
	wallTimer := time.AfterFunc(wallTimeout, func() {
		mu.Lock()
		wallTimedOut = true
		mu.Unlock()
		beginKill()
	})

	appendStream := func(stream, text string) {
		mu.Lock()
		defer mu.Unlock()
		if stream == "stdout" {
			if int64(len(res.Stdout)) < MaxBuffer {
				res.Stdout += text
			} else {
				res.Truncated = true
			}
		} else {
			if int64(len(res.Stderr)) < MaxBuffer {
				res.Stderr += text
			} else {
				res.Truncated = true
			}
		}
		if !idleTimedOut {
			idleTimer.Reset(idleTimeout) // 任何活动都证明流还活着
		}
		if opts.OnActivity != nil {
			go opts.OnActivity(text, stream)
		}
	}

	drain := func(r io.Reader, stream string, wg *sync.WaitGroup) {
		defer wg.Done()
		buf := make([]byte, 64*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				appendStream(stream, string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go drain(stdoutPipe, "stdout", &wg)
	go drain(stderrPipe, "stderr", &wg)

	waitDone := make(chan struct{})
	go func() {
		// 先等进程退出，再等管道排干 —— 合起来才是 Node 'close' 语义。
		_ = cmd.Wait()
		wg.Wait()
		close(waitDone)
	}()

	<-waitDone // Node 'close'：进程退出 + stdio 排干
	// close 后的最后清扫：组长死了，trap 了 SIGTERM 的孙子可能还在——绝不留孤儿。
	mu.Lock()
	sent := killSent
	idleFlag, wallFlag := idleTimedOut, wallTimedOut
	stdoutCopy, stderrCopy, truncCopy := res.Stdout, res.Stderr, res.Truncated
	mu.Unlock()
	unregister(child.pid)
	idleTimer.Stop()
	wallTimer.Stop()
	// Node finish() 里的 exitResolve()：标记该子进程已退出（供
	// KillActiveChildren 的 allSettled 等价等待）。
	close(child.exited)
	if sent {
		_ = syscall.Kill(-child.pid, syscall.SIGKILL) // 组已消失则 ESRCH 忽略
	}

	res.ExitCode = exitCodeOf(cmd)
	res.Signal = signalOf(cmd)
	res.TimedOut = idleFlag || wallFlag
	res.IdleTimedOut = idleFlag
	res.WallTimedOut = wallFlag
	res.Stdout = stdoutCopy
	res.Stderr = stderrCopy
	res.Truncated = truncCopy
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
