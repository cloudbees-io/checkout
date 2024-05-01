package git

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_lookupGitCliInRunnerTemp(t *testing.T) {
	dir, err := os.MkdirTemp("", "git")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	os.Setenv("RUNNER_TEMP", dir)
	err = os.WriteFile(dir+"/git", []byte("git"), 0755)
	require.NoError(t, err)

	require.Equal(t, dir+"/git", lookupGitCliInRunnerTemp())
}

func Test_runMerge(t *testing.T) {
	dir, err := os.MkdirTemp("", "git")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.WriteFile(dir+"/merge.sh", []byte("#!/bin/sh\necho $1 $2 $3"), 0755)
	require.NoError(t, err)

	var g = &GitCLI{
		ctx:         context.Background(),
		mergeBinary: dir + "/merge.sh",
	}
	out, err := g.runMerge("arg1", "arg2", "arg3")
	require.NoError(t, err)
	require.Equal(t, "arg1 arg2 arg3\n", out)
}
