package verify

import (
	"testing"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptySuccessfulDiffIsKnownBoundaryEvidence(t *testing.T) {
	repo := initRepo(t)
	files, ok := GetChangedFiles(repo, "HEAD")
	require.True(t, ok)
	require.NotNil(t, files, "nil denotes an unavailable scan, not a known empty candidate diff")
	assert.Empty(t, files)
	boundary := CheckBoundaries(files, contract.Boundaries{}, nil)
	assert.True(t, boundary.Pass)
	assert.False(t, boundary.Skipped)
}
