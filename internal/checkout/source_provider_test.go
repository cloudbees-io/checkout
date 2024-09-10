package checkout

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_findEventContext(t *testing.T) {
	t.Setenv("CLOUDBEES_EVENT_PATH", filepath.Join("testdata", "event.json"))

	eventContext, err := findEventContext()
	require.NoError(t, err)
	require.NotEmpty(t, eventContext)

	// validate some fields
	provider, _ := getStringFromMap(eventContext, "provider")
	require.Equal(t, "github", provider)

	ref, _ := getStringFromMap(eventContext, "ref")
	require.Equal(t, "refs/heads/main", ref)
}
