package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Editor values contain only configuration fields, never credential values or
// arbitrary notes/commands from the source file. Unedited sections stay on disk.
var editorSections = []string{"subscription", "planning", "jev", "loops", "routes", "channels", "profiles", "native-profiles", "execution-policy", "sessions", "permissions", "budget", "watchdog", "retry", "worktree", "scheduler", "judge-panel"}

type EditorDocument struct {
	Target   string                     `json:"target"`
	Scope    string                     `json:"scope"`
	Source   string                     `json:"source"`
	Revision string                     `json:"revision"`
	Values   map[string]json.RawMessage `json:"values"`
	Defaults map[string]json.RawMessage `json:"defaults"`
	Applied  bool                       `json:"applied"`
}

type EditorRequest struct {
	Scope    string                     `json:"scope"`
	Revision string                     `json:"revision"`
	Values   map[string]json.RawMessage `json:"values"`
	Reset    bool                       `json:"reset"`
}

func editorValues(cfg *Config) map[string]json.RawMessage {
	var all map[string]json.RawMessage
	_ = json.Unmarshal(configJSON(cfg), &all)
	for key := range all {
		if !slices.Contains(editorSections, key) {
			delete(all, key)
		}
	}
	return all
}

func editorDocument(cwd, scope string) (*EditorDocument, []byte, os.FileInfo, error) {
	target := filepath.Join(cwd, "flywheel.config.json")
	if scope != "project" && scope != "global" {
		return nil, nil, nil, errors.New("configuration scope must be global or project")
	}
	if scope == "global" {
		path, err := GlobalConfigPath()
		if err != nil {
			return nil, nil, nil, err
		}
		target = path
	}
	target, err := absMigrationPath(target)
	if err != nil {
		return nil, nil, nil, err
	}
	raw, info, err := readMigrationSource(target)
	if err != nil {
		return nil, nil, nil, err
	}
	defaults, err := embeddedTable()
	if err != nil {
		return nil, nil, nil, err
	}
	base, err := parseConfig(defaults, "(embedded legal table)")
	if err != nil {
		return nil, nil, nil, err
	}
	seed, source := raw, target
	if info == nil {
		seed, source = defaults, "(embedded legal table)"
	}
	cfg, err := parseConfig(seed, source)
	if err != nil {
		return nil, nil, nil, err
	}
	return &EditorDocument{Target: target, Scope: scope, Source: source,
		Revision: migrationHash(append([]byte(target+"\x00"), raw...)), Values: editorValues(cfg), Defaults: editorValues(base)}, raw, info, nil
}

func ReadEditor(cwd, scope string) (*EditorDocument, error) {
	document, _, _, err := editorDocument(cwd, scope)
	return document, err
}

// SaveEditor shares migration's lock, optimistic identity check, backup and
// atomic publication. UI retries must reread the target revision first.
func SaveEditor(cwd string, request EditorRequest) (*EditorDocument, error) {
	document, raw, info, err := editorDocument(cwd, request.Scope)
	if err != nil {
		return nil, err
	}
	if request.Revision != document.Revision {
		return document, errors.New("configuration changed; reload and review the new values before saving")
	}
	seed := raw
	if info == nil || request.Reset {
		seed, err = embeddedTable()
		if err != nil {
			return document, err
		}
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(seed, &root) // editorDocument validated both source and template.
	for key, value := range request.Values {
		if !slices.Contains(editorSections, key) {
			return document, fmt.Errorf("configuration editor cannot change section %q", key)
		}
		root[key] = value
	}
	if _, legacy := root["jev-shadow"]; legacy {
		if _, explicit := root["jev"]; !explicit {
			root["jev"] = document.Values["jev"]
		}
		delete(root, "jev-shadow")
	}
	candidate := configJSON(root)
	if _, err := parseConfig(candidate, document.Target); err != nil {
		return document, err
	}
	migration := &Migration{Target: document.Target}
	err = applyMigration(migration, raw, candidate, info)
	document.Applied = migration.Applied
	if err != nil {
		return document, err
	}
	next, err := ReadEditor(cwd, request.Scope)
	if err != nil {
		return document, err
	}
	next.Applied = true
	return next, nil
}
