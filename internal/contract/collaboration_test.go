package contract

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func collaborationPacket() *Packet {
	return &Packet{Schedule: &PacketSchedule{Writes: []string{"src/"}}, Collaboration: &PacketCollaboration{
		Version: 1, Tasks: []CollaborationTask{{Goal: "Implement the parser", Writes: []string{"src/parser/"}}, {Goal: "Implement the formatter", Writes: []string{"src/formatter.go"}}},
	}}
}

func TestCollaborationRequiresReviewedDisjointWriteScopes(t *testing.T) {
	require.NoError(t, ValidateCollaboration(nil))
	require.NoError(t, ValidateCollaboration(&Packet{}))
	require.NoError(t, ValidateCollaboration(collaborationPacket()))
	for name, change := range map[string]func(*Packet){
		"version":              func(p *Packet) { p.Collaboration.Version = 2 },
		"one task":             func(p *Packet) { p.Collaboration.Tasks = p.Collaboration.Tasks[:1] },
		"too many tasks":       func(p *Packet) { p.Collaboration.Tasks = make([]CollaborationTask, 9) },
		"missing schedule":     func(p *Packet) { p.Schedule = nil },
		"empty parent scopes":  func(p *Packet) { p.Schedule.Writes = nil },
		"invalid parent scope": func(p *Packet) { p.Schedule.Writes = []string{"../src"} },
		"blank goal":           func(p *Packet) { p.Collaboration.Tasks[0].Goal = " \t" },
		"long goal":            func(p *Packet) { p.Collaboration.Tasks[0].Goal = strings.Repeat("界", 513) },
		"empty scopes":         func(p *Packet) { p.Collaboration.Tasks[0].Writes = nil },
		"too many scopes":      func(p *Packet) { p.Collaboration.Tasks[0].Writes = make([]string, 65) },
		"outside parent":       func(p *Packet) { p.Collaboration.Tasks[0].Writes = []string{"tests/parser.go"} },
		"parent is too narrow": func(p *Packet) { p.Schedule.Writes = []string{"src/parser/file.go", "src/formatter.go"} },
		"duplicate scope":      func(p *Packet) { p.Collaboration.Tasks[0].Writes = []string{"src/parser", "src/parser/"} },
		"equal scopes":         func(p *Packet) { p.Collaboration.Tasks[1].Writes = []string{"src/parser"} },
		"overlap descendant":   func(p *Packet) { p.Collaboration.Tasks[1].Writes = []string{"src/parser/file.go"} },
		"overlap ancestor":     func(p *Packet) { p.Collaboration.Tasks[1].Writes = []string{"src"} },
		"invalid path":         func(p *Packet) { p.Collaboration.Tasks[0].Writes = []string{"src/../parser"} },
	} {
		t.Run(name, func(t *testing.T) {
			p := collaborationPacket()
			change(p)
			require.Error(t, ValidateCollaboration(p))
		})
	}
	for _, scope := range []string{"", ".", "..", "../x", "/src", "src//x", "src/./x", "src\\x", "src\x00x", "src\nx", "src/*", "src/[a]"} {
		require.False(t, normalizedWriteScope(scope), scope)
	}
	require.True(t, normalizedWriteScope("src/file.go"))
	p := collaborationPacket()
	p.Collaboration.Tasks[0].Goal = strings.Repeat("界", 512)
	p.Collaboration.Tasks[0].Writes = []string{"src/parser", "src/parser_test.go"}
	require.NoError(t, ValidateCollaboration(p), "UTF-8 goals and distinct prefix siblings are allowed")
}

func TestCollaborationIsLoadedAndCannotUseQuick(t *testing.T) {
	declaration := "\nschedule: {after: [], writes: [src]}\ncollaboration: {version: 1, tasks: [{goal: Parser, writes: [src/parser]}, {goal: Formatter, writes: [src/formatter]}]}"
	content := strings.Replace(validPacket, "contract-version: 1", "contract-version: 1"+declaration, 1)
	packet, err := ParsePacket([]byte(content), "task-packet-M01-T01.md")
	require.NoError(t, err)
	require.Len(t, packet.Collaboration.Tasks, 2)
	for _, bad := range []string{
		strings.Replace(content, "writes: [src/formatter]", "writes: [src/parser/subdir]", 1),
		strings.Replace(content, "collaboration: {version: 1", "collaboration: {version: 2", 1),
		strings.Replace(content, "goal: Formatter", "goal: ' '", 1),
		strings.Replace(content, "collaboration: {version: 1", "collaboration: {unreviewed: true, version: 1", 1),
	} {
		_, err = ParsePacket([]byte(bad), "task-packet-M01-T01.md")
		require.Error(t, err)
	}
	spec, packets := qualifiedWorkflowContract()
	packets[0].Collaboration = packet.Collaboration
	for _, requested := range []string{"auto", "quick"} {
		selected, err := SelectWorkflow(spec, packets, requested)
		require.NoError(t, err)
		require.Equal(t, "full", selected.Effective)
		require.Contains(t, selected.Reason, "collaboration")
	}
}
