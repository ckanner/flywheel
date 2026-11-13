package contract

import (
	"testing"

	"github.com/ckanner/flywheel/templates"
	"github.com/stretchr/testify/require"
)

func commandContract(t *testing.T) Preparation {
	p := preparationContractFixture(t)
	p.Actions = []InvestigationAction{{ID: "reproduce", Kind: "run_command", Purpose: "Reproduce the default", Hypotheses: []string{p.Hypotheses[0].ID}, Command: &InvestigationCommand{Argv: []string{"python3", "reproduce.py"}, Inputs: []string{p.ReadPaths[0]}, Env: []string{}, TimeoutMS: 1000, ExpectExit: 0, ExecutionBoundary: "host-process"}}}
	return p
}

func httpContract(t *testing.T) Preparation {
	p := commandContract(t)
	p.Actions[0].Kind, p.Actions[0].Command = "http_read", nil
	p.Actions[0].HTTP = &InvestigationHTTP{URL: "https://example.invalid/health?verbose=1", Method: "GET", HeadersEnv: map[string]string{"Authorization": "SERVICE_AUTH"}, TimeoutMS: 1000, MaxBytes: 1024, ExpectStatus: 200}
	return p
}

func TestExplicitInvestigationActionsValidateFrozenAuthority(t *testing.T) {
	for _, p := range []Preparation{commandContract(t), httpContract(t)} {
		loaded, err := ValidatePreparation(preparationJSON(t, p))
		require.NoError(t, err)
		require.Equal(t, p.Actions, loaded.Actions)
	}
	for name, change := range map[string]func(*Preparation){
		"ungranted copy":       func(p *Preparation) { p.Actions[0].Command.Inputs = []string{"secrets"} },
		"empty executable":     func(p *Preparation) { p.Actions[0].Command.Argv[0] = " " },
		"NUL argv":             func(p *Preparation) { p.Actions[0].Command.Argv[1] = "a\x00b" },
		"timeout exceeds task": func(p *Preparation) { p.Actions[0].Command.TimeoutMS = 61000 },
		"file path mixed":      func(p *Preparation) { p.Actions[0].Path = p.ReadPaths[0] },
		"query mixed":          func(p *Preparation) { p.Actions[0].Query = "x" },
		"no host grant":        func(p *Preparation) { p.Actions[0].Command.ExecutionBoundary = "sandbox" },
		"empty argv":           func(p *Preparation) { p.Actions[0].Command.Argv = nil },
		"empty input set":      func(p *Preparation) { p.Actions[0].Command.Inputs = nil },
		"inline env value":     func(p *Preparation) { p.Actions[0].Command.Env = []string{"TOKEN=value"} },
	} {
		t.Run(name, func(t *testing.T) {
			p := commandContract(t)
			change(&p)
			_, err := ValidatePreparation(preparationJSON(t, p))
			require.Error(t, err)
		})
	}
	for name, change := range map[string]func(*Preparation){
		"userinfo":             func(p *Preparation) { p.Actions[0].HTTP.URL = "https://user:secret@example.invalid/" },
		"HTTP destination":     func(p *Preparation) { p.Actions[0].HTTP.URL = "http://example.invalid/" },
		"write method":         func(p *Preparation) { p.Actions[0].HTTP.Method = "POST" },
		"timeout exceeds task": func(p *Preparation) { p.Limits.MaxSeconds = 1; p.Actions[0].HTTP.TimeoutMS = 1001 },
		"output exceeds task":  func(p *Preparation) { p.Actions[0].HTTP.MaxBytes = p.Limits.MaxOutputBytes + 1 },
		"path mixed":           func(p *Preparation) { p.Actions[0].Path = p.ReadPaths[0] },
		"query mixed":          func(p *Preparation) { p.Actions[0].Query = "x" },
		"duplicate header":     func(p *Preparation) { p.Actions[0].HTTP.HeadersEnv["authorization"] = "SECOND_AUTH" },
		"routing header":       func(p *Preparation) { p.Actions[0].HTTP.HeadersEnv["Host"] = "HOST" },
		"compression header":   func(p *Preparation) { p.Actions[0].HTTP.HeadersEnv["Accept-Encoding"] = "ENCODING" },
		"credential literal":   func(p *Preparation) { p.Actions[0].HTTP.HeadersEnv["Authorization"] = "secret value" },
	} {
		t.Run(name, func(t *testing.T) {
			p := httpContract(t)
			change(&p)
			_, err := ValidatePreparation(preparationJSON(t, p))
			require.Error(t, err)
		})
	}
	p := preparationContractFixture(t)
	p.Actions[0].Command = commandContract(t).Actions[0].Command
	_, err := ValidatePreparation(preparationJSON(t, p))
	require.Error(t, err)
	// The typed boundary rejects absent handlers too, before any dispatch.
	for _, a := range []InvestigationAction{{ID: "missing-command", Kind: "run_command"}, {ID: "missing-http", Kind: "http_read"}} {
		require.Error(t, validateInvestigationAction(a, map[string]bool{}, p.Limits))
	}
}

func TestInvestigationHTTPDestinationsAreExactAndBounded(t *testing.T) {
	for _, raw := range []string{"https://service.example/path?query=a", "http://127.0.0.1:1234/health", "http://[::1]/health"} {
		require.NoError(t, ValidateInvestigationURL(raw))
	}
	for _, raw := range []string{"%", "https:///missing-host", "https://x/#fragment", "https://user:pass@x/", "https:opaque", "https://x/\n", "ftp://x/", "http://localhost/", "http://192.0.2.1/"} {
		require.Error(t, ValidateInvestigationURL(raw), raw)
	}
}

func TestBundledDiagnosticExampleUsesCurrentContract(t *testing.T) {
	raw, err := templates.FS.ReadFile("preparation.diagnostics.example.json")
	require.NoError(t, err)
	p, err := ValidatePreparation(raw)
	require.NoError(t, err)
	require.Equal(t, "run_command", p.Actions[0].Kind)
	require.Equal(t, "http_read", p.Actions[1].Kind)
}
