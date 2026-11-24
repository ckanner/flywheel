package decision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type readError struct{}

func (readError) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestClientFailureAndAbstentionSemantics(t *testing.T) {
	for _, scenario := range []string{"request", "cancel", "timeout", "http", "read", "invalid", "version", "applicability", "choice", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			s, p, o := fixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			o.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				switch scenario {
				case "cancel":
					cancel()
					return nil, errors.New("closed")
				case "timeout":
					return nil, timeoutError{}
				case "http":
					return reply([]byte(o.APIKey), 401), nil
				case "read":
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(readError{})}, nil
				case "invalid":
					return reply([]byte("not JSON"), 200), nil
				case "version":
					return reply([]byte(strings.ReplaceAll(string(native()), "jev-1.14.0", "jev-1.15.0")), 200), nil
				case "applicability":
					return reply([]byte(strings.ReplaceAll(string(native()), `"noul":0.98`, `"noul":0.1`)), 200), nil
				case "redirect":
					r := reply(nil, 302)
					r.Header.Set("Location", "https://must-not-follow.test/v1/systemone")
					return r, nil
				}
				return reply(native(), 200), nil
			})
			if scenario == "request" {
				o.Endpoint = "://"
			}
			if scenario == "choice" {
				p.MinChoice = .99
			}
			r := receiptFor(s, p, o)
			call(ctx, s, p, o, &r)
			reasons := map[string]string{"request": "invalid-request", "cancel": "cancelled-outcome-unknown", "timeout": "timeout-outcome-unknown", "http": "http-error", "read": "read-error", "invalid": "invalid-response", "version": "model-version-mismatch", "applicability": "applicability-abstained", "choice": "choice-abstained", "redirect": "http-error"}
			if r.Reason != reasons[scenario] || calls > 1 || strings.Contains(string(raw(r)), o.APIKey) {
				t.Fatalf("incorrect failure semantics: %#v, calls=%d", r, calls)
			}
			if Select(s, r, p, o).Adopted {
				t.Fatal("failed, inapplicable or uncertain answer adopted")
			}
		})
	}
}

func TestNativeResponseRequiresCompleteValidDistribution(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"usage", func(v map[string]any) { delete(v, "usage") }},
		{"negative-usage", func(v map[string]any) { v["usage"].(map[string]any)["input_tokens"] = -1 }},
		{"noul-type", func(v map[string]any) {
			v["answers"].(map[string]any)["applicable"].(map[string]any)["type"] = "choice"
		}},
		{"missing-choice", func(v map[string]any) {
			v["answers"].(map[string]any)["action"].(map[string]any)["choice"] = "made-up-action"
		}},
		{"missing-confidence", func(v map[string]any) { delete(v["answers"].(map[string]any)["action"].(map[string]any), "confidence") }},
		{"missing-probability", func(v map[string]any) {
			v["answers"].(map[string]any)["action"].(map[string]any)["probabilities"] = map[string]any{"inspect-config": .96, "wrong": .04}
		}},
		{"out-of-range", func(v map[string]any) {
			v["answers"].(map[string]any)["action"].(map[string]any)["probabilities"] = map[string]any{"inspect-config": 1.1, "inspect-log": -.1}
		}},
		{"bad-sum", func(v map[string]any) {
			v["answers"].(map[string]any)["action"].(map[string]any)["probabilities"] = map[string]any{"inspect-config": .6, "inspect-log": .1}
		}},
		{"not-maximum", func(v map[string]any) {
			v["answers"].(map[string]any)["action"].(map[string]any)["choice"] = "inspect-log"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, p, o := fixture()
			var v map[string]any
			if err := json.Unmarshal(native(), &v); err != nil {
				t.Fatal(err)
			}
			test.mutate(v)
			r := receiptFor(s, p, o)
			if err := parseResponse(raw(v), s, o.ExpectedModel, &r); err == nil {
				t.Fatal("malformed or inconsistent response accepted")
			}
		})
	}
	s, p, o := fixture()
	r := receiptFor(s, p, o)
	if err := parseResponse([]byte(strings.Repeat(" ", maxDocument+1)), s, o.ExpectedModel, &r); err == nil {
		t.Fatal("unbounded response accepted")
	}
}

func TestHTTPGatewayAliasModelResolvesConcreteVersion(t *testing.T) {
	s, p, o := fixture()
	o.Model, o.ExpectedModel = "jev", "jev"
	calls := 0
	o.Transport = transport(&calls)
	r, err := Evaluate(context.Background(), t.TempDir()+"/journal", s, p, o)
	if err != nil || r.Status != "ok" || r.Model != "jev-1.14.0" {
		t.Fatalf("expected concrete jev-1.14.0 receipt for gateway alias jev, got r=%+v err=%v", r, err)
	}
	if err := ValidateReceiptIdentity(s, p, o, r); err != nil {
		t.Fatalf("valid concrete version receipt rejected: %v", err)
	}
	sel := Select(s, r, p, o)
	if !sel.Adopted || sel.ActionID != "inspect-config" {
		t.Fatalf("expected adoption for gateway alias jev, got %+v", sel)
	}
	// Response must still carry a concrete jev-x.y.z version, never the bare alias.
	var v map[string]any
	if err := json.Unmarshal(native(), &v); err != nil {
		t.Fatal(err)
	}
	v["model"] = "jev"
	bad := receiptFor(s, p, o)
	if err := parseResponse(raw(v), s, o.ExpectedModel, &bad); err == nil {
		t.Fatal("bare alias model in response must be rejected")
	}
}

func TestMeasuredDurationAndExplicitAdvisory(t *testing.T) {
	s, p, o := fixture()
	p.Mode = Advisory
	calls := 0
	o.Transport = transport(&calls)
	now := time.Unix(100, 0)
	o.Now = func() time.Time { result := now; now = now.Add(5 * time.Millisecond); return result }
	r, err := Evaluate(context.Background(), t.TempDir()+"/journal", s, p, o)
	if err != nil || r.DurationMS != 5 || calls != 1 || Select(s, r, p, o).Adopted {
		t.Fatalf("advisory: %#v, %v", r, err)
	}
}
