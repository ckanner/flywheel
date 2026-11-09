package contract

import (
	"fmt"
	"path"
	"strings"
)

// Dependencies point to earlier contract units; this keeps the persisted order
// deterministic and makes cycles impossible without a second graph algorithm.
func ValidateSchedules(packets []*Packet) error {
	seen := map[string]bool{}
	for _, packet := range packets {
		if schedule := packet.Schedule; schedule != nil {
			for _, dependency := range schedule.After {
				if !seen[dependency] {
					return fmt.Errorf("%s: dependency %s must be an earlier packet", packet.PacketID, dependency)
				}
			}
			for _, scope := range schedule.Writes {
				clean := strings.TrimSuffix(scope, "/")
				if clean == "" || clean == "." || path.IsAbs(clean) || path.Clean(clean) != clean || strings.HasPrefix(clean, "../") || strings.ContainsAny(clean, "\\\x00\r\n*?[]") {
					return fmt.Errorf("%s: write scope must be a normalized relative path: %q", packet.PacketID, scope)
				}
			}
		}
		seen[packet.PacketID] = true
	}
	return nil
}

func InWriteScope(file string, scopes []string) bool {
	for _, scope := range scopes {
		prefix := strings.TrimSuffix(scope, "/")
		if file == prefix || strings.HasPrefix(file, prefix+"/") {
			return true
		}
	}
	return false
}
