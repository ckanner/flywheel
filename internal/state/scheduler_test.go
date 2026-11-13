package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerStateAcceptsLegacyAndExplicitPolicies(t *testing.T) {
	for _, raw := range []string{
		`{"mode":"serial","max-parallel":2}`,
		`{"mode":"parallel","max-parallel":2,"dispatch":"batch","prewarm":false}`,
		`{"mode":"parallel","max-parallel":3,"dispatch":"ready-queue","prewarm":true}`,
	} {
		dir := t.TempDir()
		writeRaw(t, StateFile(dir), `{"revision":1,"scheduler":`+raw+`}`)
		st, err := Load(dir)
		require.NoError(t, err)
		require.NotNil(t, st.Scheduler)
	}
	for _, raw := range []string{
		`{"mode":"parallel","max-parallel":2,"dispatch":""}`,
		`{"mode":"parallel","max-parallel":2,"dispatch":"ready"}`,
		`{"mode":"parallel","max-parallel":2,"prewarm":null}`,
		`{"mode":"parallel","max-parallel":2,"prewarm":"true"}`,
	} {
		dir := t.TempDir()
		writeRaw(t, StateFile(dir), `{"revision":1,"scheduler":`+raw+`}`)
		_, err := Load(dir)
		require.Error(t, err)
	}
}
