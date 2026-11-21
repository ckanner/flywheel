package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ckanner/flywheel/internal/imageinput"
	"github.com/ckanner/flywheel/internal/machine"
	"github.com/spf13/cobra"
)

func controlImages(cmd *cobra.Command, cwd string) (*imageinput.Inspection, error) {
	var request struct {
		Paths []string `json:"paths"`
	}
	raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 8193))
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 8192 || !json.Valid(raw) || decoder.Decode(&request) != nil {
		return nil, fmt.Errorf("images requires a bounded JSON object with one to four repository-relative paths")
	}
	return imageinput.Inspect(machine.ProjectRoot(cwd), request.Paths)
}
