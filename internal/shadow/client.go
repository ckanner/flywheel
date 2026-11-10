package shadow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Options struct {
	SaveInput     bool // opt-in automatic observation retains its bounded summary
	Endpoint      string
	Model         string
	ExpectedModel string
	APIKey        string
	Timeout       time.Duration
	MaxCalls      int
	Transport     http.RoundTripper
	Now           func() time.Time
}

func validTarget(o Options) bool {
	u, err := url.Parse(o.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/v1/systemone" {
		return false
	}
	local := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	return (u.Scheme == "https" || (u.Scheme == "http" && local)) &&
		o.Model != "" && len(o.Model) <= 120 && o.ExpectedModel != "" && len(o.ExpectedModel) <= 120 &&
		o.Timeout > 0 && o.Timeout <= 30*time.Second && o.MaxCalls >= 1 && o.MaxCalls <= 100
}

type response struct {
	Model   string `json:"model"`
	Answers map[string]struct {
		Noul float64 `json:"noul"`
	} `json:"answers"`
	Usage Usage `json:"usage"`
}

func denyRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

func call(ctx context.Context, c Case, o Options, record *Record) {
	record.Status, record.Advice, record.Reason = "error", "abstain", "transport"
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint, bytes.NewReader(requestBody(c, o.Model))) // validTarget has parsed the URL
	request.Header.Set("Authorization", "Bearer "+o.APIKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: o.Timeout, Transport: o.Transport, CheckRedirect: denyRedirect}
	result, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			record.Reason = "cancelled-outcome-unknown"
		} else if errors.Is(err, context.DeadlineExceeded) {
			record.Reason = "timeout-outcome-unknown"
		}
		return
	}
	defer result.Body.Close()
	record.HTTPStatus = result.StatusCode
	if result.StatusCode != http.StatusOK {
		record.Reason = "http-error"
		return // Never persist upstream error prose or request headers.
	}
	raw, err := io.ReadAll(io.LimitReader(result.Body, maxDocument+1))
	if err != nil || validate(raw, "response") != nil {
		record.Reason = "invalid-response"
		return
	}
	var value response
	_ = json.Unmarshal(raw, &value) // the response schema fixes finite probabilities and safe integer usage
	if value.Model != o.ExpectedModel {
		record.Reason = "model-version-mismatch"
		return
	}
	record.Model, record.Usage = value.Model, value.Usage
	record.Probabilities = map[string]float64{}
	for key, answer := range value.Answers {
		record.Probabilities[key] = answer.Noul
	}
	record.Status = "ok"
	record.Advice, record.Reason = advice(record.Probabilities, policy)
}

func containsKey(dataset Dataset, key string) bool {
	return key != "" && strings.Contains(string(jsonBytes(dataset)), key)
}
