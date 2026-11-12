package state

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var openPreparationEntry = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
	return root.OpenFile(name, flags, mode)
}

// Pin the directory and inspect the opened descriptor. A path replaced after a
// size check must not turn a bounded receipt read into a symlink/device read.
func readPreparationData(dir, relative string, limit int) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := preparationParents(root, relative); err != nil {
		return nil, err
	}
	f, err := openPreparationEntry(root, relative, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("preparation record must be a regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, fmt.Errorf("preparation record exceeds %d bytes", limit)
	}
	return raw, nil
}

func preparationParents(root *os.Root, relative string) error {
	if !filepath.IsLocal(relative) || filepath.Clean(relative) != relative {
		return fmt.Errorf("invalid preparation record path")
	}
	parts := strings.Split(filepath.Dir(relative), string(filepath.Separator))
	for i := range parts {
		info, err := root.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("preparation record parent must be a real directory")
		}
	}
	return nil
}

var renamePreparationEntry = (*os.Root).Rename
var linkPreparationEntry = (*os.Root).Link

// State replacement uses atomic rename; an immutable receipt uses exclusive
// link publication. Neither may overwrite an unknown temporary entry, and both
// flush file contents and the actual opened parent directory before success.
func writePreparationData(dir, relative string, raw []byte, exclusive bool) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := preparationParents(root, relative); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", relative, os.Getpid())
	f, err := openPreparationEntry(root, tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	defer root.Remove(tmp)
	if _, err := f.Write(raw); err != nil {
		return err
	}
	if err := syncStateFile(f); err != nil {
		return err
	}
	if exclusive {
		err = linkPreparationEntry(root, tmp, relative)
	} else {
		err = renamePreparationEntry(root, tmp, relative)
	}
	if err != nil {
		return err
	}
	parent, err := root.Open(filepath.Dir(relative))
	if err != nil {
		return err
	}
	defer parent.Close()
	return syncStateFile(parent)
}
