package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ckanner/flywheel/internal/config"
)

type MigrationReport struct {
	*config.Migration
	OK                  bool           `json:"ok"`
	Error               string         `json:"error,omitempty"`
	Cwd                 string         `json:"cwd"`
	TaskDir             string         `json:"taskDir"`
	Current             *Configuration `json:"current,omitempty"`
	Configuration       *Configuration `json:"configuration,omitempty"`
	ConfigurationStatus string         `json:"configurationStatus,omitempty"`
}

// MigrateContext reuses inspection's physical directory and bounded Git-root
// discovery without loading the old config first: a custom loop/route must be
// reported as a migration blocker, not hidden by ordinary schema validation.
func MigrateContext(parent context.Context, options config.MigrationOptions) (report *MigrationReport, resultErr error) {
	ctx, cancel := context.WithTimeout(parent, doctorBudget)
	defer cancel()
	report = &MigrationReport{Migration: &config.Migration{Protocol: 1, Kind: "config-migration", Preset: options.Preset}}
	defer func() {
		report.OK = resultErr == nil && len(report.Blockers) == 0
		if resultErr != nil {
			report.Error = resultErr.Error()
		}
	}()
	if err := ctx.Err(); err != nil {
		return report, err
	}
	cwd, err := resolveDirectory(options.Cwd)
	if err != nil {
		return report, fmt.Errorf("resolve working directory: %w", err)
	}
	if options.TaskDir != "" {
		task := options.TaskDir
		if !filepath.IsAbs(task) {
			task = filepath.Join(cwd, task)
		}
		options.TaskDir, err = resolveDirectory(task)
		if err != nil {
			return report, fmt.Errorf("resolve task directory: %w", err)
		}
	}
	options.Cwd, err = projectRoot(ctx, cwd)
	if err != nil {
		return report, err
	}
	report.Cwd, report.TaskDir = options.Cwd, options.TaskDir
	report.Migration, err = config.Migrate(options)
	if report.Original != nil {
		report.Current = configurationView(report.Original, options.Cwd, options.TaskDir)
	}
	if report.Candidate != nil {
		report.Configuration = configurationView(report.Candidate, options.Cwd, options.TaskDir)
		report.ConfigurationStatus = "proposed"
	}
	if report.Applied {
		// Read the actual effective file again. A project/task file introduced by
		// an external editor must be visible instead of claiming the global wins.
		// Cleanup errors after publication do not turn the actual change back
		// into a proposal: retain the error and inspect what is now on disk.
		var inspectErr error
		report.Configuration, inspectErr = InspectContext(ctx, options.Cwd, options.TaskDir)
		report.ConfigurationStatus = "effective"
		if inspectErr != nil {
			report.ConfigurationStatus = "unconfirmed"
		}
		err = errors.Join(err, inspectErr)
	}
	return report, err
}
