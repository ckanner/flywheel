package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/spf13/cobra"
)

func readSessionCommand(reader io.Reader) (*machine.SessionCommand, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, 131073))
	if err != nil {
		return nil, err
	}
	if len(raw) > 131072 || !json.Valid(raw) {
		return nil, fmt.Errorf("invalid or oversized session command")
	}
	var request machine.SessionCommand
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	return &request, request.Validate()
}

var sessionDirectoryStat = os.Lstat
var sessionInputOpen = os.OpenFile
var sessionInputRead = io.ReadAll
var sessionInputTemp = os.CreateTemp
var sessionInputWrite = (*os.File).Write
var sessionInputSync = (*os.File).Sync

// The private payload keeps user prompts out of process argv. Its directory is
// volatile ledger space; the engine accepts only the exact task's ordinary file.
func startSessionControl(cmd *cobra.Command, taskDir, cwd string) (controlResponse, error) {
	request, err := readSessionCommand(cmd.InOrStdin())
	if err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	if request.Action == "abandon" {
		// Validate the host-generated proof before spawning anything. The run
		// child re-reads the same environment proof and passes it to the engine.
		if err := sessionActionProof(request, nil, false); err != nil {
			return controlResponse{Protocol: 1, TaskDir: taskDir}, err
		}
	}
	directory := filepath.Join(taskDir, ".control")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	info, err := sessionDirectoryStat(directory)
	if err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, fmt.Errorf("control directory must not be a symlink")
	}
	file, err := sessionInputTemp(directory, "session-request-*")
	if err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	defer file.Close()
	raw, _ := json.Marshal(request)
	if _, err := sessionInputWrite(file, raw); err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	if err := sessionInputSync(file); err != nil {
		return controlResponse{Protocol: 1, TaskDir: taskDir}, err
	}
	quick := request.Action == "abandon" || request.Action == "reconcile" || request.Action == "claim" || request.Action == "release"
	reply, err := startControlledRunWait(taskDir, cwd, "", []string{"--only", "session", "--session-request", file.Name(), "--session-request-digest", fmt.Sprintf("%x", sha256.Sum256(raw)), "--expect-revision", fmt.Sprint(request.Revision)}, quick)
	// The engine may exit semantically (for example escalated) while the control
	// envelope remains ok:true. Surface the exact session outcome instead of
	// making the caller infer it from an unrelated state revision.
	reply.SessionControl = sessionControlReceipt(reply.State, request)
	if err != nil && reply.SessionControl != nil {
		reply.SessionControl.Applied = false
		reply.SessionControl.Message = err.Error()
	}
	// On a launch error the child may still be reading. Leave its bounded private
	// payload until the child consumes it; never erase an ambiguous request.
	return reply, err
}

func loadSessionInput(taskDir, path, digest string) (*machine.SessionCommand, error) {
	root, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(root, ".control")
	if parent != directory || !strings.HasPrefix(filepath.Base(path), "session-request-") {
		return nil, fmt.Errorf("session input must belong to this task's control directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("session input must be a private ordinary file")
	}
	file, err := sessionInputOpen(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, fmt.Errorf("session input identity changed")
	}
	raw, err := sessionInputRead(io.LimitReader(file, 131073))
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != digest {
		return nil, fmt.Errorf("session input content changed")
	}
	request, err := readSessionCommand(strings.NewReader(string(raw)))
	if err == nil {
		err = os.Remove(path)
	}
	return request, err
}
