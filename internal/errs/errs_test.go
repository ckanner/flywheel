package errs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetryableAndFallbackSets(t *testing.T) {
	retryable := []ErrorCode{EMPTY_RESPONSE, RATE_LIMIT, SERVER, TIMEOUT, TRANSPORT}
	for _, c := range retryable {
		assert.True(t, IsRetryable(c), "%s must be retryable", c)
	}
	for _, c := range AllCodes {
		if !contains(retryable, c) {
			assert.False(t, IsRetryable(c), "%s must not be retryable", c)
		}
	}
	assert.True(t, ShouldFallback(QUOTA))
	assert.True(t, ShouldFallback(RATE_LIMIT))
	assert.False(t, ShouldFallback(AUTH))
	assert.False(t, ShouldFallback(SERVER))
	assert.False(t, ShouldFallback(EMPTY_RESPONSE))
}

func contains(codes []ErrorCode, c ErrorCode) bool {
	for _, x := range codes {
		if x == c {
			return true
		}
	}
	return false
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		in   SpawnResult
		want ErrorCode
	}{
		{"timed out wins over everything", SpawnResult{TimedOut: true, ExitCode: 0, Stderr: "all good"}, TIMEOUT},
		{"spawn error", SpawnResult{SpawnError: errors.New("ENOENT")}, TRANSPORT},
		{"context window", SpawnResult{Stderr: "maximum context length exceeded"}, CONTEXT_WINDOW_EXCEEDED},
		{"context window 2", SpawnResult{Stderr: "prompt is too long"}, CONTEXT_WINDOW_EXCEEDED},
		{"context window 3", SpawnResult{Stderr: "context window exceeded"}, CONTEXT_WINDOW_EXCEEDED},
		{"429 rate limit", SpawnResult{Stderr: "HTTP 429 Too Many Requests"}, RATE_LIMIT},
		{"rate limit word", SpawnResult{Stderr: "rate limit hit"}, RATE_LIMIT},
		{"too many requests", SpawnResult{Stdout: "too many requests"}, RATE_LIMIT},
		{"401 auth", SpawnResult{Stderr: "401: invalid credentials"}, AUTH},
		{"unauthorized", SpawnResult{Stderr: "Unauthorized"}, AUTH},
		{"invalid api key", SpawnResult{Stderr: "Invalid API key provided"}, AUTH},
		{"autherror", SpawnResult{Stderr: "autherror: failed"}, AUTH},
		{"403 quota", SpawnResult{Stderr: "403 Forbidden"}, QUOTA},
		{"quota word", SpawnResult{Stderr: "quota exceeded"}, QUOTA},
		{"insufficient balance", SpawnResult{Stderr: "insufficient balance"}, QUOTA},
		{"payment required", SpawnResult{Stderr: "402 Payment Required"}, QUOTA},
		{"billing", SpawnResult{Stderr: "billing error"}, QUOTA},
		{"500 server", SpawnResult{Stderr: "500 Internal Server Error"}, SERVER},
		{"server error", SpawnResult{Stderr: "server error occurred"}, SERVER},
		{"bad gateway", SpawnResult{Stderr: "502 Bad Gateway"}, SERVER},
		{"service unavailable", SpawnResult{Stderr: "503 Service Unavailable"}, SERVER},
		{"overloaded", SpawnResult{Stderr: "server overloaded"}, SERVER},
		{"timeout word", SpawnResult{Stderr: "request timeout"}, TIMEOUT},
		{"timed out", SpawnResult{Stderr: "timed out waiting"}, TIMEOUT},
		// 注意：Node 原实现里 "context.{0,20}exceeded" 先于 TIMEOUT 模式命中，
		// "context deadline exceeded" 归 CONTEXT_WINDOW_EXCEEDED —— 忠实移植，
		// 此处锁定该行为。
		{"context deadline exceeded", SpawnResult{Stderr: "context deadline exceeded"}, CONTEXT_WINDOW_EXCEEDED},
		{"deadline exceeded standalone", SpawnResult{Stderr: "the deadline exceeded"}, TIMEOUT},
		{"not timeout stays clean", SpawnResult{Stderr: "did not time out"}, ""},
		{"n't time out stays clean", SpawnResult{Stderr: "didn't time out"}, ""},
		{"setTimeout never classifies", SpawnResult{Stderr: "setTimeout(1000) called"}, ""},
		{"econnrefused", SpawnResult{Stderr: "Error: connect ECONNREFUSED"}, TRANSPORT},
		{"econnreset", SpawnResult{Stderr: "ECONNRESET"}, TRANSPORT},
		{"enotfound", SpawnResult{Stderr: "ENOTFOUND api.example.com"}, TRANSPORT},
		{"socket hang up", SpawnResult{Stderr: "socket hang up"}, TRANSPORT},
		{"network error", SpawnResult{Stderr: "network error"}, TRANSPORT},
		{"fetch failed", SpawnResult{Stderr: "fetch failed"}, TRANSPORT},
		{"connection refused", SpawnResult{Stderr: "connection refused"}, TRANSPORT},
		{"dns", SpawnResult{Stderr: "dns lookup failed"}, TRANSPORT},
		{"400 invalid request", SpawnResult{Stderr: "400 Bad Request"}, INVALID_REQUEST},
		{"malformed", SpawnResult{Stderr: "malformed request"}, INVALID_REQUEST},
		{"signal without watchdog", SpawnResult{Signal: "SIGKILL", ExitCode: 0}, TRANSPORT},
		{"non-zero unclassified", SpawnResult{ExitCode: 3, Stderr: "something odd"}, TRANSPORT},
		{"clean zero", SpawnResult{ExitCode: 0, Stderr: "", Stdout: "ok"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, ClassifyError(c.in))
		})
	}
}

// P0-6 边界：数字状态码只在词边界匹配，数字内部形式绝不误读。
func TestStatusCodeBoundary(t *testing.T) {
	notErrors := []string{
		"sleep 500ms and retry",    // 500 在数字内部（500ms）
		"1,500 requests processed", // 1,500
		"429.5 average latency",    // 429.5
		"port 4291 refused",        // port 4291
		"HTTP 429x",                // 429x → 后续字母，不匹配
		"error code 0401",          // 0401 → 前导 0 是字母数字
		"value is 500.00",          // 500.00 → . 是禁止边界字符
	}
	for _, s := range notErrors {
		got := ClassifyError(SpawnResult{Stderr: s, ExitCode: 0})
		assert.Equal(t, ErrorCode(""), got, "must not classify: %q", s)
	}
	// "timeout" 词本身就该命中 TIMEOUT —— 5002 只是不匹配 5xx 状态码。
	assert.Equal(t, TIMEOUT, ClassifyError(SpawnResult{Stderr: "timeout at 5002ms"}))
	shouldErrors := []struct {
		s    string
		want ErrorCode
	}{
		{"HTTP 429 Too Many Requests", RATE_LIMIT},
		{"error 500 Internal Server Error", SERVER},
		{"401 Unauthorized", AUTH},
		{"403 Forbidden", QUOTA},
		{"400 Bad Request", INVALID_REQUEST},
		{"code 500 at end", SERVER},
	}
	for _, c := range shouldErrors {
		got := ClassifyError(SpawnResult{Stderr: c.s})
		assert.Equal(t, c.want, got, "must classify: %q", c.s)
	}
}

// 顺序：先匹配先赢 — 401 后接 quota 文本仍归 AUTH（模式顺序），
// 403 归 QUOTA 而非 SERVER（403 ≠ 5xx）。
func TestPatternPriority(t *testing.T) {
	assert.Equal(t, AUTH, ClassifyError(SpawnResult{Stderr: "401 Unauthorized, quota exceeded"}))
	assert.Equal(t, QUOTA, ClassifyError(SpawnResult{Stderr: "403 Forbidden, server error"}))
	assert.Equal(t, RATE_LIMIT, ClassifyError(SpawnResult{Stderr: "429 rate limit, server error"}))
	assert.Equal(t, CONTEXT_WINDOW_EXCEEDED, ClassifyError(SpawnResult{Stderr: "context window exceeded 500 internal"}))
}

func TestExtractRetryAfterMs(t *testing.T) {
	assert.Equal(t, int64(0), ExtractRetryAfterMs(""))
	assert.Equal(t, int64(0), ExtractRetryAfterMs("no hint here"))
	assert.Equal(t, int64(3000), ExtractRetryAfterMs("Retry-After: 3"))
	assert.Equal(t, int64(120000), ExtractRetryAfterMs("retry-after: 120"))
	assert.Equal(t, int64(0), ExtractRetryAfterMs("Retry-After: Fri, 31 Dec 1999 23:59:59 GMT")) // HTTP-date 不解析
	assert.Equal(t, int64(5000), ExtractRetryAfterMs("x\nretry-after: 5\ny"))
}

func TestComputeBackoff(t *testing.T) {
	// 无 jitter 语义（random 固定 0.5 时 jitter 完全抵消）时 exponential 精确可断言。
	noJitter := BackoffOptions{Jitter: 0, Random: func() float64 { return 0.5 }}
	assert.Equal(t, int64(500), ComputeBackoff(0, noJitter))
	assert.Equal(t, int64(1000), ComputeBackoff(1, noJitter))
	assert.Equal(t, int64(2000), ComputeBackoff(2, noJitter))
	assert.Equal(t, int64(10000), ComputeBackoff(20, noJitter), "capped at maxDelayMs")
	assert.Equal(t, int64(500), ComputeBackoff(0, BackoffOptions{Random: func() float64 { return 0.5 }}))

	// jitter 边界：base ±10%。
	jittered := BackoffOptions{Jitter: 0.1, Random: func() float64 { return 0 }}
	assert.Equal(t, int64(450), ComputeBackoff(0, jittered))
	jitteredHi := BackoffOptions{Jitter: 0.1, Random: func() float64 { return 1 }}
	assert.Equal(t, int64(550), ComputeBackoff(0, jitteredHi))

	// Retry-After 尊重但封顶。
	withRetry := BackoffOptions{BaseDelayMs: 500, MaxDelayMs: 10000, Jitter: 0.1, RetryAfterMs: 5000, Random: func() float64 { return 0.5 }}
	assert.Equal(t, int64(5000), ComputeBackoff(0, withRetry))
	capped := BackoffOptions{BaseDelayMs: 500, MaxDelayMs: 10000, Jitter: 0.1, RetryAfterMs: 86400000, Random: func() float64 { return 0.5 }}
	assert.Equal(t, int64(10000), ComputeBackoff(1, capped))

	// 非法/越界参数按 Node 语义处理：负 base/max 回退默认，jitter 不 clamp。
	assert.Equal(t, int64(1000), ComputeBackoff(1, BackoffOptions{BaseDelayMs: -1, MaxDelayMs: -1, Jitter: 2, Random: func() float64 { return 0.5 }}))
	assert.Equal(t, int64(3000), ComputeBackoff(1, BackoffOptions{BaseDelayMs: -1, MaxDelayMs: -1, Jitter: 2, Random: func() float64 { return 1 }}))
	// 负 Retry-After 归一为 0。
	assert.Equal(t, int64(500), ComputeBackoff(0, BackoffOptions{Jitter: 0.1, RetryAfterMs: -5, Random: func() float64 { return 0.5 }}))
	// 未注入 Random 时走全局源（不崩溃、落在合法区间）。
	got := ComputeBackoff(0, BackoffOptions{})
	assert.GreaterOrEqual(t, got, int64(0))
	assert.LessOrEqual(t, got, int64(10000))
}
