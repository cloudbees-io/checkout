package checkout

import (
	"path/filepath"
	"testing"

	"github.com/cloudbees-io/checkout/internal/auth"
	"github.com/stretchr/testify/require"
)

func Test_findEventContext(t *testing.T) {
	t.Setenv("CLOUDBEES_EVENT_PATH", filepath.Join("testdata", "event.json"))

	cfg := Config{}
	err := cfg.findEventContext()
	require.NoError(t, err)
	require.NotEmpty(t, cfg.eventContext)

	// validate some fields
	provider, _ := getStringFromMap(cfg.eventContext, "provider")
	require.Equal(t, "github", provider)

	ref, _ := getStringFromMap(cfg.eventContext, "ref")
	require.Equal(t, "refs/heads/main", ref)
}

func Test_ensureScmPathForBitbucketDatacenterUrl(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "without scm in path",
			url:      "https://bitbucket.saas-preprod.beescloud",
			expected: "https://bitbucket.saas-preprod.beescloud/scm",
		},
		{
			name:     "with scm in path",
			url:      "https://bitbucket.saas-preprod.beescloud/scm",
			expected: "https://bitbucket.saas-preprod.beescloud/scm",
		},
		{
			name:     "with scm in path and trailing slash",
			url:      "https://bitbucket.saas-preprod.beescloud/scm/",
			expected: "https://bitbucket.saas-preprod.beescloud/scm/",
		},
		{
			name:     "with multiple paths - not ending with scm",
			url:      "https://bitbucket.saas-preprod.beescloud/abc/def",
			expected: "https://bitbucket.saas-preprod.beescloud/abc/def/scm",
		},
		{
			name:     "with multiple paths - ending with scm",
			url:      "https://bitbucket.saas-preprod.beescloud/abc/def/scm",
			expected: "https://bitbucket.saas-preprod.beescloud/abc/def/scm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Provider:           auth.BitbucketDatacenterProvider,
				BitbucketServerURL: tt.url,
			}
			err := cfg.ensureScmPathForBitbucketDatacenterUrl()
			require.NoError(t, err)
			require.Equal(t, tt.expected, cfg.BitbucketServerURL)
		})
	}
}
