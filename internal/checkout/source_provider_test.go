package checkout

import (
	"errors"
	"path/filepath"
	"strings"
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
			name:  "preserve .git extension",
			input: "https://github.com/owner1/repo1.git",
			want:  "https://github.com/owner1/repo1.git",
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
		{
			name:    "fail on HTTP URL with SSH key",
			input:   "https://github.com/owner1/repo1",
			ssh:     true,
			wantErr: "ssh-key input is not supported when provided an HTTP URL within the repository input",
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

const (
	testMergeCommit = "c62c786c961f156a3730012fe02ec7e6436bd938"
	testHeadSHA     = "c20b0101ea18d53ff729bfcdb89536cc295ecda9"
	testBranchTip   = "88cb877000000000000000000000000000000000"
)

var errNotOurRef = errors.New("exit status 128: fatal: remote error: upload-pack: not our ref " + testMergeCommit)

func TestFetchWithPullRequestHeadFallback(t *testing.T) {
	// a Bitbucket like host: serves branches, rejects the merge commit
	host := func(calls *[][]string) func([]string) error {
		return func(rs []string) error {
			*calls = append(*calls, rs)
			if strings.Contains(rs[0], testMergeCommit) {
				return errNotOurRef
			}
			return nil
		}
	}
	revParse := func(ref string) (string, error) {
		if ref == "refs/remotes/origin/feature/x" {
			return testBranchTip, nil
		}
		return "", errors.New("unknown revision")
	}

	for _, tc := range []struct {
		name       string
		ref        string
		headSHA    string
		hasHeadSHA bool
		wantFirst  []string
		wantCommit string
	}{
		{name: "bare SHA rejected, PR head SHA fetched", ref: "refs/pull-requests/2/from", headSHA: testHeadSHA, hasHeadSHA: true, wantFirst: []string{testMergeCommit}, wantCommit: testHeadSHA},
		{name: "no PR head SHA, use the branch tip", ref: "refs/pull-requests/2/from", wantFirst: []string{testMergeCommit}, wantCommit: testBranchTip},
		{name: "head SHA not fetched (branch moved on), use the branch tip", ref: "", headSHA: testHeadSHA, wantFirst: []string{testMergeCommit}, wantCommit: testBranchTip},
		{name: "SHA with destination ref (normalized event ref) is retried", ref: "refs/heads/feature/x", headSHA: testHeadSHA, hasHeadSHA: true, wantFirst: []string{"+" + testMergeCommit + ":refs/remotes/origin/feature/x"}, wantCommit: testHeadSHA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			exists := func(sha string) bool { return tc.hasHeadSHA && sha == tc.headSHA }

			ref, commit, err := fetchWithPullRequestHeadFallback(host(&calls), exists, revParse, tc.ref, testMergeCommit, "feature/x", tc.headSHA)

			require.NoError(t, err)
			require.Empty(t, ref, "the PR head is checked out detached")
			require.Equal(t, tc.wantCommit, commit)
			require.Equal(t, [][]string{tc.wantFirst, {"+refs/heads/feature/x:refs/remotes/origin/feature/x"}}, calls)
		})
	}
}

func TestFetchWithPullRequestHeadFallback_ReturnsTheOriginalErrorWhenThereIsNothingToFallBackTo(t *testing.T) {
	boom := errors.New("exit status 128: fatal: unable to access: network down")
	never := func(string) bool { return false }
	noRev := func(string) (string, error) { return "", errors.New("unknown revision") }

	for _, tc := range []struct {
		name      string
		err       error
		ref       string
		commit    string
		prBranch  string
		fetchFail func(rs []string) bool // which fetch calls fail with err; the rest succeed
		wantCalls int
	}{
		{name: "other error", err: boom, ref: "", commit: testMergeCommit, prBranch: "feature/x", fetchFail: func([]string) bool { return true }, wantCalls: 1},
		{name: "event without PR head", err: errNotOurRef, ref: "", commit: testMergeCommit, prBranch: "", fetchFail: func([]string) bool { return true }, wantCalls: 1},
		{name: "ref only fetch (no commit)", err: errNotOurRef, ref: "refs/heads/main", commit: "", prBranch: "feature/x", fetchFail: func([]string) bool { return true }, wantCalls: 1},
		{name: "PR branch can't be fetched either", err: errNotOurRef, ref: "", commit: testMergeCommit, prBranch: "gone", fetchFail: func([]string) bool { return true }, wantCalls: 2},
		{name: "PR branch fetched but no head found", err: errNotOurRef, ref: "", commit: testMergeCommit, prBranch: "feature/x", fetchFail: func(rs []string) bool { return strings.Contains(rs[0], testMergeCommit) }, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			fetch := func(rs []string) error {
				calls++
				if tc.fetchFail(rs) {
					return tc.err
				}
				return nil
			}

			ref, commit, err := fetchWithPullRequestHeadFallback(fetch, never, noRev, tc.ref, tc.commit, tc.prBranch, "")

			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.ref, ref)
			require.Equal(t, tc.commit, commit)
			require.Equal(t, tc.wantCalls, calls)
		})
	}
}

func TestPullRequestHead(t *testing.T) {
	const repo = "https://bitbucket.org/acme/app"
	cfg := func(repository string) *Config {
		return &Config{
			Repository:         repository,
			repositoryCloneURL: repository + ".git",
			eventContext: map[string]interface{}{
				"repositoryUrl": repo,
				"sha":           testMergeCommit,
				"pullRequest": map[string]interface{}{
					"head": map[string]interface{}{"branch": "feature/x", "sha": testHeadSHA},
				},
			},
		}
	}

	branch, sha := cfg(repo).pullRequestHead(testMergeCommit)
	require.Equal(t, "feature/x", branch, "the event's own commit")
	require.Equal(t, testHeadSHA, sha)

	branch, _ = cfg(repo).pullRequestHead(testHeadSHA)
	require.Equal(t, "feature/x", branch, "the PR head")

	branch, _ = cfg(repo).pullRequestHead("1111111111111111111111111111111111111111")
	require.Empty(t, branch, "an explicit SHA says nothing about the PR branch")

	branch, _ = cfg("https://bitbucket.org/acme/other").pullRequestHead(testMergeCommit)
	require.Empty(t, branch, "the event only describes the workflow repository")
}

func TestTestRef_UnsupportedRefIsNotAnError(t *testing.T) {
	// Bitbucket PR events carry refs such as refs/pull-requests/2/from, which the all-history fetch does not retrieve.
	// They can't be verified, so report "not present" to fetch them instead of failing the checkout.
	present, err := testRef(nil, "refs/pull-requests/2/from", testHeadSHA)
	require.NoError(t, err)
	require.False(t, present)
}
