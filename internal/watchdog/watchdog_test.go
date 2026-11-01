package watchdog

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunQuickEcho(t *testing.T) {
	r := Run("sh", []string{"-c", "echo out-line; echo err-line >&2"}, Opts{
		IdleTimeoutMs: 5000, WallTimeoutMs: 10000, KillGraceMs: 500,
	})
	assert.Empty(t, r.SpawnError)
	require.NotNil(t, r.ExitCode)
	assert.Equal(t, 0, *r.ExitCode)
	assert.Empty(t, r.Signal)
	assert.False(t, r.TimedOut)
	assert.Contains(t, r.Stdout, "out-line")
	assert.Contains(t, r.Stderr, "err-line")
	assert.False(t, r.Truncated)
}

func TestRunSpawnError(t *testing.T) {
	r := Run("/no/such/binary", nil, Opts{})
	assert.NotEmpty(t, r.SpawnError)
	assert.Nil(t, r.ExitCode)
}

func TestRunIdleTimeoutKillsProcessGroup(t *testing.T) {
	start := time.Now()
	r := Run("sh", []string{"-c", "sleep 30"}, Opts{
		IdleTimeoutMs: 120, WallTimeoutMs: 60000, KillGraceMs: 300,
	})
	assert.True(t, r.IdleTimedOut)
	assert.True(t, r.TimedOut)
	assert.False(t, r.WallTimedOut)
	assert.Less(t, time.Since(start).Milliseconds(), int64(10000), "idle 超时必须及时杀进程")
}

func TestRunWallTimeoutCapsOverall(t *testing.T) {
	// 有输出活动（idle 不超时），wall 兜底生效。
	r := Run("sh", []string{"-c", `i=0; while [ $i -lt 100 ]; do echo tick; sleep 1; i=$((i+1)); done`}, Opts{
		IdleTimeoutMs: 800, WallTimeoutMs: 400, KillGraceMs: 300,
	})
	assert.True(t, r.WallTimedOut)
	assert.True(t, r.TimedOut)
	assert.False(t, r.IdleTimedOut)
}

func TestRunIdleResetOnActivity(t *testing.T) {
	// 每 60ms 输出一行共 10 次（总 ~600ms > idle 250ms）——活动重置 idle，正常跑完。
	r := Run("sh", []string{`-c`, `i=0; while [ $i -lt 10 ]; do echo tick-$i; sleep 0.06; i=$((i+1)); done`}, Opts{
		IdleTimeoutMs: 250, WallTimeoutMs: 30000, KillGraceMs: 500,
	})
	assert.Empty(t, r.SpawnError)
	require.NotNil(t, r.ExitCode)
	assert.Equal(t, 0, *r.ExitCode)
	assert.False(t, r.TimedOut)
	assert.Equal(t, 10, strings.Count(r.Stdout, "tick-"))
}

func TestRunOnActivityCallback(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	var streams []string
	r := Run("sh", []string{"-c", "echo a; echo b >&2"}, Opts{
		OnActivity: func(chunk string, stream string) {
			mu.Lock()
			streams = append(streams, stream)
			mu.Unlock()
			calls.Add(1)
		},
	})
	assert.Empty(t, r.SpawnError)
	assert.GreaterOrEqual(t, calls.Load(), int32(2))
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, streams, "stdout")
	assert.Contains(t, streams, "stderr")
}
func TestRunTruncatesOverMaxBuffer(t *testing.T) {
	orig := MaxBuffer
	MaxBuffer = 1024
	t.Cleanup(func() { MaxBuffer = orig })

	// 托管复制语义：cappedBuffer 硬字节上限——写入时只保留 cap 内的字节，
	// 超出部分丢弃并置 truncated（比 Node 的 chunk 粒度更精确）。
	r := Run("sh", []string{"-c", "head -c 2048 /dev/zero | tr '\\0' a; sleep 0.2; echo second"}, Opts{
		IdleTimeoutMs: 5000, WallTimeoutMs: 20000,
	})
	assert.Empty(t, r.SpawnError)
	assert.True(t, r.Truncated)
	assert.Equal(t, 1024, len(r.Stdout), "超出 cap 的字节必须被截断")
	assert.NotContains(t, r.Stdout, "second")
}

func TestRunKillsWholeGroupIncludingGrandchildren(t *testing.T) {
	// shell 再 spawn 孙进程；idle 超时 kill 组后孙进程必须也死。
	dir := t.TempDir()
	marker := dir + "/alive"
	script := `(sleep 30 & echo $! > ` + marker + `; wait) & wait`
	r := Run("sh", []string{"-c", script}, Opts{
		IdleTimeoutMs: 150, WallTimeoutMs: 60000, KillGraceMs: 300,
	})
	assert.True(t, r.IdleTimedOut)

	data, err := os.ReadFile(marker)
	require.NoError(t, err)
	grandchild := strings.TrimSpace(string(data))
	require.NotEmpty(t, grandchild)

	// 孙进程组已死：kill(pid, 0) 应报 ESRCH（轮询几轮等内核回收）。
	gpid, perr := strconv.Atoi(grandchild)
	require.NoError(t, perr)
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := syscall.Kill(gpid, 0)
		if err == syscall.ESRCH {
			break // 已死
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d survived group kill", gpid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRunSIGTERMTrappedEscalatesToSIGKILL(t *testing.T) {
	start := time.Now()
	// trap 掉 SIGTERM 的进程只能靠 SIGKILL 升级。
	r := Run("sh", []string{"-c", `trap "" TERM; sleep 30`}, Opts{
		IdleTimeoutMs: 150, WallTimeoutMs: 60000, KillGraceMs: 400,
	})
	elapsed := time.Since(start)
	assert.True(t, r.TimedOut)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(150), "必须先给 grace")
	assert.Less(t, elapsed.Milliseconds(), int64(15000), "SIGKILL 升级必须生效")
}

func TestKillActiveChildren(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]Result, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = Run("sh", []string{"-c", "sleep 30"}, Opts{
				IdleTimeoutMs: 60000, WallTimeoutMs: 60000, KillGraceMs: 500,
			})
		}(i)
	}
	time.Sleep(300 * time.Millisecond) // 等两个子进程都登记在册

	killed := time.Now()
	KillActiveChildren(time.Second)
	wg.Wait()
	assert.Less(t, time.Since(killed).Milliseconds(), int64(10000))
	for _, r := range results {
		assert.True(t, r.Cancelled)
		assert.False(t, r.TimedOut)
		assert.True(t, r.ExitCode != nil || r.Signal != "", "取消必须等待进程终态")
	}
	regMu.Lock()
	n := len(registry)
	regMu.Unlock()
	assert.Equal(t, 0, n, "全部退出后注册表必须清空")
}

func TestKillActiveChildrenNoopWhenEmpty(t *testing.T) {
	start := time.Now()
	KillActiveChildren(time.Millisecond)
	assert.Less(t, time.Since(start).Milliseconds(), int64(100))
}

func TestKillActiveChildrenEscalatesToSIGKILL(t *testing.T) {
	// trap 了 SIGTERM 的子进程：grace 到期后 AfterFunc 必须 SIGKILL 补刀。
	done := make(chan Result, 1)
	go func() {
		done <- Run("sh", []string{"-c", `trap "" TERM; sleep 30`}, Opts{
			IdleTimeoutMs: 60000, WallTimeoutMs: 60000, KillGraceMs: 300,
		})
	}()
	time.Sleep(250 * time.Millisecond) // 等登记在册
	start := time.Now()
	KillActiveChildren(time.Second)
	r := <-done
	elapsed := time.Since(start)
	assert.Less(t, elapsed.Milliseconds(), int64(15000), "SIGKILL 升级必须生效")
	assert.True(t, r.Cancelled)
	assert.NotEmpty(t, r.Signal)
}

func TestRunCancelledContextNeverStarts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	marker := t.TempDir() + "/unexpected"
	r := Run("sh", []string{"-c", fmt.Sprintf("touch %q", marker)}, Opts{Context: ctx})
	assert.True(t, r.Cancelled)
	assert.Nil(t, r.ExitCode)
	assert.NoFileExists(t, marker)
}

func TestRunContextCancellationCannotBecomeSuccessfulExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := Run("sh", []string{"-c", `trap 'exit 0' TERM; echo ready; while :; do :; done`}, Opts{
		Context: ctx, IdleTimeoutMs: 30_000, WallTimeoutMs: 30_000, KillGraceMs: 100,
		OnActivity: func(chunk, _ string) {
			if strings.Contains(chunk, "ready") {
				cancel()
			}
		},
	})
	require.NotNil(t, r.ExitCode)
	assert.Equal(t, 0, *r.ExitCode, "TERM trap 正常退出也必须保留取消原因")
	assert.True(t, r.Cancelled)
	assert.False(t, r.TimedOut)
}

func TestRunNormalExitCleansOwnedBackgroundWriters(t *testing.T) {
	marker := t.TempDir() + "/writes"
	script := fmt.Sprintf(`(trap '' TERM; while :; do printf x >> %q; sleep 0.01; done) >/dev/null 2>&1 &
while [ ! -s %q ]; do sleep 0.01; done
exit 0`, marker, marker)
	r := Run("sh", []string{"-c", script}, Opts{IdleTimeoutMs: 30_000, WallTimeoutMs: 30_000, KillGraceMs: 100})
	require.NotNil(t, r.ExitCode)
	assert.Equal(t, 0, *r.ExitCode)
	assert.Empty(t, r.CleanupError)
	assert.False(t, r.Cancelled)
	before, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.NotEmpty(t, before)
	time.Sleep(150 * time.Millisecond)
	after, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, before, after, "正常会话结束后孙进程不得继续写工作区")
}

func TestRunLeaderExitWithInheritedPipesIsBoundedAndFails(t *testing.T) {
	started := time.Now()
	r := Run("sh", []string{"-c", `sleep 30 & exit 0`}, Opts{
		IdleTimeoutMs: 30_000, WallTimeoutMs: 30_000, KillGraceMs: 80,
	})
	assert.Less(t, time.Since(started), 3*time.Second)
	assert.NotEmpty(t, r.CleanupError, "管道被后代占用而截断排空，不得冒充完整结果")
	assert.True(t, r.Truncated)
	assert.False(t, r.TimedOut)
}

func TestRunReportsUnconfirmedCleanup(t *testing.T) {
	original := groupSignal
	t.Cleanup(func() { groupSignal = original })
	for _, fault := range []string{"kill", "inspect", "still-alive"} {
		t.Run(fault, func(t *testing.T) {
			groupSignal = func(pid int, sig syscall.Signal) error {
				if fault == "kill" && sig == syscall.SIGKILL || fault == "inspect" && sig == 0 {
					return syscall.EPERM
				}
				if fault == "still-alive" && sig == 0 {
					return nil
				}
				return original(pid, sig)
			}
			r := Run("true", nil, Opts{KillGraceMs: 20})
			require.NotNil(t, r.ExitCode)
			assert.Equal(t, 0, *r.ExitCode)
			assert.NotEmpty(t, r.CleanupError, "正常退出码不能掩盖无法确认清理的故障")
		})
	}
}

func TestKillActiveChildrenDoesNotKillLaterCalls(t *testing.T) {
	t.Cleanup(func() { KillActiveChildren(time.Millisecond) })
	ready, term := make(chan struct{}, 1), make(chan struct{}, 1)
	firstDone := make(chan Result, 1)
	go func() {
		firstDone <- Run("sh", []string{"-c", `trap 'echo term' TERM; echo ready; while :; do :; done`}, Opts{
			IdleTimeoutMs: 30_000, WallTimeoutMs: 30_000,
			OnActivity: func(chunk, _ string) {
				if strings.Contains(chunk, "ready") {
					ready <- struct{}{}
				}
				if strings.Contains(chunk, "term") {
					term <- struct{}{}
				}
			},
		})
	}()
	require.Eventually(t, func() bool {
		select {
		case <-ready:
			return true
		default:
			return false
		}
	}, 3*time.Second, 5*time.Millisecond)
	killed := make(chan struct{})
	go func() { KillActiveChildren(300 * time.Millisecond); close(killed) }()
	require.Eventually(t, func() bool {
		select {
		case <-term:
			return true
		default:
			return false
		}
	}, 3*time.Second, 5*time.Millisecond) // SIGTERM 已发送；后续调用不在取消快照中。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	laterDone := make(chan Result, 1)
	go func() {
		laterDone <- Run("sh", []string{"-c", `echo later; sleep 30`}, Opts{
			Context: ctx, IdleTimeoutMs: 30_000, WallTimeoutMs: 30_000, KillGraceMs: 100,
		})
	}()
	<-killed
	assert.True(t, (<-firstDone).Cancelled)
	select {
	case r := <-laterDone:
		t.Fatalf("unrelated later call was killed: %+v", r)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()
	assert.True(t, (<-laterDone).Cancelled)
}

func TestKillGroupFallbackToSingleKill(t *testing.T) {
	// 组 kill 对已消失 pid 报错 → 回退单杀（同样 ESRCH，仅覆盖分支）。
	dead := exec.Command("true")
	require.NoError(t, dead.Start())
	pid := dead.Process.Pid
	_, _ = dead.Process.Wait()
	killGroup(pid, syscall.SIGTERM) // 不 panic 即可
}

func TestRunTruncatesStderrOverMaxBuffer(t *testing.T) {
	orig := MaxBuffer
	MaxBuffer = 512
	t.Cleanup(func() { MaxBuffer = orig })

	r := Run("sh", []string{"-c", "head -c 1024 /dev/zero | tr '\\0' e >&2; sleep 0.2; echo late >&2"}, Opts{
		IdleTimeoutMs: 5000, WallTimeoutMs: 20000,
	})
	assert.Empty(t, r.SpawnError)
	assert.True(t, r.Truncated)
	assert.LessOrEqual(t, len(r.Stderr), 512)
}

// TestRunInteractStdinHook 协议驱动交互钩子：hook 写入 stdin、子进程回显为
// stdout 活动（idle 重置），finish() 走与超时同一条 kill 路径——TimedOut
// 必须为 false（driver 主动结束不是超时），进程被 SIGTERM 收割。
func TestRunInteractStdinHook(t *testing.T) {
	wrote := make(chan string, 1)
	r := Run("sh", []string{"-c", "cat"}, Opts{
		IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000, KillGraceMs: 1_000,
		InteractStdin: func(stdin io.WriteCloser, finish func()) {
			_, _ = stdin.Write([]byte("ping\n"))
			wrote <- "sent"
			// 等 cat 把回显刷出来再结束（回显即 stdout 活动，证明 idle 计时被重置）。
			time.Sleep(300 * time.Millisecond)
			finish()
			// finish 后 stdin 不再使用；关闭让潜在的对端写出尽快 EPIPE。
			_ = stdin.Close()
		},
	})
	assert.Equal(t, "sent", <-wrote)
	assert.Empty(t, r.SpawnError)
	assert.False(t, r.TimedOut, "driver 主动 finish 不是超时")
	assert.Contains(t, r.Stdout, "ping", "stdin 写入经 cat 回显")
	assert.NotEmpty(t, r.Signal, "进程经 finish 的 SIGTERM 路径收割")
}

// TestRunInteractStdinFinishIdempotent finish 幂等：多次调用不 panic、不重复
// kill（once 语义）。
func TestRunInteractStdinFinishIdempotent(t *testing.T) {
	r := Run("sh", []string{"-c", "cat"}, Opts{
		IdleTimeoutMs: 30_000, WallTimeoutMs: 60_000, KillGraceMs: 1_000,
		InteractStdin: func(stdin io.WriteCloser, finish func()) {
			finish()
			finish() // 第二次必须是 no-op（sync.Once）
			_ = stdin.Close()
		},
	})
	assert.False(t, r.TimedOut)
	assert.NotEmpty(t, r.Signal)
}
