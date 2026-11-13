package investigation

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
	"github.com/stretchr/testify/require"
)

func httpAction(url string) contract.InvestigationAction {
	return contract.InvestigationAction{ID: "http", Kind: "http_read", Purpose: "Read the approved endpoint", Hypotheses: []string{"service"}, HTTP: &contract.InvestigationHTTP{URL: url, Method: "GET", HeadersEnv: map[string]string{}, TimeoutMS: 1000, MaxBytes: 1024, ExpectStatus: 200}}
}

func TestHTTPReadUsesExactTargetNamedHeadersAndNoRedirectOrProxy(t *testing.T) {
	calls, redirected := 0, 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected++; w.Write([]byte("not approved")) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "Bearer sensitive-value", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Proxy-Authorization"))
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, 302)
			return
		}
		w.Write([]byte("evidence " + r.Header.Get("Authorization")))
	}))
	defer server.Close()
	a := httpAction(server.URL + "/read?exact=yes")
	a.HTTP.HeadersEnv["Authorization"] = "READ_AUTH"
	o := Options{LookupEnv: func(name string) (string, bool) {
		if name == "READ_AUTH" {
			return "Bearer sensitive-value", true
		}
		return "", false
	}}
	t.Setenv("HTTP_PROXY", "http://192.0.2.1:1")
	r := performAction(context.Background(), "", a, contract.InvestigationLimits{}, 1024, o)
	require.NoError(t, r.Err)
	require.Equal(t, "evidence [REDACTED]", r.Output)
	require.Equal(t, "not-created", r.Execution.Cleanup)
	require.Equal(t, 200, r.Execution.HTTPStatus)
	require.NotEmpty(t, r.ContentHash)
	a.HTTP.URL = server.URL + "/redirect"
	r = readHTTP(context.Background(), a, 1024, o)
	require.Error(t, r.Err)
	require.Equal(t, 302, r.Execution.HTTPStatus)
	require.Zero(t, redirected)
	require.Equal(t, 2, calls)
}

func TestHTTPHeadAndIncompleteResponsesAreRecordedConservatively(t *testing.T) {
	for _, scenario := range []string{"head", "status", "truncated", "timeout", "cancel", "body-error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "status" {
					w.WriteHeader(503)
				}
				if scenario == "truncated" {
					w.Write([]byte(strings.Repeat("x", 300)))
					return
				}
				if scenario == "timeout" {
					<-r.Context().Done()
					return
				}
				if scenario == "cancel" {
					cancel()
					<-r.Context().Done()
					return
				}
				w.Write([]byte("evidence"))
			}))
			defer server.Close()
			a := httpAction(server.URL)
			if scenario == "head" {
				a.HTTP.Method = "HEAD"
			}
			if scenario == "timeout" {
				a.HTTP.TimeoutMS = 10
			}
			if scenario == "body-error" {
				old := readHTTPBody
				t.Cleanup(func() { readHTTPBody = old })
				readHTTPBody = func(io.Reader) ([]byte, error) { return []byte("partial"), io.ErrUnexpectedEOF }
			}
			r := readHTTP(ctx, a, 128, Options{})
			if scenario == "head" {
				require.NoError(t, r.Err)
				require.Empty(t, r.Output)
			} else {
				require.Error(t, r.Err)
			}
			if scenario == "truncated" {
				require.True(t, r.Truncated)
				require.Len(t, r.Output, 128)
			}
			if scenario == "timeout" {
				require.True(t, r.Execution.TimedOut)
			}
			if scenario == "cancel" {
				require.True(t, r.Execution.Cancelled)
			}
		})
	}
}

func TestHTTPRejectsBadTargetsHeadersAndTransportFailuresBeforeAdoption(t *testing.T) {
	for _, scenario := range []string{"url", "method", "missing-env", "newline-header", "empty-header", "transport", "late-timeout", "late-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			a := httpAction("https://example.invalid/health")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			o := Options{LookupEnv: func(string) (string, bool) { return "", false }}
			old := performHTTPRead
			t.Cleanup(func() { performHTTPRead = old })
			performHTTPRead = func(client *http.Client, request *http.Request) (*http.Response, error) {
				require.Nil(t, client.Transport.(*http.Transport).Proxy)
				if scenario == "transport" {
					return nil, io.ErrUnexpectedEOF
				}
				if scenario == "late-timeout" {
					<-request.Context().Done()
				}
				if scenario == "late-cancel" {
					cancel()
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("evidence"))}, nil
			}
			if scenario == "url" {
				a.HTTP.URL = "file:///etc/passwd"
			}
			if scenario == "method" {
				a.HTTP.Method = "bad method"
			}
			if scenario == "missing-env" || scenario == "newline-header" || scenario == "empty-header" {
				a.HTTP.HeadersEnv["Authorization"] = "READ_AUTH"
			}
			if scenario == "newline-header" {
				o.LookupEnv = func(string) (string, bool) { return "bad\r\nheader", true }
			}
			if scenario == "empty-header" {
				o.LookupEnv = func(string) (string, bool) { return "", true }
			}
			if scenario == "late-timeout" {
				a.HTTP.TimeoutMS = 1
			}
			r := readHTTP(ctx, a, 128, o)
			require.Error(t, r.Err)
		})
	}
}

func TestHTTPReadRunsThroughDurableInvestigationAndDoesNotReplay(t *testing.T) {
	dir, cwd, p := preparationFixture(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte("service evidence")) }))
	defer server.Close()
	a := httpAction(server.URL)
	a.Hypotheses = []string{p.Hypotheses[0].ID}
	p.Actions = []contract.InvestigationAction{a}
	startFixture(t, dir, cwd, p)
	st, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "awaiting-report", st.Status)
	require.Len(t, st.Observations, 1)
	require.Equal(t, "observed", st.Observations[0].Status)
	require.Equal(t, 200, st.Observations[0].Execution.HTTPStatus)
	st, err = Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	loaded, err := state.LoadPreparation(dir)
	require.NoError(t, err)
	require.Equal(t, st.Observations, loaded.Observations)
	s := snapshot(loaded, p.Actions, loaded.Observations[0].Invocation.CandidateHash)
	require.Contains(t, s.Facts[len(s.Facts)-1].Text, "http-status=200")
	require.NotContains(t, s.Facts[len(s.Facts)-1].Text, "service evidence")
	require.NotEmpty(t, s.MissingFacts)
}

func TestHTTPResponseDeadlineAndOutputLimitsAreIndependent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	a := httpAction(server.URL)
	a.HTTP.TimeoutMS = 10
	r := readHTTP(context.Background(), a, 128, Options{})
	require.Error(t, r.Err)
	require.True(t, r.Execution.TimedOut)
	require.Equal(t, "partial", r.Output)
}

func TestHTTPResponseTruncationDoesNotLeakCredentialPrefix(t *testing.T) {
	secret := "prefix-sensitive-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("body:" + r.Header.Get("Authorization"))) }))
	defer server.Close()
	a := httpAction(server.URL)
	a.HTTP.HeadersEnv["Authorization"] = "TOKEN"
	a.HTTP.MaxBytes = 12
	r := readHTTP(context.Background(), a, 128, Options{LookupEnv: func(string) (string, bool) { return secret, true }})
	require.Error(t, r.Err)
	require.True(t, r.Truncated)
	require.NotContains(t, r.Output, "prefix-")
	require.NotContains(t, r.Output, "sensit")
}
