package investigation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
)

var performHTTPRead = (*http.Client).Do
var readHTTPBody = io.ReadAll

func httpReadTimedOut(ctx context.Context, err error) bool {
	var network net.Error
	return ctx.Err() == context.DeadlineExceeded || (errors.As(err, &network) && network.Timeout())
}

func readHTTP(ctx context.Context, action contract.InvestigationAction, remaining int, o Options) (result actionResult) {
	e := &state.InvestigationExecution{Kind: "http_read", Cleanup: "not-created"}
	result.Execution = e
	h := action.HTTP
	if err := contract.ValidateInvestigationURL(h.URL); err != nil {
		result.Err = err
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(h.TimeoutMS)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, h.Method, h.URL, nil)
	if err != nil {
		result.Err = fmt.Errorf("invalid approved HTTP request")
		return
	}
	secrets := []string{}
	for name, env := range h.HeadersEnv {
		value, ok := o.lookup(env)
		if !ok || value == "" || strings.ContainsAny(value, "\r\n") {
			result.Err = fmt.Errorf("approved HTTP header environment is unavailable or invalid: %s", env)
			return
		}
		req.Header.Set(name, value)
		secrets = append(secrets, value)
	}
	// Do not inherit environment proxies or follow redirects outside the exact
	// approved URL. New non-reused connections also avoid transport retries.
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Duration(h.TimeoutMS) * time.Millisecond}).DialContext, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 32 << 10, TLSHandshakeTimeout: time.Duration(h.TimeoutMS) * time.Millisecond, ResponseHeaderTimeout: time.Duration(h.TimeoutMS) * time.Millisecond}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := performHTTPRead(client, req)
	if err != nil {
		e.TimedOut, e.Cancelled = httpReadTimedOut(requestCtx, err), ctx.Err() == context.Canceled
		result.Err = fmt.Errorf("approved HTTP request did not produce a complete response")
		return
	}
	defer response.Body.Close()
	e.HTTPStatus = response.StatusCode
	limit := min(h.MaxBytes, remaining)
	raw, err := readHTTPBody(io.LimitReader(response.Body, int64(limit)+1))
	e.TimedOut, e.Cancelled = httpReadTimedOut(requestCtx, err), ctx.Err() == context.Canceled
	redacted := redactExperiment(string(raw), secrets)
	result.Output, result.Truncated = boundedOutput(redacted, limit)
	result.Truncated = result.Truncated || len(raw) > limit
	result.ContentHash = digest([]byte(redacted))
	if err != nil || e.TimedOut || e.Cancelled || result.Truncated || response.StatusCode != h.ExpectStatus {
		result.Err = fmt.Errorf("HTTP observation did not match the approved complete response")
	}
	return
}
