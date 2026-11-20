package contract

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

// PacketCollaboration freezes the reviewed task split. It authorizes bounded
// write scopes, not the quality of a split or hard limits on native processes.
type PacketCollaboration struct {
	Version int                 `json:"version"`
	Tasks   []CollaborationTask `json:"tasks"`
}

type CollaborationTask struct {
	Goal   string   `json:"goal"`
	Writes []string `json:"writes"`
}

// ValidateCollaboration is used both at contract load and before native Swarm
// admission. A missing declaration is valid for ordinary packet execution.
func ValidateCollaboration(packet *Packet) error {
	if packet == nil || packet.Collaboration == nil {
		return nil
	}
	c := packet.Collaboration
	if c.Version != 1 || len(c.Tasks) < 2 || len(c.Tasks) > 8 {
		return fmt.Errorf("collaboration requires version 1 and 2–8 tasks")
	}
	if packet.Schedule == nil || len(packet.Schedule.Writes) == 0 {
		return fmt.Errorf("collaboration requires approved schedule.writes")
	}
	// Check the parent scopes here as well: ParsePacket is also used without
	// LoadPackets' cross-packet schedule validation.
	for _, scope := range packet.Schedule.Writes {
		if !normalizedWriteScope(scope) {
			return fmt.Errorf("collaboration schedule.writes must contain normalized relative paths")
		}
	}
	var previous []string
	for i, task := range c.Tasks {
		if strings.TrimSpace(task.Goal) == "" || utf8.RuneCountInString(task.Goal) > 512 || len(task.Writes) == 0 || len(task.Writes) > 64 {
			return fmt.Errorf("collaboration task %d requires a bounded goal and 1–64 write scopes", i+1)
		}
		seen := map[string]bool{}
		for _, scope := range task.Writes {
			clean := strings.TrimSuffix(scope, "/")
			if !normalizedWriteScope(scope) || seen[clean] || !InWriteScope(clean, packet.Schedule.Writes) {
				return fmt.Errorf("collaboration task %d write scope %q must be unique, normalized and within schedule.writes", i+1, scope)
			}
			for _, other := range previous {
				if InWriteScope(clean, []string{other}) || InWriteScope(other, []string{clean}) {
					return fmt.Errorf("collaboration task %d write scope %q overlaps another task", i+1, scope)
				}
			}
			seen[clean] = true
		}
		for scope := range seen {
			previous = append(previous, scope)
		}
	}
	return nil
}

func normalizedWriteScope(scope string) bool {
	clean := strings.TrimSuffix(scope, "/")
	return clean != "" && clean != "." && clean != ".." && !path.IsAbs(clean) && path.Clean(clean) == clean &&
		!strings.HasPrefix(clean, "../") && !strings.ContainsAny(clean, "\\\x00\r\n*?[]")
}
