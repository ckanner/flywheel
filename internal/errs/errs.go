// Package errs — 错误分类与重试策略（docs/design.md §6，源自 deepseek-harness）。
// spawn 失败归一化为稳定错误码；只有固定子集可重试。数值状态码只在词边界匹配
// （"500ms"、"1,500"、"429.5"、"port 4291" 不得误读为 HTTP 500/429 — P0-6）。
//
// 注意：Node 原实现用了 lookbehind/lookahead 正则（RE2 不支持），此处以等价
// 捕获式改写（消费型 "([^0-9A-Za-z.,]|$)" + TIMEOUT 前置否定后处理），
// 语义由单测锁定。
package errs

import (
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"strings"
)

// mathRand 全局随机源（Go 1.20+ 自动播种）；测试可注入替换。
var mathRand = rand.Float64

// ErrorCode 稳定错误码（docs/design.md §6）。
type ErrorCode string

const (
	AUTH                    ErrorCode = "AUTH"
	QUOTA                   ErrorCode = "QUOTA"
	RATE_LIMIT              ErrorCode = "RATE_LIMIT"
	SERVER                  ErrorCode = "SERVER"
	TRANSPORT               ErrorCode = "TRANSPORT"
	TIMEOUT                 ErrorCode = "TIMEOUT"
	EMPTY_RESPONSE          ErrorCode = "EMPTY_RESPONSE"
	INVALID_REQUEST         ErrorCode = "INVALID_REQUEST"
	CONTEXT_WINDOW_EXCEEDED ErrorCode = "CONTEXT_WINDOW_EXCEEDED"
)

// AllCodes 全部错误码（保持 §6 顺序）。
var AllCodes = []ErrorCode{AUTH, QUOTA, RATE_LIMIT, SERVER, TRANSPORT, TIMEOUT, EMPTY_RESPONSE, INVALID_REQUEST, CONTEXT_WINDOW_EXCEEDED}

// RetryableCodes — 默认只重试这些（§6）。
var RetryableCodes = map[ErrorCode]bool{
	EMPTY_RESPONSE: true,
	RATE_LIMIT:     true,
	SERVER:         true,
	TIMEOUT:        true,
	TRANSPORT:      true,
}

// FallbackCodes — 触发渠道降级（§6 降级）：限流/配额（403）。AUTH 不降级
// （坏 key = 配置错误；换渠道带同一个坏 key 无济于事）。
var FallbackCodes = map[ErrorCode]bool{
	QUOTA:      true,
	RATE_LIMIT: true,
}

// IsRetryable 判断错误码是否默认重试。
func IsRetryable(code ErrorCode) bool { return RetryableCodes[code] }

// ShouldFallback 判断错误码是否触发渠道降级。
func ShouldFallback(code ErrorCode) bool { return FallbackCodes[code] }

// SpawnResult 一次 spawn 的可观察结果，用于错误分类。
type SpawnResult struct {
	ExitCode   int
	Signal     string
	TimedOut   bool
	Stderr     string
	Stdout     string
	SpawnError error
}

// statusCode 构造词边界状态码匹配：`(^|[^0-9A-Za-z.,])429([^0-9A-Za-z.,]|$)`。
// 原 Node 版用 negative lookahead `(?!...)`（RE2 不支持），等价改写为消费型
// 捕获，布尔检测语义一致。
func statusCode(digits string) string {
	return "(^|[^0-9A-Za-z.,])" + digits + "([^0-9A-Za-z.,]|$)"
}

// 顺序即优先级：先匹配先赢；更具体的模式在前。
var patterns = []struct {
	code ErrorCode
	re   *regexp.Regexp
}{
	{CONTEXT_WINDOW_EXCEEDED, regexp.MustCompile(`(?i)context.{0,20}(window|length|too long|exceeded)|maximum context|prompt is too long`)},
	{RATE_LIMIT, regexp.MustCompile(`(?i)` + statusCode("429") + `|rate.?limit|too many requests`)},
	{AUTH, regexp.MustCompile(`(?i)` + statusCode("401") + `|unauthorized|invalid api key|authentication failed|autherror`)},
	{QUOTA, regexp.MustCompile(`(?i)` + statusCode("403") + `|forbidden|quota|insufficient.{0,20}(balance|credit|fund|quota)|payment required|billing`)},
	{SERVER, regexp.MustCompile(`(?i)` + statusCode("5\\d\\d") + `|server error|internal error|bad gateway|service unavailable|overloaded`)},
	{TIMEOUT, regexp.MustCompile(`(?i)\btimed?\s?out\b|\bdeadline exceeded\b`)},
	{TRANSPORT, regexp.MustCompile(`(?i)econnrefused|econnreset|enotfound|eai_again|socket hang up|network error|fetch failed|connection refused|dns`)},
	{INVALID_REQUEST, regexp.MustCompile(`(?i)` + statusCode("400") + `|invalid request|bad request|malformed`)},
}

// timeoutNegated 检查匹配位置之前是否有 "not " / "n't "（原 lookbehind 语义，
// P0-6："did not time out" 必须是成功）。
func timeoutNegated(haystack string, start int) bool {
	lo := start - 6
	if lo < 0 {
		lo = 0
	}
	before := haystack[lo:start]
	lower := strings.ToLower(before)
	return strings.HasSuffix(lower, "not ") || strings.HasSuffix(lower, "n't ")
}

// ClassifyError 将 spawn 结果归一化为稳定错误码；无错误特征时返回 ""。
// 优先级：timedOut > spawnError > 文本模式 > signal > 非零退出码(TRANSPORT)。
func ClassifyError(r SpawnResult) ErrorCode {
	if r.TimedOut {
		return TIMEOUT
	}
	if r.SpawnError != nil {
		return TRANSPORT
	}
	haystack := r.Stderr + "\n" + r.Stdout
	for _, p := range patterns {
		if p.code == TIMEOUT {
			// RE2 无 lookbehind：先找全部匹配，任一匹配未被 "not "/"n't " 前置否定即命中。
			for _, loc := range p.re.FindAllStringIndex(haystack, -1) {
				if !timeoutNegated(haystack, loc[0]) {
					return TIMEOUT
				}
			}
			continue
		}
		if p.re.MatchString(haystack) {
			return p.code
		}
	}
	if r.Signal != "" {
		return TRANSPORT // 被看门狗以外的东西杀掉
	}
	if r.ExitCode != 0 {
		// 未分类非零退出：视为意外传输级失败（可重试；持续则耗尽重试升级）。
		return TRANSPORT
	}
	return ""
}

// ExtractRetryAfterMs 提取 Retry-After 提示（仅整秒、header 风格；HTTP-date
// 形式不解析，返回 0）。
func ExtractRetryAfterMs(text string) int64 {
	re := regexp.MustCompile(`(?i)retry-after:\s*(\d+)`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	var secs int64
	fmt.Sscanf(m[1], "%d", &secs)
	return secs * 1000
}

// BackoffOptions 指数退避参数（§6：500ms → 10s，±10% jitter；Retry-After
// 尊重但封顶在 maxDelayMs — 未信任的 provider 输出不得把 loop 停数小时，P0-5）。
type BackoffOptions struct {
	BaseDelayMs  float64
	MaxDelayMs   float64
	Jitter       *float64 // nil=默认 0.1；显式 0=无 jitter（C7：配置面必须可表达）
	RetryAfterMs float64
	Random       func() float64
}

// ComputeBackoff 计算第 attempt 次（从 0 起）重试的退避毫秒数。
func ComputeBackoff(attempt int, opts BackoffOptions) int64 {
	base := opts.BaseDelayMs
	if base <= 0 {
		base = 500
	}
	maxDelay := opts.MaxDelayMs
	if maxDelay <= 0 {
		maxDelay = 10_000
	}
	jitter := 0.1
	if opts.Jitter != nil {
		jitter = *opts.Jitter
	}
	randFn := opts.Random
	if randFn == nil {
		randFn = mathRand
	}
	retryAfter := opts.RetryAfterMs
	if retryAfter < 0 {
		retryAfter = 0
	}

	exponential := math.Min(base*math.Pow(2, float64(attempt)), maxDelay)
	spread := exponential * jitter
	jittered := exponential - spread + randFn()*2*spread
	delay := math.Max(jittered, retryAfter)
	delay = math.Min(delay, maxDelay)
	return int64(math.Round(delay))
}
