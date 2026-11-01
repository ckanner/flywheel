package drivers

import (
	"encoding/json"
	"strings"
)

// jsonLines forwards complete stdout lines without retaining an unbounded partial
// line. Watchdog separately marks the full transcript truncated; a dropped line
// can therefore never make a run successful. Call flush only after Run returns.
type jsonLines struct {
	onLine  func(string)
	limit   int64
	pending string
	discard bool
}

func (s *jsonLines) feed(chunk, stream string) {
	if stream != "stdout" {
		return
	}
	s.pending += chunk
	for {
		end := strings.IndexByte(s.pending, '\n')
		if end < 0 {
			break
		}
		if !s.discard && int64(end) <= s.limit {
			s.onLine(s.pending[:end])
		}
		s.pending = s.pending[end+1:]
		s.discard = false
	}
	if int64(len(s.pending)) > s.limit {
		s.pending = ""
		s.discard = true
	}
}

func (s *jsonLines) flush() {
	if s.pending != "" && !s.discard {
		s.onLine(s.pending)
	}
}

// Error events remain errors even when a CLI omits its usual message shape.
func jsonErrorMessage(message, detail json.RawMessage, fallback string) string {
	for _, raw := range []json.RawMessage{message, detail} {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil && strings.TrimSpace(text) != "" {
			return text
		}
		var nested struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(raw, &nested); err == nil && strings.TrimSpace(nested.Message) != "" {
			return nested.Message
		}
	}
	return fallback
}
