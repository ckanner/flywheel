package shadow

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Run is explicit, bounded and advisory. Neither completed nor failed records
// can alter a task. A reserved call without a terminal is never auto-replayed.
func Run(ctx context.Context, dataset Dataset, directory string, options Options) (Report, error) {
	if _, err := ParseDataset(jsonBytes(dataset)); err != nil {
		return Report{}, err
	}
	if ctx == nil || !validTarget(options) {
		return Report{}, fmt.Errorf("invalid shadow target or limits: use HTTPS /v1/systemone, a pinned expected model, timeout <=30s and max-calls 1..100")
	}
	target := targetFor(options)
	if containsKey(dataset, options.APIKey) || (options.APIKey != "" && strings.Contains(string(jsonBytes(target)), options.APIKey)) {
		return Report{}, fmt.Errorf("credential value found in shadow input or target")
	}
	for _, c := range dataset.Cases {
		if len(requestBody(c, options.Model)) > 64*1024 {
			return Report{}, fmt.Errorf("shadow request exceeds 64 KiB; prepare a smaller reviewed summary")
		}
	}
	root, err := openStore(directory, true)
	if err != nil {
		return Report{}, err
	}
	defer root.Close()
	if err := installTarget(root, target); err != nil {
		return Report{}, err
	}
	needed := 0
	for _, c := range dataset.Cases {
		_, exists, err := loadRecord(root, recordFor(c, target))
		if err != nil {
			return Report{}, err
		}
		if !exists && len(c.HardBlockers) == 0 {
			needed++
		}
	}
	if needed > options.MaxCalls {
		return Report{}, fmt.Errorf("shadow batch needs %d new calls, exceeding max-calls %d", needed, options.MaxCalls)
	}
	if needed > 0 && options.APIKey == "" {
		return Report{}, fmt.Errorf("shadow API key environment variable is empty")
	}
	if options.SaveInput {
		if err := installDataset(root, dataset); err != nil {
			return Report{}, err
		}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	calls := 0
	for _, c := range dataset.Cases {
		if ctx.Err() != nil {
			break
		}
		record := recordFor(c, target)
		_, exists, err := loadRecord(root, record)
		if err != nil {
			return Report{}, err
		}
		if exists {
			continue
		}
		record.At = now().UTC().Format(time.RFC3339Nano)
		owned, err := reserve(root, record)
		if err != nil {
			return Report{}, err
		}
		if !owned {
			continue
		}
		if len(c.HardBlockers) > 0 {
			zero := int64(0)
			record.Status, record.Advice, record.Reason = "skipped", "full", c.HardBlockers[0]
			record.Usage = Usage{&zero, &zero}
		} else {
			start := now()
			call(ctx, c, options, &record)
			record.DurationMS = now().Sub(start).Milliseconds()
			calls++
		}
		if err := publish(root, record.Key+".json", jsonBytes(record)); err != nil {
			return Report{}, err
		}
	}
	report, err := readReport(root, dataset, target)
	report.NewCalls = calls
	report.Cancelled = ctx.Err() != nil
	return report, err
}

func ReadReport(dataset Dataset, directory string) (Report, error) {
	if _, err := ParseDataset(jsonBytes(dataset)); err != nil {
		return Report{}, err
	}
	root, err := openStore(directory, false)
	if err != nil {
		return Report{}, err
	}
	defer root.Close()
	target, err := readTarget(root)
	if err != nil {
		return Report{}, err
	}
	return readReport(root, dataset, target)
}

func readReport(root *os.Root, dataset Dataset, target Target) (Report, error) {
	report := Report{Protocol: 1, Mode: "shadow", Target: target, Rows: []Row{}}
	for _, c := range dataset.Cases {
		record, exists, err := loadRecord(root, recordFor(c, target))
		if err != nil {
			return Report{}, err
		}
		row := Row{ID: c.ID, Source: c.Source, Baseline: c.Baseline, Label: c.Label}
		if exists {
			if record.Status == "skipped" && (len(c.HardBlockers) == 0 || record.Reason != c.HardBlockers[0]) {
				return Report{}, fmt.Errorf("shadow skip does not match deterministic blockers")
			}
			if record.Status == "ok" {
				mode, reason := advice(record.Probabilities, target.Policy)
				if record.Model != target.ExpectedModel || len(record.Probabilities) != len(questions) || record.HTTPStatus != 200 || record.Advice != mode || record.Reason != reason {
					return Report{}, fmt.Errorf("shadow result semantics do not match the recorded policy")
				}
			}
			row.Record = &record
		}
		report.Rows = append(report.Rows, row)
	}
	report.Summary = summarize(report.Rows)
	return report, nil
}
