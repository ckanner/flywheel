package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"syscall"
	"time"

	"github.com/ckanner/flywheel/internal/decision"
	"github.com/spf13/cobra"
)

const decisionDocumentLimit = 128 << 10

var decisionOpenFile = func(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
var decisionReadAll = io.ReadAll
var decisionKeyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,199}$`)

type decisionCapture struct {
	Snapshot   decision.Snapshot `json:"snapshot"`
	Policy     decision.Policy   `json:"policy"`
	TargetHash string            `json:"target-hash"`
}

type decisionCLIReport struct {
	Mode             decision.Mode    `json:"mode"`
	Policy           decision.Policy  `json:"policy"`
	Baseline         string           `json:"baseline-id"`
	Adopted          bool             `json:"adopted"`
	SourceTargetHash string           `json:"source-target-hash,omitempty"`
	Receipt          decision.Receipt `json:"receipt"`
}

func decisionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "decision", Short: "离线重放或显式评估有界决策；不采用动作、不改变任务"}
	cmd.AddCommand(decisionEvaluationCmd(false), decisionEvaluationCmd(true))
	return cmd
}

func decisionEvaluationCmd(live bool) *cobra.Command {
	var input, policyFile, response, directory, keyEnv string
	var maxCalls int
	options := decision.Options{}
	name, description := "replay", "离线读取原生响应或 fixture；不发送 HTTP 请求"
	if live {
		name, description = "evaluate", "显式付费 advisory 评估；独立私有账本，不自动重试未知调用"
	}
	cmd := &cobra.Command{Use: name, Short: description, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if input == "" || options.Endpoint == "" || options.Model == "" || options.ExpectedModel == "" {
				return fmt.Errorf("decision requires --input, --endpoint, --model and --expected-model")
			}
			if live && (directory == "" || !decisionKeyName.MatchString(keyEnv) || maxCalls < 1 || maxCalls > 100) {
				return fmt.Errorf("evaluate requires --directory, --key-env and explicit --max-calls (1..100)")
			}
			capture, err := loadDecisionInput(input, policyFile)
			if err != nil {
				return err
			}
			capture.Policy.Mode = decision.Offline
			var receipt decision.Receipt
			if live {
				capture.Policy.Mode, capture.Policy.MaxCalls = decision.Advisory, maxCalls
				options.APIKey = os.Getenv(keyEnv)
				if options.APIKey == "" {
					return fmt.Errorf("decision key environment variable is empty: %s", keyEnv)
				}
				ctx, cancel := diagnosticContext(cmd)
				defer cancel()
				receipt, err = decision.Evaluate(ctx, directory, capture.Snapshot, capture.Policy, options)
			} else {
				var native []byte
				native, err = readDecisionDocument(response)
				if err == nil {
					receipt, err = decision.Replay(capture.Snapshot, capture.Policy, options, native)
				}
			}
			if err != nil {
				return err
			}
			printJSON(cmd, decisionCLIReport{Mode: capture.Policy.Mode, Policy: capture.Policy, Baseline: capture.Snapshot.BaselineID, SourceTargetHash: capture.TargetHash, Receipt: receipt})
			if receipt.Reason == "cancelled-before-dispatch" {
				return &exitError{code: 143}
			}
			if receipt.Status == "error" || receipt.Status == "unknown" {
				return &exitError{code: 3}
			}
			return nil
		}}
	cmd.Flags().StringVar(&input, "input", "", "已审阅的 shadow .snapshot.json，或裸 Snapshot JSON")
	cmd.Flags().StringVar(&policyFile, "policy", "", "可选独立策略 JSON；裸 Snapshot 必需，mode 由子命令强制限定")
	cmd.Flags().StringVar(&options.Endpoint, "endpoint", "", "明确的 HTTPS /v1/systemone 地址；replay 仅记录身份")
	cmd.Flags().StringVar(&options.Model, "model", "", "固定版本 jev-x.y.z；不允许可变别名")
	cmd.Flags().StringVar(&options.ExpectedModel, "expected-model", "", "必须与请求模型及响应版本完全相同")
	cmd.Flags().DurationVar(&options.Timeout, "timeout", 5*time.Second, "单次调用时限，不自动重试，最大 30s")
	if live {
		cmd.Flags().StringVar(&directory, "directory", "", "独立私有评估账本；保留原未知回执与调用预算")
		cmd.Flags().StringVar(&keyEnv, "key-env", "", "明确的密钥环境变量名；不要传密钥值")
		cmd.Flags().IntVar(&maxCalls, "max-calls", 0, "显式累计调用上限 1..100，包含未知调用")
		cmd.Flags().BoolVar(&options.HardBudget, "hard-budget", false, "要求可证明请求上限；原生 API 无此能力时跳过调用")
	} else {
		cmd.Flags().StringVar(&response, "response", "", "原生 System One 响应或离线 fixture JSON")
	}
	return cmd
}

func readDecisionDocument(path string) ([]byte, error) {
	f, err := decisionOpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > decisionDocumentLimit {
		return nil, fmt.Errorf("decision document must be a regular file no larger than 128 KiB")
	}
	raw, err := decisionReadAll(io.LimitReader(f, decisionDocumentLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > decisionDocumentLimit {
		return nil, fmt.Errorf("decision document grew beyond 128 KiB")
	}
	return raw, nil
}

func decodeDecisionDocument(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil || !json.Valid(raw) {
		return fmt.Errorf("invalid decision JSON document")
	}
	return nil
}

func loadDecisionInput(input, policyFile string) (decisionCapture, error) {
	var capture decisionCapture
	raw, err := readDecisionDocument(input)
	if err != nil {
		return capture, err
	}
	var shape map[string]json.RawMessage
	if err := decodeDecisionDocument(raw, &shape); err != nil {
		return capture, err
	}
	if _, wrapped := shape["snapshot"]; wrapped {
		err = decodeDecisionDocument(raw, &capture)
	} else {
		err = decodeDecisionDocument(raw, &capture.Snapshot)
		if err == nil && policyFile == "" {
			err = fmt.Errorf("a bare decision snapshot requires --policy")
		}
	}
	if err != nil {
		return capture, err
	}
	if policyFile != "" {
		raw, err = readDecisionDocument(policyFile)
		if err != nil {
			return capture, err
		}
		capture.Policy = decision.Policy{}
		err = decodeDecisionDocument(raw, &capture.Policy)
	}
	return capture, err
}
