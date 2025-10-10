package checkout

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSSHURL(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "GitHub URL simple",
			input: "git@github.com:org1/repo1",
			want:  true,
		},
		{
			name:  "GitHub URL with minus and underscore",
			input: "git@github.com:some-org_x/some-repo_x",
			want:  true,
		},
		{
			name:  "BitBucket URL with .git extension",
			input: "git@bitbucket.org:some-org/some-repo.git",
			want:  true,
		},
		{
			name:  "with upper case repo name",
			input: "git@example.com:org1/Repo1",
			want:  true,
		},
		{
			name:  "with upper case user",
			input: "GIT@example.com:org1/repo1",
			want:  true,
		},
		{
			name:  "with port",
			input: "git@example.com:1234/org1/repo1",
			want:  true,
		},
		{
			name:  "with protocol",
			input: "ssh://git@example.com:1234/org1/repo1",
			want:  true,
		},
		{
			name:  "with protocol but without port",
			input: "ssh://git@example.com/org1/repo1",
			want:  true,
		},
		{
			name:  "with subdomain",
			input: "git@git.my-repos_x.example.com:org1/repo1",
			want:  true,
		},
		{
			name:  "with home dir",
			input: "git@git.my-repos_x.example.com:~/repos/org1/repo1",
			want:  true,
		},
		{
			name:  "gerrit url (without user)",
			input: "ssh://gerrithost:29418/RecipeBook.git",
			want:  true,
		},
		{
			name:  "all syntax features",
			input: "ssh://git@git.my-repos_x.example.com:1234/some-org_x/some-repo_x.git",
			want:  true,
		},
		{
			name:  "HTTP URL",
			input: "https://github.com/org1/repo1",
			want:  false,
		},
		{
			name:  "short form syntax",
			input: "org1/repo1",
			want:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := isSSHURL(tc.input)
			require.Equal(t, tc.want, actual)
		})
	}
}

func TestNormalizeSSHURL(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "GitHub SSH URL",
			input: "git@github.com:org1/repo1",
			want:  "ssh://git@github.com/org1/repo1",
		},
		{
			name:  "Bitbucket SSH URL",
			input: "git@bitbucket.org:org1/repo1.git",
			want:  "ssh://git@bitbucket.org/org1/repo1.git",
		},
		{
			name:  "Normalized SSH URL",
			input: "ssh://example.com/org1/repo1.git",
			want:  "ssh://example.com/org1/repo1.git",
		},
		{
			name:  "HTTP URL",
			input: "https://github.com/org1/repo1.git",
			want:  "https://github.com/org1/repo1.git",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := normalizeSSHURL(tc.input)
			require.Equal(t, tc.want, actual)
		})
	}
}
