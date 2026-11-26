package git

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_runMerge(t *testing.T) {
	dir, err := os.MkdirTemp("", "git")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.WriteFile(dir+"/merge.sh", []byte("#!/bin/sh\necho $1 $2 $3"), 0755)
	require.NoError(t, err)

	var g = &GitCLI{
		ctx: context.Background(),
	}
	mergeBin := dir + "/merge.sh"
	out, err := g.runMerge(mergeBin, "arg1", "arg2", "arg3")
	require.NoError(t, err)
	require.Equal(t, "arg1 arg2 arg3\n", out)
}

func Test_Fetch_errorKeepsGitDiagnosticAndExitError(t *testing.T) {
	dir := t.TempDir()
	fakeGit := dir + "/git"
	err := os.WriteFile(fakeGit, []byte("#!/bin/sh\necho 'fatal: remote error: upload-pack: not our ref abc' >&2\nexit 128\n"), 0755)
	require.NoError(t, err)

	g := &GitCLI{ctx: context.Background(), exe: fakeGit, cwd: dir, env: map[string]string{}}

	err = g.Fetch([]string{"abc"}, FetchOptions{FetchDepth: 1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not our ref")

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 128, exitErr.ExitCode())
}
