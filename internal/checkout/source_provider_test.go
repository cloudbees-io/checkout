package checkout

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRepositoryURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		ssh     bool
		want    string
		wantErr string
	}{
		{
			name:    "fail when short form repository URL (v1) provided",
			input:   "owner1/repo1",
			wantErr: `invalid repository URL "owner1/repo1" provided, expects full clone URL`,
		},
		{
			name:  "identity",
			input: "https://github.com/owner1/repo1",
			want:  "https://github.com/owner1/repo1",
		},
		{
			name: "strip .git extension from HTTP URL",
			// This is to be able to compare the URL with the one within the SCM event which may or may not have a .git extension.
			input: "https://github.com/owner1/repo1.git",
			want:  "https://github.com/owner1/repo1",
		},
		{
			name:  "support custom port",
			input: "https://example.com:8443/owner1/repo1",
			want:  "https://example.com:8443/owner1/repo1",
		},
		{
			name:  "SSH URL",
			input: "git@github.com:mgoltzsche/podman-static.git",
			ssh:   true,
			want:  "ssh://git@github.com/mgoltzsche/podman-static.git",
		},
		{
			name:    "fail on SSH URL without ssh-key",
			input:   "git@github.com:mgoltzsche/podman-static.git",
			wantErr: "must also specify the ssh-key input when specifying an SSH URL as repository input",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := normalizeRepositoryURL(tc.input, tc.ssh)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}

			require.Equal(t, tc.want, actual)
		})
	}
}

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
