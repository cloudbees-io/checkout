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
			name:  "number",
			input: "git@example.com:1234/org1/repo1",
			want:  true,
		},
		{
			name:  "subdomain",
			input: "git@git.my-repos_x.example.com:1234/some-org_x/some-repo_x.git",
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
