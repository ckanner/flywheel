package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type globalConfigTempFile interface {
	io.WriteCloser
	Name() string
}

// Narrow I/O seams keep write/close/publication failures testable while the
// regression tests still use real temporary files and atomic filesystem links.
var (
	createGlobalConfigTemp = func(dir string) (globalConfigTempFile, error) {
		return os.CreateTemp(dir, ".flywheel-config-*.tmp")
	}
	linkGlobalConfigTemp   = os.Link
	removeGlobalConfigTemp = os.Remove
)

// materializeGlobalFile publishes only a completely written, closed file. Link
// creates the destination atomically and never replaces a racing user file or
// symlink. The temporary name is on the same filesystem as the final path, and
// its private permissions are retained when publishing a new configuration.
func materializeGlobalFile(path string, raw []byte) (created bool, resultErr error) {
	temp, err := createGlobalConfigTemp(filepath.Dir(path))
	if err != nil {
		return false, err
	}
	defer func() {
		_ = temp.Close() // Also closes the descriptor on a pre-publication failure.
		if err := removeGlobalConfigTemp(temp.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("cannot remove own configuration temporary file %s: %w", temp.Name(), err))
		}
	}()
	if _, err := io.Copy(temp, bytes.NewReader(raw)); err != nil {
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := linkGlobalConfigTemp(temp.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("publish new global configuration without replacing an existing entry: %w", err)
	}
	return true, nil
}
