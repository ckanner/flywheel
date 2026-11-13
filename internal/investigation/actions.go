// Package investigation owns bounded evidence collection before a legal Spec
// exists. Go file inspection and explicitly approved command/HTTP actions share
// frozen authority, durable invocation receipts, and bounded cancellation.
package investigation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/state"
)

func digest(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

var openInspectionFile = (*os.Root).OpenFile
var readInspectionDirectory = (*os.File).ReadDir
var readInspectionBytes = io.ReadAll

// os.Root confines traversal to the selected repository. Symlink components
// and nonregular leaves are rejected; directory grants reveal names only.
func readPath(ctx context.Context, cwd, path string, limit int) ([]byte, bool, error) {
	raw, directory, _, err := readPathWithMode(ctx, cwd, path, limit)
	return raw, directory, err
}

func readPathWithMode(ctx context.Context, cwd, path string, limit int) ([]byte, bool, os.FileMode, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, 0, err
	}
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return nil, false, 0, err
	}
	defer root.Close()
	parts := strings.Split(filepath.ToSlash(path), "/")
	var expected os.FileInfo
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, false, 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, false, 0, fmt.Errorf("symlink reads are not allowed")
		}
		expected = info
	}
	f, err := openInspectionFile(root, path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, false, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, 0, err
	}
	// O_NOFOLLOW protects the leaf, while os.Root keeps traversal inside cwd.
	// Compare the opened inode too: a parent could have been replaced with an
	// in-root symlink after Lstat, redirecting an exact grant to another file.
	if !os.SameFile(expected, info) {
		return nil, false, 0, fmt.Errorf("inspection path changed before opening")
	}
	if info.IsDir() {
		entries, err := readInspectionDirectory(f, 4097)
		if err != nil && err != io.EOF {
			return nil, false, 0, err
		}
		if len(entries) > 4096 {
			return nil, false, 0, fmt.Errorf("directory exceeds 4096-name inspection limit")
		}
		names := []string{}
		for _, entry := range entries {
			if !utf8.ValidString(entry.Name()) {
				return nil, false, 0, fmt.Errorf("inspection requires UTF-8 names")
			}
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		// JSON keeps embedded newlines distinct from two different names.
		raw, _ := json.Marshal(names)
		return raw, true, info.Mode().Perm(), ctx.Err()
	}
	if !info.Mode().IsRegular() {
		return nil, false, 0, fmt.Errorf("only regular files and directory names can be inspected")
	}
	raw, err := readInspectionBytes(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, false, 0, err
	}
	if len(raw) > limit {
		return nil, false, 0, fmt.Errorf("file exceeds approved inspection size")
	}
	if !utf8.Valid(raw) {
		return nil, false, 0, fmt.Errorf("inspection requires UTF-8 text")
	}
	return raw, false, info.Mode().Perm(), ctx.Err()
}

func candidate(ctx context.Context, st *state.PreparationState) (string, error) {
	facts := map[string]string{}
	executableInputs := map[string]bool{}
	for _, action := range st.Contract.Actions {
		if action.Command != nil {
			for _, input := range action.Command.Inputs {
				executableInputs[input] = true
			}
		}
	}
	for _, path := range st.Contract.ReadPaths {
		raw, dir, mode, err := readPathWithMode(ctx, st.Cwd, path, st.Contract.Limits.MaxFileBytes)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			// Missing/inaccessible inputs are explicit observations, never success.
			facts[path] = "unavailable:" + err.Error()
		} else {
			facts[path] = fmt.Sprintf("directory=%t:%s", dir, digest(raw))
			if executableInputs[path] {
				facts[path] += fmt.Sprintf(":mode=%o", mode)
			}
		}
	}
	return state.PreparationHash(facts), nil
}

func executeAction(ctx context.Context, cwd string, action contract.InvestigationAction, limits contract.InvestigationLimits, remaining int) (string, string, bool, error) {
	raw, directory, err := readPath(ctx, cwd, action.Path, limits.MaxFileBytes)
	if err != nil {
		return "", "", false, err
	}
	if (action.Kind == "list_files") != directory {
		return "", "", false, fmt.Errorf("action kind does not match file type")
	}
	contentHash := digest(raw)
	output := string(raw)
	if action.Kind == "search_text" {
		lines := []string{}
		for i, line := range strings.Split(output, "\n") {
			if strings.Contains(line, action.Query) {
				lines = append(lines, fmt.Sprintf("%d:%s", i+1, line))
			}
		}
		output = strings.Join(lines, "\n")
	}
	truncated := len(output) > remaining
	if truncated {
		output = output[:remaining]
		for !utf8.ValidString(output) {
			output = output[:len(output)-1]
		}
	}
	return output, contentHash, truncated, nil
}
