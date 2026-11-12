package machine

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/ckanner/flywheel/internal/investigation"
	"github.com/ckanner/flywheel/internal/state"
)

// RunInvestigation shares the implementation engine's lifetime lease and
// token-bound cancellation. It never creates a Spec, packet or completed task.
func RunInvestigation(parent context.Context, dir string, revision int64) (result *state.PreparationState, runErr error) {
	release, err := AcquireRunLease(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	clearPid(dir)
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	preparation, err := state.LoadPreparation(dir)
	if err != nil {
		return nil, err
	}
	c := &Context{TaskDir: dir, Now: time.Now, runID: preparation.RunID, metricsFile: filepath.Join(dir, "metrics.jsonl")}
	finish := c.measurePhase("investigation", preparation.Contract.ID)
	defer func() { finish(); runErr = errors.Join(runErr, c.err()) }()
	if c.err() != nil {
		return nil, c.err()
	}
	if err := c.writePid(); err != nil {
		return nil, err
	}
	defer clearPid(dir)
	stop := watchStopRequests(ctx, dir, c.controlToken, cancel)
	defer stop()
	return investigation.Run(ctx, dir, investigation.Options{ExpectedRevision: revision})
}
