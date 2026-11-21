package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ckanner/flywheel/internal/machine"
	"github.com/ckanner/flywheel/internal/modelcatalog"
	"github.com/spf13/cobra"
)

func controlModels(cmd *cobra.Command, cwd string) (*modelcatalog.Report, error) {
	raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 4097))
	if err != nil {
		return nil, err
	}
	var request modelcatalog.Request
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 4096 || !json.Valid(raw) || decoder.Decode(&request) != nil {
		return nil, fmt.Errorf("models requires one bounded JSON model-catalog request")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("model catalog selectors cannot be null")
		}
	}
	report, err := modelcatalog.List(cmd.Context(), machine.ProjectRoot(cwd), request)
	if err != nil {
		return nil, err
	}
	return &report, nil
}
