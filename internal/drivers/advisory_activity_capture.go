package drivers

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/ckanner/flywheel/internal/watchdog"
)

// This deliberately invalid JSONL marker makes a failed observation check
// survive final parsing even if an earlier sequence was already omitted.
const advisoryCaptureRejected = "[flywheel: advisory activity capture rejected invalid stream]\n"

type advisoryActivityCapture struct {
	classify      func(json.RawMessage) (bool, error)
	terminalTypes []string
	frameLimit    int
	pending       []byte
	latest        []byte
	terminal      bool
	failed        bool
}

// The classifier sees every complete JSON object, so SDK adapters can bind
// activity to a preceding ready/session frame. It returns true only for a
// strictly validated advisory activity. Other frames stay byte-for-byte intact.
// Retain the newest observation as a bounded diagnostic at terminal/EOF; never
// retain every token's metadata for the whole invocation.
func newAdvisoryActivityCapture(classify func(json.RawMessage) (bool, error), terminalTypes ...string) watchdog.OutputCapture {
	if len(terminalTypes) == 0 {
		terminalTypes = []string{"result"}
	}
	return &advisoryActivityCapture{classify: classify, terminalTypes: terminalTypes, frameLimit: int(watchdog.MaxBuffer)}
}

func (c *advisoryActivityCapture) Filter(chunk []byte) []byte {
	if c.failed {
		return chunk
	}
	var output []byte
	for len(chunk) != 0 {
		end := bytes.IndexByte(chunk, '\n')
		n := len(chunk)
		if end >= 0 {
			n = end + 1
		}
		if len(c.pending)+n > c.frameLimit {
			output = append(output, c.reject(c.pending)...)
			c.pending = nil
			return append(output, chunk...)
		}
		c.pending = append(c.pending, chunk[:n]...)
		chunk = chunk[n:]
		if end < 0 {
			break
		}
		line := c.pending
		c.pending = nil
		output = append(output, c.frame(line)...)
		if c.failed {
			return append(output, chunk...)
		}
	}
	return output
}

func (c *advisoryActivityCapture) frame(raw []byte) []byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw
	}
	var header struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &header) != nil || c.terminal {
		return c.reject(raw)
	}
	omit, err := c.classify(raw)
	observation := header.Type == "activity" || header.Type == "event"
	if err != nil || (omit && !observation) || (header.Type == "activity" && !omit) {
		return c.reject(raw)
	}
	if omit {
		c.latest = append(c.latest[:0], raw...)
		return nil
	}
	if slices.Contains(c.terminalTypes, header.Type) {
		c.terminal = true
		output := append(c.latest, raw...)
		c.latest = nil
		return output
	}
	return raw
}

func (c *advisoryActivityCapture) reject(raw []byte) []byte {
	c.failed = true
	output := append(c.latest, advisoryCaptureRejected...)
	c.latest = nil
	return append(output, raw...)
}

func (c *advisoryActivityCapture) Flush() []byte {
	output := c.latest
	c.latest = nil
	if len(bytes.TrimSpace(c.pending)) != 0 {
		// A valid-looking but unterminated JSON document is still a damaged
		// protocol frame. Final parsing must not turn it into a clean result.
		c.failed = true
		output = append(output, advisoryCaptureRejected...)
	}
	output = append(output, c.pending...)
	c.pending = nil
	return output
}

func newProActivityCapture(harness string, request codexProStart) watchdog.OutputCapture {
	var sequence int64
	return newAdvisoryActivityCapture(func(raw json.RawMessage) (bool, error) {
		var header struct {
			Type string `json:"type"`
		}
		// The capture already decoded this exact header successfully.
		_ = json.Unmarshal(raw, &header)
		if header.Type != "activity" {
			return false, nil
		}
		return true, validateProActivityEvent(raw, harness, request, &sequence)
	})
}
