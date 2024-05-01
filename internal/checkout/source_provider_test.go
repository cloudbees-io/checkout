package checkout

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_pullRequestInfo(t *testing.T) {
	eventContext := safeLoadEventContext("testdata/event.json")
	prInfo, err := pullRequestInfo(eventContext)
	require.NoError(t, err)

	require.Equal(t, &PullRequest{
		HeadSha:       "fake-head-commit-sha",
		BaseSha:       "fake-base-commit-sha",
		CommitterDate: "2023-01-01T12:00:00Z",
	}, prInfo)
}
