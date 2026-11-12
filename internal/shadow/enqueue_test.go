package shadow

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnqueueNeedsNoKeyAndDefersAllHTTP(t *testing.T) {
	dataset, directory := sample(), privateDir(t)
	o := opts("https://example.org/v1/systemone")
	o.APIKey = ""
	calls := 0
	o.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, os.ErrPermission })
	require.NoError(t, Enqueue(dataset, directory, o))
	require.NoError(t, Enqueue(dataset, directory, o))
	require.Zero(t, calls)
	require.FileExists(t, filepath.Join(directory, "cases.json"))
	report, err := ReadReport(dataset, directory)
	require.NoError(t, err)
	require.Equal(t, 1, report.Summary.NotRun)
	require.Zero(t, report.NewCalls)
	pending, err := filepath.Glob(filepath.Join(directory, "*.pending"))
	require.NoError(t, err)
	require.Empty(t, pending)
	o.APIKey = "explicit-experiment-key"
	report, err = Run(context.Background(), dataset, directory, o)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, report.NewCalls)
}

func TestEnqueueRejectsUnreviewableInputsAndStorageFailures(t *testing.T) {
	for _, scenario := range []string{"dataset", "target", "secret", "large", "directory", "manifest"} {
		t.Run(scenario, func(t *testing.T) {
			dataset, directory := sample(), privateDir(t)
			o := opts("https://example.org/v1/systemone")
			switch scenario {
			case "dataset":
				dataset.Version = 100
			case "target":
				o.Endpoint = "invalid"
			case "secret":
				o.APIKey = "unlikely-experiment-credential"
				dataset.Cases[0].Task = o.APIKey
			case "large":
				dataset.Cases[0].Evidence = make([]string, 32)
				for i := range dataset.Cases[0].Evidence {
					dataset.Cases[0].Evidence[i] = strings.Repeat("字", 1500)
				}
			case "directory":
				directory = filepath.Join(directory, "file")
				require.NoError(t, os.WriteFile(directory, nil, 0600))
			case "manifest":
				require.NoError(t, os.WriteFile(filepath.Join(directory, "target.json"), []byte("{"), 0600))
			}
			require.Error(t, Enqueue(dataset, directory, o))
		})
	}
}
