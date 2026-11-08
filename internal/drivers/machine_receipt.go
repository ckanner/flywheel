package drivers

import (
	"encoding/json"
	"path/filepath"
)

// Receipt helpers handle only transport-neutral shapes. Each adapter separately
// validates its native execution, permissions and history semantics.
func receiptHasFields(raw json.RawMessage, names ...string) bool {
	var fields map[string]json.RawMessage
	// Callers already decoded this same value into a struct; its only
	// possible shapes are an object or null, which also decode into this map.
	_ = json.Unmarshal(raw, &fields)
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

// Opaque IDs are compared as JSON strings, without interpreting their spelling.
// Null, missing, empty and another thread are not interchangeable.
func requestedSessionMatches(raw json.RawMessage, expected string) bool {
	if expected == "" {
		return string(raw) == "null"
	}
	var id string
	return json.Unmarshal(raw, &id) == nil && id == expected
}

func samePhysicalDirectory(actual, requested string) bool {
	if !filepath.IsAbs(actual) {
		return false
	}
	physical, err := filepath.EvalSymlinks(actual)
	return err == nil && physical == requested
}
