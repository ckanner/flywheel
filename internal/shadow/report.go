package shadow

import (
	"math"
	"sort"
	"strings"
)

type Row struct {
	ID       string   `json:"id"`
	Source   Source   `json:"source"`
	Baseline Baseline `json:"baseline"`
	Label    *Label   `json:"label,omitempty"`
	Record   *Record  `json:"record"`
}
type LabelCounts struct {
	Cases           int      `json:"cases"`
	Evaluated       int      `json:"evaluated"`
	Decided         int      `json:"decided"`
	ExpectedQuick   int      `json:"expected-quick"`
	ExpectedFull    int      `json:"expected-full"`
	QuickCandidates int      `json:"quick-candidates"`
	Agreed          int      `json:"agreed"`
	FalseQuick      int      `json:"false-quick"`
	MissedQuick     int      `json:"missed-quick"`
	FalseQuickRate  *float64 `json:"false-quick-per-labelled-full"`
	QuickCoverage   *float64 `json:"labelled-quick-coverage"`
}
type Summary struct {
	Cases                int                    `json:"cases"`
	Completed            int                    `json:"completed"`
	Unknown              int                    `json:"unknown"`
	NotRun               int                    `json:"not-run"`
	Errors               int                    `json:"errors"`
	Skipped              int                    `json:"skipped"`
	QuickCandidates      int                    `json:"quick-candidates"`
	FullCandidates       int                    `json:"full-candidates"`
	Abstentions          int                    `json:"abstentions"`
	Compared             int                    `json:"compared-with-baseline"`
	Disagreements        int                    `json:"baseline-disagreements"`
	KnownInputTokens     int64                  `json:"known-input-tokens"`
	KnownOutputTokens    int64                  `json:"known-output-tokens"`
	UnknownInputCalls    int                    `json:"unknown-input-calls"`
	UnknownOutputCalls   int                    `json:"unknown-output-calls"`
	P50MS                *int64                 `json:"p50-ms"`
	P95MS                *int64                 `json:"p95-ms"`
	Labels               map[string]LabelCounts `json:"labels"`
	ProductionCalibrated bool                   `json:"production-calibrated"`
}
type Report struct {
	Protocol  int     `json:"protocol"`
	Mode      string  `json:"mode"`
	Target    Target  `json:"target"`
	NewCalls  int     `json:"new-calls"`
	Cancelled bool    `json:"cancelled"`
	Summary   Summary `json:"summary"`
	Rows      []Row   `json:"rows"`
}

// Every metric is per distinct dataset case. Provisional labels and synthetic
// examples remain separate; baseline agreement never becomes an accuracy label.
func summarize(rows []Row) Summary {
	s := Summary{Cases: len(rows), Labels: map[string]LabelCounts{}}
	var elapsed []int64
	for _, row := range rows {
		r := row.Record
		countLabel(&s, row)
		if r == nil {
			s.NotRun++
			continue
		}
		if r.Status == "pending" {
			s.Unknown++
		} else {
			s.Completed++
		}
		if r.Status == "error" {
			s.Errors++
		}
		if r.Status == "skipped" {
			s.Skipped++
		} else {
			if r.Usage.Input == nil {
				s.UnknownInputCalls++
			} else {
				s.KnownInputTokens += *r.Usage.Input
			}
			if r.Usage.Output == nil {
				s.UnknownOutputCalls++
			} else {
				s.KnownOutputTokens += *r.Usage.Output
			}
		}
		if r.Status == "ok" {
			elapsed = append(elapsed, r.DurationMS)
		}
		switch r.Advice {
		case "quick":
			s.QuickCandidates++
		case "full":
			s.FullCandidates++
		case "abstain":
			s.Abstentions++
		}
		if r.Advice != "abstain" && row.Baseline.Mode != "unknown" {
			s.Compared++
			if r.Advice != row.Baseline.Mode {
				s.Disagreements++
			}
		}
	}
	for key, counts := range s.Labels {
		if strings.HasSuffix(key, "/confirmed") && counts.Cases == counts.Evaluated {
			counts.FalseQuickRate = fraction(counts.FalseQuick, counts.ExpectedFull)
			counts.QuickCoverage = fraction(counts.ExpectedQuick-counts.MissedQuick, counts.ExpectedQuick)
		}
		s.Labels[key] = counts
	}
	if len(elapsed) > 0 {
		sort.Slice(elapsed, func(i, j int) bool { return elapsed[i] < elapsed[j] })
		median := elapsed[(len(elapsed)-1)/2]
		if len(elapsed)%2 == 0 {
			median += (elapsed[len(elapsed)/2] - median) / 2
		}
		p95 := elapsed[int(math.Ceil(float64(len(elapsed))*0.95))-1]
		s.P50MS, s.P95MS = &median, &p95
	}
	return s
}

func fraction(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	value := float64(n) / float64(d)
	return &value
}

func countLabel(summary *Summary, row Row) {
	if row.Label == nil {
		return
	}
	key := row.Source.Kind + "/" + row.Label.Status
	c := summary.Labels[key]
	c.Cases++
	if row.Label.Mode == "quick" {
		c.ExpectedQuick++
	} else {
		c.ExpectedFull++
	}
	if r := row.Record; r != nil && (r.Status == "ok" || r.Status == "skipped") {
		c.Evaluated++
		if r.Advice != "abstain" {
			c.Decided++
			if r.Advice == row.Label.Mode {
				c.Agreed++
			}
		}
		if r.Advice == "quick" {
			c.QuickCandidates++
			if row.Label.Mode == "full" {
				c.FalseQuick++
			}
		}
		if row.Label.Mode == "quick" && r.Advice != "quick" {
			c.MissedQuick++
		}
	}
	summary.Labels[key] = c
}
