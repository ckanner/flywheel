package state

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func workerPrewarmStateValue() map[string]any {
	return map[string]any{
		"revision": 1,
		"packets": map[string]any{
			"M01-T01": map[string]any{"status": "executed"},
		},
		"sessions": map[string]any{"version": 3, "records": map[string]any{}},
		"worker-prewarm": map[string]any{
			"version": 1, "id": "warm-0001", "scope": "M01-T01", "binding": strings.Repeat("a", 64),
			"started-at": "2026-11-13T00:00:00Z", "engine-run-id": "run-one",
		},
	}
}

func writeWorkerPrewarmState(t *testing.T, value map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	writeRaw(t, StateFile(dir), string(raw))
	return dir
}

func TestWorkerPrewarmLoadKeepsLegacyStatesAndVersionFence(t *testing.T) {
	for _, version := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			value := workerPrewarmStateValue()
			delete(value, "worker-prewarm")
			if version == 0 {
				delete(value, "sessions")
			} else {
				value["sessions"].(map[string]any)["version"] = version
			}
			st, err := Load(writeWorkerPrewarmState(t, value))
			require.NoError(t, err)
			require.Nil(t, st.WorkerPrewarm)
			if version == 0 {
				require.Nil(t, st.Sessions)
			} else {
				require.Equal(t, version, st.Sessions.Version)
			}
		})
	}
}

func TestWorkerPrewarmLoadAndCASPreserveExactResponsibility(t *testing.T) {
	for _, scope := range []string{"M01-T01", "final:M01-T01"} {
		for _, withRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/run=%t", scope, withRun), func(t *testing.T) {
				value := workerPrewarmStateValue()
				warm := value["worker-prewarm"].(map[string]any)
				warm["scope"] = scope
				if !withRun {
					delete(warm, "engine-run-id")
				}
				// Disabling future preparation does not erase an existing obligation.
				value["scheduler"] = map[string]any{"mode": "serial", "max-parallel": 2, "prewarm": false}
				dir := writeWorkerPrewarmState(t, value)
				st, err := Load(dir)
				require.NoError(t, err)
				want := &WorkerPrewarm{Version: 1, ID: "warm-0001", Scope: scope, Binding: strings.Repeat("a", 64), StartedAt: "2026-11-13T00:00:00Z"}
				if withRun {
					want.EngineRunID = "run-one"
				}
				require.Equal(t, want, st.WorkerPrewarm)
				require.Equal(t, 3, st.Sessions.Version)
				next, err := Save(dir, st, nil)
				require.NoError(t, err)
				require.Equal(t, int64(2), next.Revision)
				reloaded, err := Load(dir)
				require.NoError(t, err)
				require.Equal(t, want, reloaded.WorkerPrewarm)
				require.Equal(t, 3, reloaded.Sessions.Version)
				// The caller can discharge a proven cleanup without making the ledger
				// readable by an engine that predates the responsibility contract.
				reloaded.WorkerPrewarm = nil
				_, err = Save(dir, reloaded, nil)
				require.NoError(t, err)
				cleared, err := Load(dir)
				require.NoError(t, err)
				require.Nil(t, cleared.WorkerPrewarm)
				require.Equal(t, 3, cleared.Sessions.Version)
			})
		}
	}
}

func TestWorkerPrewarmLoadRejectsMalformedPreparationBeforeRecovery(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"null", func(v, _ map[string]any) { v["worker-prewarm"] = nil }},
		{"array", func(v, _ map[string]any) { v["worker-prewarm"] = []any{} }},
		{"unknown field", func(_, w map[string]any) { w["receipt-path"] = "/tmp/other-receipt" }},
		{"unknown version", func(_, w map[string]any) { w["version"] = 2 }},
		{"null version", func(_, w map[string]any) { w["version"] = nil }},
		{"fractional version", func(_, w map[string]any) { w["version"] = 1.5 }},
		{"short id", func(_, w map[string]any) { w["id"] = "warm-01" }},
		{"long id", func(_, w map[string]any) { w["id"] = strings.Repeat("a", 129) }},
		{"null id", func(_, w map[string]any) { w["id"] = nil }},
		{"traversal id", func(_, w map[string]any) { w["id"] = "../escape" }},
		{"embedded traversal id", func(_, w map[string]any) { w["id"] = "warm/../other" }},
		{"absolute id", func(_, w map[string]any) { w["id"] = "/tmp/warm-01" }},
		{"backslash id", func(_, w map[string]any) { w["id"] = `warm\other` }},
		{"dot id", func(_, w map[string]any) { w["id"] = "warm.0001" }},
		{"blank scope", func(_, w map[string]any) { w["scope"] = "" }},
		{"milestone scope", func(_, w map[string]any) { w["scope"] = "M01" }},
		{"other role scope", func(_, w map[string]any) { w["scope"] = "execute:M01-T01" }},
		{"traversal scope", func(_, w map[string]any) { w["scope"] = "M01-T01/../M02-T01" }},
		{"nonstring scope", func(_, w map[string]any) { w["scope"] = 1 }},
		{"short binding", func(_, w map[string]any) { w["binding"] = strings.Repeat("a", 63) }},
		{"long binding", func(_, w map[string]any) { w["binding"] = strings.Repeat("a", 65) }},
		{"uppercase binding", func(_, w map[string]any) { w["binding"] = strings.Repeat("A", 64) }},
		{"nonhex binding", func(_, w map[string]any) { w["binding"] = strings.Repeat("g", 64) }},
		{"null binding", func(_, w map[string]any) { w["binding"] = nil }},
		{"blank started at", func(_, w map[string]any) { w["started-at"] = "" }},
		{"null started at", func(_, w map[string]any) { w["started-at"] = nil }},
		{"blank run", func(_, w map[string]any) { w["engine-run-id"] = "" }},
		{"long run", func(_, w map[string]any) { w["engine-run-id"] = strings.Repeat("r", 257) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := workerPrewarmStateValue()
			tc.edit(value, value["worker-prewarm"].(map[string]any))
			st, err := Load(writeWorkerPrewarmState(t, value))
			require.ErrorContains(t, err, "worker-prewarm")
			require.Nil(t, st, "malformed identity must not reach process or receipt recovery")
		})
	}
	for _, name := range []string{"version", "id", "scope", "binding", "started-at"} {
		t.Run("missing "+name, func(t *testing.T) {
			value := workerPrewarmStateValue()
			delete(value["worker-prewarm"].(map[string]any), name)
			_, err := Load(writeWorkerPrewarmState(t, value))
			require.ErrorContains(t, err, "worker-prewarm")
		})
	}
	for _, id := range []string{"warm-001", strings.Repeat("a", 128), "_warm_01"} {
		value := workerPrewarmStateValue()
		warm := value["worker-prewarm"].(map[string]any)
		warm["id"], warm["engine-run-id"] = id, strings.Repeat("r", 256)
		st, err := Load(writeWorkerPrewarmState(t, value))
		require.NoError(t, err)
		require.Equal(t, id, st.WorkerPrewarm.ID)
	}
}

func TestWorkerPrewarmLoadRequiresOriginalPacketAndVersionThreeLedger(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"missing sessions", func(v map[string]any) { delete(v, "sessions") }},
		{"version one", func(v map[string]any) { v["sessions"].(map[string]any)["version"] = 1 }},
		{"version two", func(v map[string]any) { v["sessions"].(map[string]any)["version"] = 2 }},
		{"missing packets", func(v map[string]any) { delete(v, "packets") }},
		{"empty packets", func(v map[string]any) { v["packets"] = map[string]any{} }},
		{"null packet", func(v map[string]any) { v["packets"].(map[string]any)["M01-T01"] = nil }},
		{"different packet", func(v map[string]any) { v["worker-prewarm"].(map[string]any)["scope"] = "M01-T02" }},
		{"different final packet", func(v map[string]any) { v["worker-prewarm"].(map[string]any)["scope"] = "final:M01-T02" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := workerPrewarmStateValue()
			tc.edit(value)
			st, err := Load(writeWorkerPrewarmState(t, value))
			require.ErrorContains(t, err, "worker prewarm requires its packet and a version 3 session ledger")
			require.Nil(t, st)
		})
	}
}
