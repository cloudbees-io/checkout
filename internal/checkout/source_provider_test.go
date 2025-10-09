package checkout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudbees-io/checkout/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	dir, err := os.MkdirTemp("", "checkout-action-test-")
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(dir) })

	t.Setenv("CLOUDBEES_EVENT_PATH", filepath.Join("testdata", "event.json"))
	t.Setenv("CLOUDBEES_WORKSPACE", dir)

	for _, tc := range []struct {
		name    string
		input   Config
		want    Config
		wantErr string
	}{
		{
			name: "short form GitHub repository",
			input: Config{
				Repository: "owner1/repo1",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "owner1/repo1",
				repositoryCloneURL: "https://github.com/owner1/repo1.git",
				GithubServerURL:    "https://github.com",
				Provider:           "github",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com",
			},
		},
		{
			name: "short form BitBucket repository",
			input: Config{
				Repository: "owner1/repo1",
				Provider:   "bitbucket",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "owner1/repo1",
				repositoryCloneURL: "https://bitbucket.org/owner1/repo1.git",
				BitbucketServerURL: "https://bitbucket.org",
				Provider:           "bitbucket",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com", // comes from the mounted scm event - very odd in this case
			},
		},
		{
			name: "short form GitLab repository",
			input: Config{
				Repository: "owner1/repo1",
				Provider:   "gitlab",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "owner1/repo1",
				repositoryCloneURL: "https://gitlab.com/owner1/repo1.git",
				GitlabServerURL:    "https://gitlab.com",
				Provider:           "gitlab",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com", // comes from the mounted scm event - very odd in this case
			},
		},
		{
			name: "fail when short form repository url provided for custom provider",
			input: Config{
				Repository: "owner1/repo1",
				Provider:   "custom",
				Submodules: "false",
				Token:      "fake-token",
			},
			wantErr: "short form repository URL provided but an absolute URL is required when using a custom SCM provider",
		},
		{
			name: "long form GitHub repository",
			input: Config{
				Repository: "https://github.com/owner1/repo1",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "https://github.com/owner1/repo1",
				repositoryCloneURL: "https://github.com/owner1/repo1.git",
				GithubServerURL:    "https://github.com",
				Provider:           "github",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com",
			},
		},
		{
			name: "long form BitBucket repository",
			input: Config{
				Repository: "https://bitbucket.org/owner1/repo1",
				Provider:   "bitbucket",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "https://bitbucket.org/owner1/repo1",
				repositoryCloneURL: "https://bitbucket.org/owner1/repo1.git",
				BitbucketServerURL: "https://bitbucket.org",
				Provider:           "bitbucket",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com", // comes from the mounted scm event - very odd in this case
			},
		},
		{
			name: "long form GitLab repository",
			input: Config{
				Repository: "https://gitlab.com/owner1/repo1",
				Provider:   "gitlab",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "https://gitlab.com/owner1/repo1",
				repositoryCloneURL: "https://gitlab.com/owner1/repo1.git",
				GitlabServerURL:    "https://gitlab.com",
				Provider:           "gitlab",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com", // comes from the mounted scm event - very odd in this case
			},
		},
		{
			name: "long form GitHub repository with .git extension",
			input: Config{
				Repository: "https://github.com/owner1/repo1.git",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "https://github.com/owner1/repo1.git",
				repositoryCloneURL: "https://github.com/owner1/repo1.git",
				GithubServerURL:    "https://github.com",
				Provider:           "github",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com",
			},
		},
		{
			name: "long form BitBucket repository with .git extension",
			input: Config{
				Repository: "https://bitbucket.org/owner1/repo1.git",
				Provider:   "bitbucket",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "https://bitbucket.org/owner1/repo1.git",
				repositoryCloneURL: "https://bitbucket.org/owner1/repo1.git",
				BitbucketServerURL: "https://bitbucket.org",
				Provider:           "bitbucket",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com", // comes from the mounted scm event - very odd in this case
			},
		},
		{
			name: "long form GitLab repository with .git extension",
			input: Config{
				Repository: "https://gitlab.com/owner1/repo1.git",
				Provider:   "gitlab",
				Submodules: "false",
				Token:      "fake-token",
			},
			want: Config{
				Repository:         "https://gitlab.com/owner1/repo1.git",
				repositoryCloneURL: "https://gitlab.com/owner1/repo1.git",
				GitlabServerURL:    "https://gitlab.com",
				Provider:           "gitlab",
				Submodules:         "false",
				Token:              "fake-token",
				Path:               ".",
				providerURL:        "https://github.com", // comes from the mounted scm event - very odd in this case
			},
		},
		{
			name: "SSH repository URL",
			input: Config{
				Repository: "git@github.com:mgoltzsche/podman-static.git",
				Provider:   "custom",
				Submodules: "false",
				Token:      "fake-token",
				SSHKey:     "fake-ssh-key",
			},
			want: Config{
				Repository:         "git@github.com:mgoltzsche/podman-static.git",
				repositoryCloneURL: "git@github.com:mgoltzsche/podman-static.git",
				Provider:           "custom",
				Submodules:         "false",
				Token:              "fake-token",
				SSHKey:             "fake-ssh-key",
				Path:               ".",
				providerURL:        "https://github.com", // comes from the mounted scm event - very odd in this case
			},
		},
		{
			name: "fail on SSH repository URL without ssh-key",
			input: Config{
				Repository: "git@github.com:mgoltzsche/podman-static.git",
				Provider:   "custom",
				Token:      "fake-token",
			},
			wantErr: "must also specify the ssh-key input when specifying an SSH URL as repository input",
		},
		{
			name: "fail on SSH repository URL without custom provider being specified",
			input: Config{
				Repository: "git@github.com:mgoltzsche/podman-static.git",
				Provider:   "github",
				SSHKey:     "fake-ssh-key",
			},
			wantErr: "provider input must be set to 'custom' when specifying an SSH URL within the repository input",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}

			require.NotNil(t, tc.input.eventContext)

			tc.input.eventContext = nil

			require.Equal(t, tc.want, tc.input, "Config struct after validation (initialization really)")
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
