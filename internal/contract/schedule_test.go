package contract

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduleDependenciesAndWriteScopesAreMechanical(t *testing.T) {
	first := &Packet{PacketID: "M01-T01", Schedule: &PacketSchedule{After: []string{}, Writes: []string{"src/a/"}}}
	second := &Packet{PacketID: "M01-T02", Schedule: &PacketSchedule{After: []string{"M01-T01"}, Writes: []string{"src/b"}}}
	require.NoError(t, ValidateSchedules([]*Packet{first, second, {PacketID: "M01-T03"}}))
	require.True(t, InWriteScope("src/a/file.go", first.Schedule.Writes))
	require.True(t, InWriteScope("src/b", second.Schedule.Writes))
	require.False(t, InWriteScope("src/ab/file.go", first.Schedule.Writes))
	for _, scope := range []string{"/tmp", "", ".", "../outside", "src/../outside", "src//a", "src/*", "src\\a", "src\nfile"} {
		first.Schedule.Writes = []string{scope}
		require.Error(t, ValidateSchedules([]*Packet{first}))
	}
	first.Schedule.Writes = []string{"src/a"}
	require.ErrorContains(t, ValidateSchedules([]*Packet{second, first}), "earlier packet")
}

func TestPacketLoadRejectsUndeclaredDependency(t *testing.T) {
	root := t.TempDir()
	content := strings.Replace(validPacket, "route: default", "route: default\nschedule: {after: [M01-T99], writes: [src/auth/]}", 1)
	require.NoError(t, os.WriteFile(filepath.Join(root, "task-packet-M01-T01.md"), []byte(content), 0600))
	_, err := LoadPackets(root, nil)
	require.ErrorContains(t, err, "earlier packet")
}
