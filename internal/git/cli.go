package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudbees-io/checkout/internal/core"
)

// GitCLI maintains a context for interacting with the Git command line executable.
type GitCLI struct {
	ctx         context.Context
	exe         string
	env         map[string]string
	cwd         string
	quiet       bool
	log         bool
	mergeBinary string
}

// NewGitCLI creates a new GitCLI instance
func NewGitCLI(ctx context.Context) (*GitCLI, error) {
	exe, err := exec.LookPath("git")
	if err != nil && !errors.Is(err, exec.ErrDot) {
		return nil, err
	} else if errors.Is(err, exec.ErrDot) {
		if exe, err = filepath.Abs(exe); err != nil {
			return nil, err
		}
	}

	mergeBinary, _ := exec.LookPath("merge")
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	env := os.Environ()
	return &GitCLI{ctx: ctx, exe: exe, env: envEntriesToMap(env), cwd: cwd, quiet: false, log: true, mergeBinary: mergeBinary}, nil
}

// SetEnv sets the environment variable for the GitCLI
func (g *GitCLI) SetEnv(key string, val string) {
	g.env[key] = val
}

// SetCwd sets the current working directory used by the GitCLI
func (g *GitCLI) SetCwd(cwd string) {
	g.cwd = cwd
}

// Cwd returns the current working directory used by the GitCLI
func (g *GitCLI) Cwd() string {
	return g.cwd
}

func (g *GitCLI) Executable() string {
	return g.exe
}

// GlobalConfigPath returns the path of the global configuration file
func (g *GitCLI) GlobalConfigPath() (string, error) {
	if home, haveHome := g.env["HOME"]; haveHome {
		// if $HOME/.gitconfig exists then we use that
		gitconfigPath := filepath.Join(home, ".gitconfig")
		if stat, err := os.Stat(gitconfigPath); err == nil && !stat.IsDir() {
			return gitconfigPath, nil
		}
	}

	if xdgConfigHome, haveXdgConfigHome := g.env["XDG_CONFIG_HOME"]; haveXdgConfigHome {
		// if $XDG_CONFIG_HOME/git/config exists then we use that
		xdgConfigPath := filepath.Join(xdgConfigHome, "git", "config")
		if stat, err := os.Stat(xdgConfigPath); err == nil && !stat.IsDir() {
			return xdgConfigPath, nil
		}
	}

	return "", fmt.Errorf("unable to locate config file '%s'", filepath.Join("$HOME", ".gitconfig"))
}

func (g *GitCLI) runMerge(args ...string) (string, error) { // this function is implemented similar to the 'run' function below
	c := exec.CommandContext(g.ctx, g.mergeBinary, args...)
	c.Dir = g.cwd
	c.Env = envMapToEntries(g.env)

	if g.log {
		fmt.Println(c.String())
	}

	if !g.quiet {
		c.Stderr = os.Stderr
	}

	var stdout = bytes.Buffer{}
	c.Stdout = &stdout
	err := c.Run()
	if e := (&exec.ExitError{}); err != nil && errors.As(err, &e) {
		core.Debug("merge command exited with status %d", e.ExitCode())
		return stdout.String(), err
	} else if err != nil {
		core.Debug("merge command exited with status %d", 126)
	} else {
		core.Debug("0")
	}

	return stdout.String(), err
}

func (g *GitCLI) run(args ...string) error {
	c := exec.CommandContext(g.ctx, g.exe, args...)
	c.Dir = g.cwd
	c.Env = envMapToEntries(g.env)

	if g.log {
		fmt.Println(c.String())
	}

	if !g.quiet {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}

	err := c.Run()
	if e := (&exec.ExitError{}); err != nil && errors.As(err, &e) {
		core.Debug("%d", e.ExitCode())
		return err
	} else if err != nil {
		core.Debug("126")
	} else {
		core.Debug("0")
	}

	return err
}

func (g *GitCLI) runOutput(args ...string) (string, error) {
	c := exec.CommandContext(g.ctx, g.exe, args...)
	c.Dir = g.cwd
	c.Env = envMapToEntries(g.env)
	if g.log {
		fmt.Println(c.String())
	}
	var stdoutBuf strings.Builder
	if !g.quiet {
		c.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
		c.Stderr = os.Stderr
	} else {
		c.Stdout = &stdoutBuf
	}
	err := c.Run()

	return stdoutBuf.String(), err
}

func (g *GitCLI) silentRunOutput(args ...string) (string, error) {
	c := exec.CommandContext(g.ctx, g.exe, args...)
	c.Dir = g.cwd
	c.Env = envMapToEntries(g.env)
	var stdoutBuf strings.Builder
	c.Stdout = &stdoutBuf
	err := c.Run()

	return stdoutBuf.String(), err
}

func (g *GitCLI) BranchList(remote bool) ([]string, error) {
	var target string
	if remote {
		target = "--remotes=origin"
	} else {
		target = "--branches"
	}

	output, err := g.silentRunOutput("rev-parse", "--symbolic-full-name", target)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, branch := range strings.Split(output, "\n") {
		branch = strings.TrimSpace(branch)

		if branch == "" {
			continue
		}

		if strings.HasPrefix(branch, "refs/heads/") {
			branch = strings.TrimPrefix(branch, "refs/heads/")
		} else if strings.HasPrefix(branch, "refs/remotes/") {
			branch = strings.TrimPrefix(branch, "refs/remotes/")
		}

		result = append(result, branch)
	}

	return result, nil
}

func (g *GitCLI) BranchDelete(remote bool, branch string) error {
	args := []string{"branch", "--delete", "--force"}

	if remote {
		args = append(args, "--remote")
	}

	args = append(args, branch)

	return g.run(args...)
}

func (g *GitCLI) BranchExists(remote bool, pattern string) (bool, error) {
	args := []string{"branch", "--list"}

	if remote {
		args = append(args, "--remote")
	}

	args = append(args, pattern)

	output, err := g.runOutput(args...)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(output) != "", nil
}

func (g *GitCLI) TagExists(pattern string) (bool, error) {
	output, err := g.runOutput("tag", "--list", pattern)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(output) != "", nil
}

func (g *GitCLI) BranchGetDefault(repositoryUrl string) (string, error) {
	output, err := g.runOutput("ls-remote", "--quiet", "--exit-code", "--symref", repositoryUrl, "HEAD")
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ref:") && strings.HasSuffix(line, "HEAD") {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "ref:"), "HEAD")), nil
		}
	}
	return "", fmt.Errorf("unexpected output when retrieving default branch")
}

func configScope(global bool) string {
	if global {
		return "--global"
	}
	return "--local"
}
func (g *GitCLI) SetConfigStr(global bool, key string, val string) error {
	return g.run("config", configScope(global), key, val)
}

func (g *GitCLI) SetConfigBool(global bool, key string, val bool) error {
	return g.run("config", configScope(global), "--type", "bool", key, strconv.FormatBool(val))
}

func (g *GitCLI) SetConfigInt(global bool, key string, val int64) error {
	return g.run("config", configScope(global), "--type", "int", key, strconv.FormatInt(val, 10))
}

func (g *GitCLI) AddConfigStr(global bool, key string, val string) error {
	return g.run("config", configScope(global), "--add", key, val)
}

func (g *GitCLI) AddConfigBool(global bool, key string, val bool) error {
	return g.run("config", configScope(global), "--type", "bool", "--add", key, strconv.FormatBool(val))
}

func (g *GitCLI) AddConfigInt(global bool, key string, val int64) error {
	return g.run("config", configScope(global), "--type", "int", "--add", key, strconv.FormatInt(val, 10))
}

func (g *GitCLI) GetConfig(global bool, key string) (string, error) {
	output, err := g.runOutput("config", configScope(global), "--get", "--null", key)
	if err != nil {
		return "", err
	}
	output = strings.TrimSuffix(output, "\x00")
	return output, nil
}

func (g *GitCLI) GetAllConfig(global bool, key string) ([]string, error) {
	output, err := g.runOutput("config", configScope(global), "--get-all", "--null", key)
	if err != nil {
		return nil, err
	}
	return strings.Split(output, "\x00"), nil
}

func (g *GitCLI) UnsetConfig(global bool, key string) (bool, error) {
	err := g.run("config", configScope(global), "--unset-all", key)
	if e := (&exec.ExitError{}); err != nil && errors.As(err, &e) {
		return false, nil
	}
	return err == nil, err
}

// IsDetached returns true if the current working directory is part of a git workspace that is in a detached head state
func (g *GitCLI) IsDetached() (bool, error) {
	// Note `branch --show-current` would be simpler but rev-parse is part of the git API whereas branch is user facing
	output, err := g.runOutput("rev-parse", "--symbolic-full-name", "--verify", "--quiet", "HEAD")
	if err != nil {
		return false, err
	}
	return !strings.HasPrefix(strings.TrimSpace(output), "ref/heads/"), nil
}

// CheckoutDetach detaches the current working directory if it is part of a git workspace.
func (g *GitCLI) CheckoutDetach() error {
	return g.run("checkout", "--detach")
}

func (g *GitCLI) SubmoduleStatus() error {
	return g.run("submodule", "status")
}

func (g *GitCLI) ShaExists(sha string) (bool, error) {
	err := g.run("rev-parse", "--verify", "--quiet", sha+"^{object}")
	if e := (&exec.ExitError{}); err != nil && errors.As(err, &e) {
		return false, nil
	}
	return err == nil, err
}

func (g *GitCLI) RevParse(ref string) (string, error) {
	output, err := g.runOutput("rev-parse", ref)
	return strings.TrimSpace(output), err
}

func (g *GitCLI) LfsFetch(ref string) error {
	return g.run("lfs", "fetch", "origin", ref)
}

func (g *GitCLI) LfsInstall() error {
	return g.run("lfs", "install", "--local")
}

func (g *GitCLI) SparseCheckout(dirs []string) error {
	args := []string{"sparse-checkout", "set"}
	args = append(args, dirs...)
	return g.run(dirs...)
}

func (g *GitCLI) SparseCheckoutNonConeMode(patterns []string) (err error) {
	if err = g.SetConfigBool(false, "core.sparseCheckout", true); err != nil {
		return err
	}
	output, err := g.runOutput("rev-parse", "--git-path", "info/sparse-checkout")
	if err != nil {
		return err
	}

	f, err := os.OpenFile(strings.TrimSpace(output), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	defer func() {
		e := f.Close()
		if err == nil {
			err = e
		}
	}()

	if _, err = f.WriteString("\n" + strings.Join(patterns, "\n") + "\n"); err != nil {
		return err
	}
	return nil
}

func (g *GitCLI) Checkout(ref string, startPoint string) error {
	args := []string{"checkout", "--progress", "--force"}
	if startPoint != "" {
		args = append(args, "-B", ref, startPoint)
	} else {
		args = append(args, ref)
	}
	return g.run(args...)
}

func (g *GitCLI) SubmoduleSync(recursive bool) error {
	args := []string{"submodule", "sync"}

	if recursive {
		args = append(args, "--recursive")
	}

	return g.run(args...)
}

func (g *GitCLI) SubmoduleUpdate(fetchDepth int, recursive bool) error {
	args := []string{"-c", "protocol.version=2", "submodule", "update", "--init", "--force"}

	if fetchDepth > 0 {
		args = append(args, fmt.Sprintf("--depth=%d", fetchDepth))
	}

	if recursive {
		args = append(args, "--recursive")
	}

	return g.run(args...)
}

func (g *GitCLI) SubmoduleForeach(recursive bool, cmd ...string) (string, error) {
	args := []string{"submodule", "foreach"}

	if recursive {
		args = append(args, "--recursive")
	}

	args = append(args, cmd...)

	return g.runOutput(args...)
}

func (g *GitCLI) Clean() error {
	return g.run("clean", "-ffdx")
}

func (g *GitCLI) Log1(format ...string) (string, error) {
	a := []string{"log", "-1"}
	a = append(a, format...)
	if len(format) == 0 {
		return g.silentRunOutput(a...)
	}
	return g.runOutput(a...)
}

func (g *GitCLI) Reset() error {
	return g.run("reset", "--hard", "HEAD")
}

func (g *GitCLI) Init(path string) error {
	return g.run("init", "--quiet", path)
}

func (g *GitCLI) RemoteAdd(name string, url string) error {
	return g.run("remote", "add", name, url)
}

func (g *GitCLI) Merge(repositoryURL, baseSHA, headSHA, committerDate, workingDir string) (string, error) {
	if g.mergeBinary == "" {
		return "", fmt.Errorf("merge binary not found")
	}

	stdout, err := g.runMerge("merge", "--clone-url", repositoryURL,
		"--base-sha", baseSHA,
		"--head-sha", headSHA,
		"--committer-date", committerDate,
		"--working-dir", workingDir)

	return stdout, err
}

type FetchOptions struct {
	Filter          string
	FetchDepth      int
	LocalRepository string
}

func (g *GitCLI) Fetch(refSpec []string, options FetchOptions) error {
	args := []string{"-c", "protocol.version=2", "fetch"}

	tags := false
	for _, r := range refSpec {
		if r == "+refs/tags/*:refs/tags/*" {
			tags = true
			break
		}
	}

	if !tags {
		args = append(args, "--no-tags")
	}

	args = append(args, "--prune", "--progress", "--no-recurse-submodules")

	if options.Filter != "" {
		args = append(args, "--filter="+options.Filter)
	}

	if options.FetchDepth > 0 {
		args = append(args, fmt.Sprintf("--depth=%d", options.FetchDepth))
	} else {
		out, err := g.runOutput("rev-parse", "--is-shallow-repository")
		if err != nil {
			return err
		}
		if shallow, _ := strconv.ParseBool(strings.TrimSpace(out)); shallow {
			args = append(args, "--unshallow")
		}
	}

	if options.LocalRepository != "" {
		args = append(args, options.LocalRepository)
	} else {
		args = append(args, "origin")
	}
	args = append(args, refSpec...)

	return g.run(args...)
}
