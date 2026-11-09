package metrics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
)

// Report refines receipts by invocation identity before aggregating them. Missing
// observations remain counted, including subscriptions without a reported price.
type Report struct {
	TotalCalls    int     `json:"totalCalls"`
	OmittedCalls  int     `json:"omittedCalls"`
	OmittedGroups int     `json:"omittedGroups"`
	Calls         []Call  `json:"calls"`
	Groups        []Group `json:"groups"`
}
type Call struct {
	Session *struct {
		Last struct {
			UsageStatus string `json:"usage-status"`
		} `json:"last"`
	} `json:"session,omitempty"`
	Invocation string     `json:"invocation-id"`
	Receipt    string     `json:"receipt-sha256"`
	Kind       string     `json:"record-kind"`
	Reconciles string     `json:"reconciles-invocation"`
	Supersedes string     `json:"supersedes-receipt"`
	Run        string     `json:"run-id"`
	Role       string     `json:"role"`
	Packet     string     `json:"packet-id"`
	Harness    string     `json:"harness"`
	Model      string     `json:"model"`
	Provider   string     `json:"provider"`
	Usage      *CallUsage `json:"usage"`
	Cost       *float64   `json:"cost-usd"`
	Duration   *int64     `json:"duration-ms"`
	Error      string     `json:"error-code"`
}
type CallUsage struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Total  int64 `json:"totalTokens"`
}
type Group struct {
	PartialUsage    int     `json:"partialUsage"`
	Run             string  `json:"runId"`
	Role            string  `json:"role"`
	Harness         string  `json:"harness"`
	Model           string  `json:"model"`
	Provider        string  `json:"provider"`
	Calls           int     `json:"calls"`
	Failed          int     `json:"failedCalls"`
	Tokens          int64   `json:"knownTokens"`
	Cost            float64 `json:"knownCostUSD"`
	Duration        int64   `json:"knownDurationMs"`
	MissingUsage    int     `json:"missingUsage"`
	MissingCost     int     `json:"missingCost"`
	MissingDuration int     `json:"missingDuration"`
}

func ReadReport(file string, runIDs ...string) (*Report, error) {
	input, err := os.Open(file)
	if os.IsNotExist(err) {
		return &Report{Calls: []Call{}, Groups: []Group{}}, nil
	}
	if err != nil {
		return nil, err
	}
	defer input.Close()
	return parseReport(input, runIDs...)
}

func parseReport(input io.Reader, runIDs ...string) (*Report, error) {
	raw, err := io.ReadAll(io.LimitReader(input, (64<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 64<<20 {
		return nil, fmt.Errorf("metrics exceed 64 MiB; archive older telemetry")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	scanner.Split(func(data []byte, eof bool) (int, []byte, error) {
		if eof && len(data) > 0 && !bytes.Contains(data, []byte{'\n'}) {
			return 0, nil, fmt.Errorf("unterminated metrics record")
		}
		return bufio.ScanLines(data, eof)
	})
	calls := []Call{}
	identities := map[string]int{}
	for scanner.Scan() {
		var call Call
		if err := json.Unmarshal(scanner.Bytes(), &call); err != nil {
			return nil, err
		}
		if call.Cost != nil && *call.Cost < 0 || call.Duration != nil && *call.Duration < 0 || call.Usage != nil && (call.Usage.Input < 0 || call.Usage.Output < 0 || call.Usage.Total < 0) {
			return nil, fmt.Errorf("negative metrics observation")
		}
		if call.Role == "" {
			return nil, fmt.Errorf("metrics record requires a role")
		}
		if call.Kind == "reconciliation" {
			index, ok := identities[call.Reconciles]
			if !ok || calls[index].Receipt != call.Supersedes {
				return nil, fmt.Errorf("metrics correction has no matching original receipt")
			}
			original := &calls[index]
			if original.Usage != nil && call.Usage != nil && !reflect.DeepEqual(original.Usage, call.Usage) || original.Cost != nil && call.Cost != nil && *original.Cost != *call.Cost {
				return nil, fmt.Errorf("metrics correction contradicts known accounting")
			}
			if call.Usage != nil {
				original.Usage = call.Usage
			}
			if call.Cost != nil {
				original.Cost = call.Cost
			}
			original.Receipt = call.Receipt
			if call.Session != nil {
				original.Session = call.Session
			}
			continue
		}
		if call.Kind != "" {
			return nil, fmt.Errorf("unknown metrics record kind")
		}
		if call.Invocation != "" {
			if _, found := identities[call.Invocation]; found {
				return nil, fmt.Errorf("duplicate metrics invocation")
			}
			identities[call.Invocation] = len(calls)
		}
		calls = append(calls, call)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(runIDs) > 0 && runIDs[0] != "" {
		selected := []Call{}
		for _, call := range calls {
			if call.Run == runIDs[0] {
				selected = append(selected, call)
			}
		}
		calls = selected
	}
	groups := map[string]*Group{}
	for _, call := range calls {
		// JSON tuple keys avoid collisions caused by model/provider punctuation.
		raw, _ := json.Marshal([]string{call.Run, call.Role, call.Harness, call.Model, call.Provider})
		key := string(raw)
		group := groups[key]
		if group == nil {
			group = &Group{Run: call.Run, Role: call.Role, Harness: call.Harness, Model: call.Model, Provider: call.Provider}
			groups[key] = group
		}
		group.Calls++
		if call.Usage != nil && (call.Session == nil || call.Session.Last.UsageStatus != "complete") {
			group.PartialUsage++
		}
		if call.Error != "" {
			group.Failed++
		}
		if call.Usage == nil {
			group.MissingUsage++
		} else if call.Usage.Total != 0 {
			group.Tokens += call.Usage.Total
		} else {
			group.Tokens += call.Usage.Input + call.Usage.Output
		}
		if call.Cost == nil {
			group.MissingCost++
		} else {
			group.Cost += *call.Cost
		}
		if call.Duration == nil {
			group.MissingDuration++
		} else {
			group.Duration += *call.Duration
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	report := &Report{TotalCalls: len(calls), OmittedCalls: max(0, len(calls)-200), OmittedGroups: max(0, len(keys)-500), Calls: calls[max(0, len(calls)-200):], Groups: []Group{}}
	for _, key := range keys[max(0, len(keys)-500):] {
		report.Groups = append(report.Groups, *groups[key])
	}
	return report, nil
}
