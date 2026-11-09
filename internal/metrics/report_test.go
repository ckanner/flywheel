package metrics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type failedReportReader struct{}

func (failedReportReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestReportReconcilesBeforeGroupingAndKeepsMissingObservations(t *testing.T) {
	input := `{"role":"execute","run-id":"a","invocation-id":"call1","receipt-sha256":"one","model":"Flash","usage":null,"cost-usd":null,"duration-ms":10}
{"role":"execute","run-id":"b","invocation-id":"call2","model":"Flash","usage":{"input":5,"output":2},"error-code":"TIMEOUT"}
{"role":"execute","record-kind":"reconciliation","invocation-id":"correction","reconciles-invocation":"call1","supersedes-receipt":"one","receipt-sha256":"two","usage":{"totalTokens":10},"cost-usd":0.125}
{"role":"review","model":"GPT","usage":{"totalTokens":5},"cost-usd":0}
`
	report, err := parseReport(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, report.Calls, 3)
	require.Len(t, report.Groups, 3)
	require.Equal(t, "a", report.Calls[0].Run)
	require.Equal(t, int64(10), report.Calls[0].Usage.Total)
	for _, group := range report.Groups {
		switch group.Run {
		case "a":
			require.Equal(t, 0.125, group.Cost)
			require.Zero(t, group.MissingCost)
			require.Equal(t, int64(10), group.Duration)
		case "b":
			require.Equal(t, 1, group.Failed)
			require.Equal(t, int64(7), group.Tokens)
			require.Equal(t, 1, group.MissingCost)
		default:
			require.Zero(t, group.Cost)
			require.Equal(t, 1, group.MissingDuration)
		}
	}
	missing, err := parseReport(strings.NewReader("{\"role\":\"plan\"}\n"))
	require.NoError(t, err)
	require.Equal(t, 1, missing.Groups[0].MissingUsage)
	// Multiple calls to the same provider/model group retain every billed turn.
	two, err := parseReport(strings.NewReader("{\"role\":\"plan\"}\n{\"role\":\"plan\"}\n"))
	require.NoError(t, err)
	require.Equal(t, 2, two.Groups[0].Calls)
}

func TestReportRejectsTornUnknownAndContradictoryTelemetry(t *testing.T) {
	original := "{\"role\":\"execute\",\"invocation-id\":\"one\",\"receipt-sha256\":\"r\",\"usage\":{\"totalTokens\":3},\"cost-usd\":1}\n"
	for _, input := range []string{"{", "null\n", "{\"role\":\"plan\"}", "{\"role\":\"plan\",\"record-kind\":\"future\"}\n", original + original,
		"{\"role\":\"plan\",\"record-kind\":\"reconciliation\",\"reconciles-invocation\":\"missing\"}\n",
		original + "{\"role\":\"plan\",\"record-kind\":\"reconciliation\",\"reconciles-invocation\":\"one\",\"supersedes-receipt\":\"r\",\"cost-usd\":2}\n",
		strings.Repeat("x", 2<<20) + "\n", "bad\n"} {
		_, err := parseReport(strings.NewReader(input))
		require.Error(t, err)
	}
	_, err := parseReport(failedReportReader{})
	require.ErrorContains(t, err, "read failure")
	_, err = parseReport(strings.NewReader(strings.Repeat("{\"role\":\"plan\",\"padding\":\""+strings.Repeat("x", 1<<20)+"\"}\n", 65)))
	require.ErrorContains(t, err, "64 MiB")
	root := t.TempDir()
	report, err := ReadReport(filepath.Join(root, "missing"))
	require.NoError(t, err)
	require.Empty(t, report.Calls)
	_, err = ReadReport(root)
	require.Error(t, err)
	file := filepath.Join(root, "log")
	require.NoError(t, os.WriteFile(file, []byte(original), 0600))
	report, err = ReadReport(file)
	require.NoError(t, err)
	require.Len(t, report.Calls, 1)
	require.NoError(t, os.WriteFile(file, []byte(original+"{\"role\":\"execute\",\"record-kind\":\"reconciliation\",\"reconciles-invocation\":\"one\",\"supersedes-receipt\":\"r\"}\n"), 0600))
	_, err = ReadReport(file)
	require.NoError(t, err)
	_, err = ReadReport(filepath.Join(file, "child"))
	require.Error(t, err)
}

func TestMetricsBoundedViewsStillAggregateEverySelectedInvocation(t *testing.T) {
	var input strings.Builder
	for i := 0; i < 600; i++ {
		fmt.Fprintf(&input, "{\"role\":\"execute\",\"run-id\":\"%d\",\"usage\":{\"totalTokens\":2},\"session\":{\"last\":{\"usage-status\":\"partial\"}}}\n", i)
	}
	report, err := parseReport(strings.NewReader(input.String()))
	require.NoError(t, err)
	require.Equal(t, 600, report.TotalCalls)
	require.Len(t, report.Calls, 200)
	require.Equal(t, 400, report.OmittedCalls)
	require.Len(t, report.Groups, 500)
	require.Equal(t, 100, report.OmittedGroups)
	report, err = parseReport(strings.NewReader(input.String()), "42")
	require.NoError(t, err)
	require.Equal(t, 1, report.TotalCalls)
	require.Equal(t, int64(2), report.Groups[0].Tokens)
	require.Equal(t, 1, report.Groups[0].PartialUsage)
	for _, value := range []string{`{"role":"execute","usage":{"input":-1}}`, `{"role":"execute","cost-usd":-1}`, `{"role":"execute","duration-ms":-1}`} {
		_, err := parseReport(strings.NewReader(value + "\n"))
		require.Error(t, err)
	}
	initial := `{"role":"execute","invocation-id":"one","receipt-sha256":"old","run-id":"original","usage":null}` + "\n"
	correction := `{"role":"execute","record-kind":"reconciliation","reconciles-invocation":"one","supersedes-receipt":"old","receipt-sha256":"new","usage":{"input":5},"session":{"last":{"usage-status":"complete"}}}` + "\n"
	report, err = parseReport(strings.NewReader(initial+correction), "original")
	require.NoError(t, err)
	require.Equal(t, 1, report.TotalCalls)
	require.Zero(t, report.Groups[0].PartialUsage)
}
