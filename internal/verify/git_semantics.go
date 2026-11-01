package verify

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ckanner/flywheel/internal/childenv"
)

// Attribute lookup is one NUL-delimited batch, independent of repository size.
// Git decides the effective per-path attributes, including info/global overrides.
var gitCheckAttributes = func(cwd, paths string) (string, error) {
	cmd := exec.Command("git", "-C", cwd, "check-attr", "--stdin", "-z", "filter", "working-tree-encoding")
	cmd.Env = envSlice(childenv.GateEnv(childenv.OSLookupEnv, nil))
	cmd.Stdin = strings.NewReader(paths)
	out, err := cmd.Output()
	return string(out), err
}

// checkGitSemantics refuses index/attribute modes that can conceal a different
// persisted blob from a normal HEAD-to-worktree diff. It never changes user flags
// or filter configuration, and does not claim independent byte-level tree proof.
func checkGitSemantics(cwd string, pathspec []string) error {
	raw, err := gitOut(cwd, append([]string{"ls-files", "-v", "-z"}, pathspec...)...)
	if err != nil {
		return fmt.Errorf("inspect candidate Git index flags: %w", err)
	}
	paths := map[string]bool{}
	var input strings.Builder
	for _, entry := range strings.Split(raw, "\x00") {
		if entry == "" {
			continue
		}
		if len(entry) < 3 || entry[1] != ' ' {
			return fmt.Errorf("invalid Git index flag inventory")
		}
		path := entry[2:]
		if entry[0] == 'S' || entry[0] >= 'a' && entry[0] <= 'z' {
			return fmt.Errorf("unsupported Git index flag on %q: assume-unchanged/skip-worktree can conceal candidate changes", path)
		}
		if !paths[path] {
			paths[path] = true
			input.WriteString(path)
			input.WriteByte(0)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	raw, err = gitCheckAttributes(cwd, input.String())
	if err != nil {
		return fmt.Errorf("inspect candidate Git attributes: %w", err)
	}
	fields := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	if len(fields) != len(paths)*6 { // path / attribute / value, two requested attributes per path
		return fmt.Errorf("incomplete Git attribute inventory")
	}
	seen, filters := map[string]bool{}, map[string]string{}
	for i := 0; i < len(fields); i += 3 {
		path, attribute, value := fields[i], fields[i+1], fields[i+2]
		key := path + "\x00" + attribute
		if !paths[path] || seen[key] || attribute != "filter" && attribute != "working-tree-encoding" {
			return fmt.Errorf("invalid Git attribute inventory")
		}
		seen[key] = true
		if value == "unspecified" || value == "unset" {
			continue
		}
		if attribute == "working-tree-encoding" {
			return fmt.Errorf("unsupported Git working-tree-encoding on %q: persisted-tree verification requires an explicit encoding strategy", path)
		}
		filters[value] = path
	}
	if len(filters) == 0 {
		return nil
	}
	// Read only filter conversion settings, not unrelated (possibly sensitive) Git
	// configuration. Exit 1 is Git's documented no-matching-config result.
	raw, err = gitOut(cwd, "config", "--null", "--get-regexp", `^filter\..*\.(clean|process)$`)
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return fmt.Errorf("inspect Git clean filter configuration: %w", err)
		}
	}
	for _, entry := range strings.Split(raw, "\x00") {
		if entry == "" {
			continue
		}
		key, value, ok := strings.Cut(entry, "\n")
		dot := strings.LastIndexByte(key, '.')
		if !ok || !strings.HasPrefix(key, "filter.") || dot < len("filter.") || key[dot+1:] != "clean" && key[dot+1:] != "process" {
			return fmt.Errorf("invalid Git clean filter configuration inventory")
		}
		if path, selected := filters[key[len("filter."):dot]]; selected && value != "" {
			return fmt.Errorf("unsupported Git clean/process filter on %q: persisted-tree verification requires an explicit filter strategy", path)
		}
	}
	return nil
}
