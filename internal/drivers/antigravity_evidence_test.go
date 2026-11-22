package drivers

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntigravityEvidenceUsesManagedAgentProofInsteadOfCatalogEmptiness(t *testing.T) {
	r := agyRequest(t)
	r.EvidenceOnly, r.PermissionMode = true, "read-only"
	for _, tools := range [][]string{{}, {"run_command", "invoke_subagent"}} {
		m := agyReceipt(r)
		proObject(m, "configuration", "observed")["tools"] = tools
		p := parseAgyFixture(r, m)
		require.True(t, p.Settled, "%+v", p)
		require.Equal(t, "completed", p.Session.Outcome)
		require.Equal(t, "verified answer", p.Text)
		require.True(t, p.Session.CleanupConfirmed)
		require.NoDirExists(t, filepath.Join(r.Cwd, ".agents"), "recovery verifies the receipt, not a removed agent file")
	}
	for name, edit := range map[string]func(map[string]any){
		"missing catalog":          func(m map[string]any) { delete(proObject(m, "configuration", "observed"), "tools") },
		"null catalog":             func(m map[string]any) { proObject(m, "configuration", "observed")["tools"] = nil },
		"missing observed agent":   func(m map[string]any) { delete(proObject(m, "configuration", "observed"), "agent") },
		"different observed agent": func(m map[string]any) { proObject(m, "configuration", "observed")["agent"] = "another-agent" },
		"missing managed agent":    func(m map[string]any) { delete(proObject(m, "native"), "managedAgent") },
		"malformed managed agent":  func(m map[string]any) { proObject(m, "native")["managedAgent"] = "not an attestation" },
		"different managed agent":  func(m map[string]any) { proObject(m, "native", "managedAgent")["name"] = "another-agent" },
		"different owner":          func(m map[string]any) { proObject(m, "native", "managedAgent")["ownerRunId"] = "other-run" },
		"different workspace": func(m map[string]any) {
			proObject(m, "native", "managedAgent")["path"] = filepath.Join(r.Cwd, "another.md")
		},
		"relative path":   func(m map[string]any) { proObject(m, "native", "managedAgent")["path"] = ".agents/agents/evidence.md" },
		"missing digest":  func(m map[string]any) { delete(proObject(m, "native", "managedAgent"), "digest") },
		"invalid digest":  func(m map[string]any) { proObject(m, "native", "managedAgent")["digest"] = strings.Repeat("z", 64) },
		"unknown cleanup": func(m map[string]any) { proObject(m, "native", "managedAgent")["cleanup"] = "unknown" },
		"missing cleanup": func(m map[string]any) { delete(proObject(m, "native", "managedAgent"), "cleanup") },
	} {
		t.Run(name, func(t *testing.T) {
			m := agyReceipt(r)
			edit(m)
			p := parseAgyFixture(r, m)
			require.False(t, p.Settled)
			require.NotEmpty(t, p.Error)
			require.Equal(t, "unknown", p.Session.Outcome)
			require.Empty(t, p.Text, "unproven evidence-only output must not be consumed")
			require.False(t, p.Session.ResumeCandidate)
		})
	}
	for _, runID := range []string{"nested/run", `nested\run`, "line\rbreak", "line\nbreak"} {
		t.Run("unsafe owned agent path "+runID, func(t *testing.T) {
			m := agyReceipt(r)
			name := "dsh-evidence-" + runID
			m["runId"] = runID
			proObject(m, "configuration", "observed")["agent"] = name
			agent := proObject(m, "native", "managedAgent")
			agent["name"], agent["ownerRunId"] = name, runID
			agent["path"] = filepath.Join(r.Cwd, ".agents", "agents", name+".md")
			p := parseAgyFixture(r, m)
			require.False(t, p.Settled)
			require.Equal(t, "unknown", p.Session.Outcome)
			require.Empty(t, p.Text)
			require.False(t, p.Session.ResumeCandidate)
		})
	}
}
