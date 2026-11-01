#!/usr/bin/env bash
# Prepare a new, self-contained Flywheel demo. No model call or Git commit.
set -eu

if [ "$#" -ne 1 ]; then
  printf '%s\n' 'usage: bash create-first-task.sh /absolute/path/to/new-demo-directory' >&2
  exit 1
fi
demo_dir=$1
case "$demo_dir" in
  /*) ;;
  *) printf '%s\n' 'demo directory must be an absolute path' >&2; exit 1 ;;
esac
if [ -e "$demo_dir" ] || [ -L "$demo_dir" ]; then
  printf '%s\n' 'demo directory already exists; choose a new path' >&2
  exit 1
fi
flywheel_bin=${FLYWHEEL_BIN:-flywheel}
config_json=$("$flywheel_bin" template flywheel.config.example.json)

mkdir -p "$demo_dir/acceptance" "$demo_dir/docs/add"
cat > "$demo_dir/go.mod" <<'EOF'
module example.local/flywheel-demo

go 1.27.0
EOF
cat > "$demo_dir/sum.go" <<'EOF'
package demo

// Add returns the sum of two integers. The task starts with a deliberate stub.
func Add(a, b int) int { return 0 }
EOF
cat > "$demo_dir/acceptance/sum_test.go" <<'EOF'
package acceptance

import (
	"testing"

	demo "example.local/flywheel-demo"
)

func TestAddAcceptance(t *testing.T) {
	for _, tc := range []struct{ a, b, want int }{
		{2, 3, 5}, {-2, -3, -5}, {-4, 4, 0}, {0, 7, 7},
	} {
		if got := demo.Add(tc.a, tc.b); got != tc.want {
			t.Errorf("Add(%d, %d) = %d; want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
EOF
cat > "$demo_dir/.gitignore" <<'EOF'
/coverage.out
EOF
cat > "$demo_dir/docs/add/.gitignore" <<'EOF'
orchestrator.log
orchestrator.pid
metrics.jsonl
.state.lock
.run.lock
.stop-request
.control/
EOF
cat > "$demo_dir/docs/add/spec.md" <<'EOF'
---
spec-version: 1
title: First Flywheel task
goal: Implement Add and meaningful unit tests in a small Go repository.
non-goals: [CLI, networking, dependencies, overflow policy beyond Go integer addition]
milestones:
  - id: M-01
    title: Addition and tests
    gate: Unit tests, coverage, and frozen acceptance pass.
requirements:
  - id: R-01
    title: Add returns a + b for positive, negative, and zero inputs
    milestone: M-01
    verification: go test ./acceptance
  - id: R-02
    title: Ordinary unit tests assert results and cover every function statement
    milestone: M-01
    verification: go test -coverprofile=coverage.out . and the coverage gate
---

# First Flywheel task

Only implement `sum.go` and add `sum_test.go`. The acceptance package is frozen.
Keep all dependency, contract, and verification settings unchanged.

## Acceptance dry-run evidence

Before approving this contract, run each L1 command from the packet and record
its exact command, exit status, and a short output summary here. Initial failure
is expected because Add deliberately returns zero and ordinary tests are absent.
Do not replace these observations with claimed model-run or Mac-run results.
EOF
cat > "$demo_dir/docs/add/task-packet-M01-T01.md" <<'EOF'
---
packet-id: M01-T01
contract-version: 1
goal: Implement Add and ordinary unit tests without changing frozen acceptance.
requirements: [R-01, R-02]
route: default
needs-live-credentials: false
context-files: [sum.go, acceptance/sum_test.go, docs/add/spec.md]
cache-scope: [docs/add/spec.md]
boundaries:
  must-not-touch: [acceptance/, go.mod, .gitignore]
  max-files: 2
acceptance:
  L1:
    - {cmd: "go test ./...", expect: "exit 0"}
    - cmd: >-
        go test -coverprofile=coverage.out . &&
        go tool cover -func=coverage.out |
        awk '$NF != "100.0%" && $NF != "100%" { bad=1 } END { exit bad }'
      expect: exit 0
  L2:
    - {cmd: "go test ./acceptance", expect: "exit 0"}
review-rubric: |
  Pass only when Add implements integer addition, tests assert actual results,
  frozen acceptance is unchanged, and no unrelated functionality was added.
  A green coverage number alone is insufficient.
escalation: {max-retries: 3, on-fail: inbox}
done-report-format:
  requirements-covered: []
  changed-files: []
  commands-run: []
  failures: []
---

Implement sum.go and sum_test.go only. Do not alter the specification,
configuration, frozen acceptance, or the coverage threshold.
Write the required done-report to the exact path provided by the engine prompt.
EOF
printf '%s\n' "$config_json" > "$demo_dir/flywheel.config.json"
printf 'Prepared %s\n' "$demo_dir"
printf '%s\n' 'Next: inspect the files, initialize Git, dry-run the acceptance commands, review the contract, then start Flywheel.'
