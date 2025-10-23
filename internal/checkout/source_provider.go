package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	path2 "path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudbees-io/checkout/internal/auth"
	"github.com/cloudbees-io/checkout/internal/core"
	"github.com/cloudbees-io/checkout/internal/git"
	"github.com/google/uuid"
)

const dotGitCloneURLSuffix = ".git"

type Config struct {
	Repository             string
	repositoryCloneURL     string
	Ref                    string
	CloudBeesApiToken      string
	CloudBeesApiURL        string
	SSHKey                 string
	SSHKnownHosts          string
	SSHStrict              bool
	PersistCredentials     bool
	Path                   string
	Clean                  bool
	SparseCheckout         string
	SparseCheckoutConeMode bool
	FetchDepth             int
	Lfs                    bool
	Submodules             string
	SetSafeDirectory       bool
	Commit                 string
	eventContext           map[string]interface{}
	LocalMerge             bool
}

var shaRegex = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func (cfg *Config) validate() error {
	// Load event context
	err := cfg.findEventContext()
	if err != nil {
		return fmt.Errorf("loading event context: %w", err)
	}

	// CloudBees Workspace
	workspacePath, found := os.LookupEnv("CLOUDBEES_WORKSPACE")
	if !found {
		return fmt.Errorf("environment variable CLOUDBEES_WORKSPACE is not defined")
	}

	core.Debug("CLOUDBEES_WORKSPACE = %s", workspacePath)

	if err := core.DirExists(workspacePath, true); err != nil {
		return err
	}

	core.Debug("repository = %s", cfg.Repository)

	cfg.repositoryCloneURL, err = normalizeRepositoryURL(cfg.Repository, cfg.SSHKey != "")
	if err != nil {
		return err
	}

	// Repository Path
	if cfg.Path == "" {
		cfg.Path = "."
	}

	cleanWorkspacePath := filepath.Clean(workspacePath)
	repositoryPath := filepath.Join(cleanWorkspacePath, cfg.Path)
	if !strings.HasPrefix(filepath.Clean(repositoryPath), cleanWorkspacePath) {
		return fmt.Errorf("repository path '%s' is not under '%s'", filepath.Join(workspacePath, cfg.Path), workspacePath)
	}

	// workflow repository ?
	isWorkflowRepository := cfg.isWorkflowRepository(cfg.eventContext)
	core.Debug("isWorkflowRepository = %v", isWorkflowRepository)

	// source branch, source version
	if cfg.Ref == "" {
		if isWorkflowRepository {
			if r, found := getStringFromMap(cfg.eventContext, "ref"); found {
				cfg.Ref = r
			}
			if s, found := getStringFromMap(cfg.eventContext, "sha"); found {
				cfg.Commit = s
			}

			if cfg.Commit != "" && cfg.Ref != "" && !strings.HasPrefix(cfg.Ref, "refs/") {
				// Some events have an unqualifed ref. For example when a PR is merged (pull_request closed event),
				// the ref is unqualifed like "main" instead of "refs/heads/main".
				cfg.Ref = "refs/heads/" + cfg.Ref
			}
		}
	} else if shaRegex.MatchString(cfg.Ref) {
		// the Ref is a SHA so checkout the sha as a detached head
		cfg.Commit = cfg.Ref
		cfg.Ref = ""
	}
	core.Debug("ref = %s", cfg.Ref)
	core.Debug("commit = %s", cfg.Commit)

	// Clean
	core.Debug("clean = %v", cfg.Clean)

	// Sparse checkout
	core.Debug("sparse checkout = %s", cfg.SparseCheckout)

	// Fetch depth
	core.Debug("fetch depth = %d", cfg.FetchDepth)

	// LFS
	core.Debug("lfs = %v", cfg.Lfs)

	// Submodules
	switch cfg.Submodules {
	case "true":
		core.Debug("submodules = true")
		core.Debug("recursive submodules = false")
	case "false":
		core.Debug("submodules = false")
		core.Debug("recursive submodules = false")
	case "recursive":
		core.Debug("submodules = true")
		core.Debug("recursive submodules = true")
	default:
		return fmt.Errorf("unsupported submodules: '%s', expected true/false/recursive", cfg.Submodules)
	}

	// Authentication
	if cfg.CloudBeesApiToken == "" && cfg.CloudBeesApiURL == "" && cfg.SSHKey == "" {
		return fmt.Errorf("no means of authentication specified. you need to specify either cloudbees-api-token and cloudbees-api-url or ssh-key")
	}

	return nil
}

func normalizeRepositoryURL(repoURL string, ssh bool) (string, error) {
	if isSSHURL(repoURL) {
		// Handle SSH URL
		if !ssh {
			return "", errors.New("must also specify the ssh-key input when specifying an SSH URL as repository input")
		}

		repoURL = normalizeSSHURL(repoURL)
	} else {
		// Handle HTTP URL
		if ssh {
			return "", errors.New("ssh-key input is not supported when provided an HTTP URL within the repository input")
		}

		parsedURL, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("invalid repository %q: %w", repoURL, err)
		}

		if !parsedURL.IsAbs() || parsedURL.Host == "" {
			return "", fmt.Errorf("invalid repository URL %q provided, expects full clone URL", repoURL)
		}
	}

	return repoURL, nil
}

func (cfg *Config) writeActionOutputs(cli *git.GitCLI) error {
	//Output commit details
	outDir := os.Getenv("CLOUDBEES_OUTPUTS")
	fullUrl := cfg.repositoryCloneURL
	err := os.WriteFile(filepath.Join(outDir, "repository-url"), []byte(fullUrl), 0640)
	if err != nil {
		return err
	}
	commitId := cfg.Commit
	if commitId == "" {
		commitId, err = cli.GetLastCommitId()
		if err != nil {
			return err
		}
	}
	// If it's a local merge, set commit ID to empty.
	if cfg.LocalMerge {
		commitId = ""
	}
	err = os.WriteFile(filepath.Join(outDir, "commit"), []byte(commitId), 0640)
	if err != nil {
		return err
	}

	// Use input ref or branch if not supplied
	ref := cfg.Ref
	if ref == "" {
		ref, err = cli.GetCurrentBranch()
		if err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(outDir, "ref"), []byte(ref), 0640)
}

func (cfg *Config) findEventContext() error {
	if eventPath, found := os.LookupEnv("CLOUDBEES_EVENT_PATH"); found {
		evtCtx, err := loadEventContext(eventPath)
		if err != nil {
			return fmt.Errorf("loading event context from file: %w", err)
		}
		cfg.eventContext = evtCtx
		return nil
	}
	return fmt.Errorf("missing event context")
}

func (cfg *Config) Run(ctx context.Context) (retErr error) {
	// validate the configuration
	if err := cfg.validate(); err != nil {
		return err
	}

	// now start getting the source code

	useSSH := cfg.SSHKey != ""

	cli, err := git.NewGitCLI(ctx)
	if err != nil {
		return err
	}

	repositoryURL := cfg.repositoryCloneURL

	fmt.Printf("Syncing Repository: %s\n", repositoryURL)

	// Remove conflicting file path

	homePath, haveHome := os.LookupEnv("HOME")
	if !haveHome {
		return fmt.Errorf("missing HOME environment variable")
	}

	workspacePath, haveWork := os.LookupEnv("CLOUDBEES_WORKSPACE")
	if !haveWork {
		workspacePath = path2.Join(homePath, "workspace")
	}

	if err := os.MkdirAll(workspacePath, os.ModePerm); err != nil {
		return err
	}

	// best effort canonicalize the workspace Path
	if w, err := filepath.EvalSymlinks(workspacePath); err == nil {
		workspacePath = w
	}

	if w, err := filepath.Abs(workspacePath); err == nil {
		workspacePath = w
	}

	repositoryPath := path2.Join(workspacePath, cfg.Path)

	// best effort canonicalize the Repository Path
	if r, err := filepath.EvalSymlinks(repositoryPath); err == nil {
		repositoryPath = r
	}
	if r, err := filepath.Abs(repositoryPath); err == nil {
		repositoryPath = r
	}

	if !strings.HasPrefix(repositoryPath+string(os.PathSeparator), workspacePath+string(os.PathSeparator)) {
		return fmt.Errorf("Repository Path '%s' is not under '%s'", repositoryPath, workspacePath)
	}

	// if repositoryPath exists but is a file, remove the file
	if stat, err := os.Stat(repositoryPath); err == nil && !stat.IsDir() {
		if err := os.Remove(repositoryPath); err != nil {
			return fmt.Errorf("could not remove conflicting file at Repository Path '%s': %v", repositoryPath, err)
		}
	}

	// Create directory
	if _, err := os.Stat(repositoryPath); err != nil {
		if err := os.MkdirAll(repositoryPath, os.ModePerm); err != nil {
			return fmt.Errorf("could not create directory '%s': %v", repositoryPath, err)
		}
	}

	// Set up Git CLI
	uniqueID := uuid.New().String()

	temp, haveTemp := os.LookupEnv("RUNNER_TEMP")
	if !haveTemp {
		temp = os.TempDir()
	}
	cli.SetEnv("RUNNER_TEMP", temp)

	if cfg.SetSafeDirectory {
		fmt.Println("Adding Repository directory to the temporary git global config as a safe directory")
		if err := cli.AddConfigStr(true, "safe.directory", workspacePath); err != nil {
			return err
		}
	}

	core.Debug("Repository Path = %s", repositoryPath)
	cli.SetCwd(repositoryPath)

	// Prepare existing directory, otherwise recreate
	if err := prepareExistingDirectory(cli, repositoryPath, repositoryURL, cfg.Clean, cfg.Ref); err != nil {
		return err
	}

	// Initialize the Repository
	if _, err := os.Stat(filepath.Join(repositoryPath, ".git")); err != nil {
		core.StartGroup("Initializing the Repository")
		if err := cli.Init(repositoryPath); err != nil {
			return err
		}
		if err := cli.RemoteAdd("origin", repositoryURL); err != nil {
			return err
		}
		core.EndGroup("Repository initialized")
	}

	// Disable automatic garbage collection
	core.StartGroup("Disabling automatic garbage collection")
	if err := cli.SetConfigInt(false, "gc.auto", 0); err != nil {
		fmt.Println("Unable to turn off git automatic garbage collection. The git fetch operation may trigger garbage collection and cause a delay.")
	}
	core.EndGroup("Automatic garbage collection disabled")

	// Setup auth
	core.StartGroup("Setting up auth")
	var sshKeyPath string
	var sshKnownHostsPath string
	var sshCommand string
	if useSSH {
		if sshKeyPath, err = auth.GenerateSSHKey(ctx, temp, uniqueID, cfg.SSHKey); err != nil {
			return err
		}

		if sshKnownHostsPath, err = auth.GenerateSSHKnownHosts(homePath, temp, uniqueID, cfg.SSHKnownHosts); err != nil {
			return err
		}

		if sshCommand, err = auth.GenerateSSHCommand(sshKeyPath, cfg.SSHStrict, sshKnownHostsPath); err != nil {
			return err
		}

		cli.SetEnv("GIT_SSH_COMMAND", sshCommand)

		defer func() {
			if !cfg.PersistCredentials {
				if err := os.Remove(sshKeyPath); err != nil && retErr == nil {
					retErr = err
				}
				if err := os.Remove(sshKnownHostsPath); err != nil && retErr == nil {
					retErr = err
				}
			} else {
				if err := cli.SetConfigStr(false, "core.sshCommand", sshCommand); err != nil && retErr == nil {
					retErr = err
				}
			}
		}()
	}

	cleaner, helperCommand, err := auth.ConfigureToken(
		cli, "", false, auth.TokenAuth{
			ApiToken: cfg.CloudBeesApiToken,
			ApiURL:   cfg.CloudBeesApiURL,
		})
	if err != nil {
		return err
	}
	defer func() {
		if !cfg.PersistCredentials {
			if err := cleaner(); err != nil {
				if retErr == nil {
					retErr = err
				} else {
					retErr = errors.Join(retErr, err)
				}
			}
		}
	}()

	core.EndGroup("Auth setup")

	// Determine the default branch
	if cfg.Ref == "" && cfg.Commit == "" {
		core.StartGroup("Determining the default branch")
		cfg.Ref, err = cli.BranchGetDefault(repositoryURL)
		if err != nil {
			return err
		}
		core.EndGroup("Default branch determined")
	}

	// LFS install
	if cfg.Lfs {
		if err := cli.LfsInstall(); err != nil {
			return err
		}
	}

	mergeLoc, err := cfg.doLocalMerge(cli, repositoryURL, helperCommand)
	if err != nil {
		return err
	}

	// Fetch the Repository
	core.StartGroup("Fetching the Repository")
	var fetchOptions git.FetchOptions
	if cfg.SparseCheckout != "" {
		fetchOptions.Filter = "blob:none"
	}
	if mergeLoc != "" {
		fetchOptions.LocalRepository = mergeLoc
	}

	if cfg.FetchDepth <= 0 {
		if err := cli.Fetch(getRefSpecForAllHistory(cfg.Ref, cfg.Commit), fetchOptions); err != nil {
			return err
		}

		// When all history is fetched, the Ref we're interested in may have moved to a different
		// commit (push or force push). If so, fetch again with a targeted refspec.
		if refPresent, err := testRef(cli, cfg.Ref, cfg.Commit); err != nil {
			return err
		} else if !refPresent {
			if err := cli.Fetch(getRefSpec(cfg.Ref, cfg.Commit), fetchOptions); err != nil {
				return err
			}
		}
	} else {
		fetchOptions.FetchDepth = cfg.FetchDepth
		if err := cli.Fetch(getRefSpec(cfg.Ref, cfg.Commit), fetchOptions); err != nil {
			return err
		}
	}
	core.EndGroup("Repository fetched")

	// Checkout info
	core.StartGroup("Determining the checkout info")
	checkoutInfo, err := getCheckoutInfo(cli, cfg.Ref, cfg.Commit)
	if err != nil {
		return err
	}
	core.EndGroup("Checkout info determined")

	// LFS fetch
	// Explicit lfs-fetch to avoid slow checkout (fetches one lfs object at a time).
	// Explicit lfs fetch will fetch lfs objects in parallel.
	// For sparse checkouts, let `checkout` fetch the needed objects lazily.
	if cfg.Lfs && cfg.SparseCheckout == "" {
		core.StartGroup("Fetching LFS objects")
		r := checkoutInfo.startPoint
		if r == "" {
			r = checkoutInfo.ref
		}
		if err := cli.LfsFetch(r); err != nil {
			return err
		}
		core.EndGroup("LFS objects fetched")
	}

	// Sparse checkout
	if cfg.SparseCheckout != "" {
		core.StartGroup("Setting up sparse checkout")
		if cfg.SparseCheckoutConeMode {
			if err := cli.SparseCheckout(strings.Split(cfg.SparseCheckout, "\n")); err != nil {
				return err
			} else if err := cli.SparseCheckoutNonConeMode(strings.Split(cfg.SparseCheckout, "\n")); err != nil {
				return err
			}
		}
		core.EndGroup("Sparse checkout setup")
	}

	// Checkout
	core.StartGroup("Checking out the Ref")
	if err := cli.Checkout(checkoutInfo.ref, checkoutInfo.startPoint); err != nil {
		return err
	}
	core.EndGroup("Ref checked out")

	if err := cfg.writeActionOutputs(cli); err != nil {
		// Report warning but do not block checkout
		log.Printf("Warning: failed to write checkout action outputs: %v", err)
	}
	// Submodules
	cfg.Submodules = strings.ToLower(strings.TrimSpace(cfg.Submodules))
	if cfg.Submodules == "true" || cfg.Submodules == "recursive" {
		// Temporarily override global config
		core.StartGroup("Setting up auth for fetching submodules")

		cleaner, _, err := auth.ConfigureToken(cli, "", true, auth.TokenAuth{
			ApiToken: cfg.CloudBeesApiToken,
			ApiURL:   cfg.CloudBeesApiURL,
		})
		if err != nil {
			return err
		}

		core.EndGroup("Auth for submodules configured")

		// Checkout submodules
		core.StartGroup("Fetching submodules")
		recursive := cfg.Submodules == "recursive"
		if err := cli.SubmoduleSync(recursive); err != nil {
			return err
		}
		if err := cli.SubmoduleUpdate(cfg.FetchDepth, recursive); err != nil {
			return err
		}
		if _, err := cli.SubmoduleForeach(recursive, cli.Executable(), "config", "--local", "gc.auto", "0"); err != nil {
			return err
		}
		core.EndGroup("Submodules fetched")

		if !cfg.PersistCredentials {
			if err := cleaner(); err != nil {
				return err
			}
		}
	}

	// Log commit sha
	_, err = cli.Log1("--format='%H'")
	if err != nil {
		return err
	}

	// remove auth - already handled by defer functions

	if os.Getenv("DEBUG_SHELL") != "" {
		c := exec.CommandContext(ctx, "sh", "-i")
		c.Dir = workspacePath
		c.Env = os.Environ()
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		if err = c.Start(); err != nil {
			return err
		}

		if err = c.Wait(); err != nil {

			return err
		}
	}

	return nil
}

func (cfg *Config) doLocalMerge(cli *git.GitCLI, repositoryURL string, credsHelperCmd string) (fetchLoc string, err error) {
	commitRef := cfg.Commit
	if cfg.Commit == "" {
		commitRef = cfg.Ref
	}

	cmdOut, err := cli.Merge(repositoryURL, commitRef, cfg.FetchDepth, credsHelperCmd)
	if err != nil {
		return "", fmt.Errorf("failed to call the merge binary: %w", err)
	}

	if cmdOut == "" {
		return "", nil
	}

	var mergeData map[string]interface{}
	if err := json.Unmarshal([]byte(cmdOut), &mergeData); err != nil {
		return "", fmt.Errorf("failed to unmarshal merge output: %w", err)
	}

	if mergeCommit, ok := getStringFromMap(mergeData, "merge_commit"); ok {
		cfg.Commit = mergeCommit
		cfg.Ref = ""
		cfg.LocalMerge = true

		fmt.Printf("Pull request merged with commit: %s\n", mergeCommit)
		if fetchLoc, ok := getStringFromMap(mergeData, "fetched_loc"); ok {
			return fetchLoc, nil
		} else {
			return "", fmt.Errorf("missing fetch location in merge output")
		}
	}

	return "", nil
}

// loadEventContext attempts to load the event context from the JSON file at the supplied path.
func loadEventContext(path string) (map[string]interface{}, error) {
	var bytes []byte
	var err error

	if bytes, err = os.ReadFile(path); err != nil {
		// best effort
		return nil, err
	}

	var event map[string]interface{}
	if err = json.Unmarshal(bytes, &event); err != nil {
		// best effort
		return nil, err
	}

	return event, nil
}

func (cfg *Config) isWorkflowRepository(eventContext map[string]interface{}) bool {
	ctxRepository, haveR := getStringFromMap(eventContext, "repositoryUrl")
	normalizedCtxRepository, _ := normalizeRepositoryURL(ctxRepository, false)
	normalizedCtxRepository = stripDotGitExtension(normalizedCtxRepository)
	normalizedCfgRepository := stripDotGitExtension(cfg.repositoryCloneURL)
	core.Debug("ctx.repository = %s", ctxRepository)
	core.Debug("cfg.repository = %s", cfg.Repository)
	core.Debug("cfg.repositoryCloneURL = %s", cfg.repositoryCloneURL)

	return haveR && (ctxRepository != "" && cfg.Repository == ctxRepository || normalizedCtxRepository != "" && normalizedCfgRepository == normalizedCtxRepository)
}

func stripDotGitExtension(repoURL string) string {
	if strings.HasSuffix(repoURL, dotGitCloneURLSuffix) {
		return repoURL[:len(repoURL)-len(dotGitCloneURLSuffix)]
	}

	return repoURL
}

func getStringFromMap(m map[string]interface{}, key string) (string, bool) {
	i, found := m[key]
	if !found {
		return "", false
	}
	if s, ok := i.(string); ok {
		return s, true
	}
	return "", false
}

func getBoolFromMap(m map[string]interface{}, key string) (bool, bool) {
	i, found := m[key]
	if !found {
		return false, false
	}
	if v, ok := i.(bool); ok {
		return v, true
	}
	return false, false
}

func getMapFromMap(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	i, found := m[key]
	if !found {
		return map[string]interface{}{}, false
	}
	if v, ok := i.(map[string]interface{}); ok {
		return v, true
	}
	return map[string]interface{}{}, false
}

type CheckoutInfo struct {
	ref        string
	startPoint string
}

func getCheckoutInfo(cli *git.GitCLI, ref string, commit string) (*CheckoutInfo, error) {
	if ref == "" && commit == "" {
		return nil, fmt.Errorf("Ref and commit cannot both be empty")
	}

	var result CheckoutInfo
	lowerRef := strings.ToLower(ref)

	if ref == "" {
		result.ref = commit
	} else if strings.HasPrefix(lowerRef, "refs/heads/") {
		result.ref = ref[len("refs/heads/"):]
		result.startPoint = "refs/remotes/origin/" + result.ref
	} else if strings.HasPrefix(lowerRef, "refs/pull/") {
		result.ref = ref[len("refs/pull/"):]
		result.startPoint = "refs/remotes/pull/" + result.ref
	} else if strings.HasPrefix(lowerRef, "refs/") {
		result.ref = ref
	} else {
		exists, err := cli.BranchExists(true, "origin/"+ref)
		if err != nil {
			return nil, err
		}
		if exists {
			result.ref = ref
			result.startPoint = "refs/remotes/origin/" + result.ref
		} else {
			exists, err = cli.TagExists(ref)
			if err != nil {
				return nil, err
			}
			if exists {
				result.ref = "refs/tags/" + ref
			}
		}
		if !exists {
			return nil, fmt.Errorf("a branch or tag with the name '%s' could not be found", ref)
		}
	}
	return &result, nil
}

func testRef(cli *git.GitCLI, ref string, commit string) (bool, error) {
	if ref == "" && commit == "" {
		return false, fmt.Errorf("Ref and commit cannot both be empty")
	}

	// No SHA? Nothing to test
	if commit == "" {
		return true, nil
	}

	// SHA only
	if ref == "" {
		return cli.ShaExists(commit)
	}

	lowerRef := strings.ToLower(ref)

	if strings.HasPrefix(lowerRef, "refs/heads/") {
		branch := ref[len("refs/heads/"):]
		if exists, err := cli.BranchExists(true, "origin/"+branch); err != nil || !exists {
			return false, err
		}
		if c, err := cli.RevParse("refs/remotes/origin/" + branch); err != nil || commit != c {
			return false, err
		}
		return true, nil
	}

	if strings.HasPrefix(lowerRef, "refs/pull/") {
		// assume matches because fetched using the commit
		return true, nil
	}

	if strings.HasPrefix(lowerRef, "refs/tags/") {
		tagName := ref[len("refs/tags/"):]
		if exists, err := cli.TagExists(tagName); err != nil || !exists {
			return false, err
		}
		if c, err := cli.RevParse(ref); err != nil || commit != c {
			return false, err
		}
		return true, nil
	}

	return false, fmt.Errorf("unexpected Ref format '%s' when testing Ref info", ref)
}

func getRefSpecForAllHistory(ref string, commit string) []string {
	r := []string{"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*"}
	if ref != "" && strings.HasPrefix(strings.ToLower(ref), "refs/pull/") {
		branch := ref[len("refs/pull/"):]
		if commit != "" {
			r = append(r, fmt.Sprintf("+%s:refs/remotes/pull/%s", commit, branch))
		} else {
			r = append(r, fmt.Sprintf("+%s:refs/remotes/pull/%s", ref, branch))
		}
	}
	return r
}

func getRefSpec(ref string, commit string) []string {
	lowerRef := strings.ToLower(ref)

	if commit != "" {
		if strings.HasPrefix(lowerRef, "refs/heads/") {
			branch := ref[len("refs/heads/"):]
			return []string{fmt.Sprintf("+%s:refs/remotes/origin/%s", commit, branch)}
		}

		if strings.HasPrefix(lowerRef, "refs/pull/") {
			branch := ref[len("refs/pull/"):]
			return []string{fmt.Sprintf("+%s:refs/remotes/pull/%s", commit, branch)}
		}

		if strings.HasPrefix(lowerRef, "refs/tags/") {
			return []string{fmt.Sprintf("+%s:%s", commit, ref)}
		}

		return []string{commit}
	}

	// Unqualified Ref, check for a matching branch or tag
	if !strings.HasPrefix(lowerRef, "refs/") {
		return []string{
			fmt.Sprintf("+refs/heads/%s*:refs/remotes/origin/%s*", ref, ref),
			fmt.Sprintf("+refs/tags/%s*:refs/tags/%s*", ref, ref),
		}
	}

	if strings.HasPrefix(lowerRef, "refs/heads/") {
		branch := ref[len("refs/heads/"):]
		return []string{fmt.Sprintf("+%s:refs/remotes/origin/%s", ref, branch)}
	}

	if strings.HasPrefix(lowerRef, "refs/pull/") {
		branch := ref[len("refs/pull/"):]
		return []string{fmt.Sprintf("+%s:refs/remotes/pull/%s", ref, branch)}
	}

	return []string{fmt.Sprintf("+%s:%s", ref, ref)}
}

func prepareExistingDirectory(cli *git.GitCLI, repositoryPath string, repositoryURL string, clean bool, ref string) (reterr error) {
	remove := false

	if stat, err := os.Stat(filepath.Join(repositoryPath, ".git")); err != nil || !stat.IsDir() {
		remove = true
	}

	if !remove {
		origin, err := cli.GetConfig(false, "remote.origin.url")
		if err != nil || repositoryURL != strings.TrimSpace(origin) {
			remove = true
		}
	}

	if !remove {
		// Best effort delete any index.lock and shallow.lock left by a previously canceled run or crashed process
		for _, n := range []string{"index.lock", "shallow.lock"} {
			lockPath := filepath.Join(repositoryPath, ".git", n)
			if _, err := os.Stat(lockPath); err != nil {
				if err := os.Remove(lockPath); err != nil {
					fmt.Printf("Unable to delete '%s': %v", lockPath, err)
					remove = true
					break
				}
			}
		}
	}

	if !remove {
		fmt.Println("Removing previously created refs, to avoid conflicts")

		// checkout detached HEAD so that we can remove all branches safely
		if detached, err := cli.IsDetached(); err != nil {
			fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
			remove = true
		} else if !detached {
			if err := cli.CheckoutDetach(); err != nil {
				fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
				remove = true
			}
		}
	}

	if !remove {
		branches, err := cli.BranchList(false)
		if err != nil {
			fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
			remove = true
		} else {
			for _, b := range branches {
				if err := cli.BranchDelete(false, b); err != nil {
					fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
					remove = true
					break
				}
			}
		}
	}

	if !remove {
		// Remove any conflicting refs/remotes/origin/*
		// Example 1: Consider Ref is refs/heads/foo and previously fetched refs/remotes/origin/foo/bar
		// Example 2: Consider Ref is refs/heads/foo/bar and previously fetched refs/remotes/origin/foo

		if !strings.HasPrefix(ref, "refs/") {
			ref = "refs/heads/" + ref
		}

		if strings.HasPrefix(ref, "refs/heads/") {
			name1 := strings.ToLower(strings.TrimPrefix(ref, "refs/heads/"))
			name1Slash := name1 + "/"
			branches, err := cli.BranchList(true)
			if err != nil {
				fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
				remove = true
			} else {
				for _, b := range branches {
					name2 := strings.ToLower(strings.TrimPrefix(b, "origin/"))
					name2Slash := name2 + "/"
					if strings.HasPrefix(name1, name2Slash) || strings.HasPrefix(name2, name1Slash) {
						if err := cli.BranchDelete(true, b); err != nil {
							fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
							remove = true
							break
						}
					}
				}
			}
		}
	}

	if !remove {
		// Check for submodules and delete any existing files if submodules are present
		if err := cli.SubmoduleStatus(); err != nil {
			fmt.Println("Bad Submodules found, removing existing files")
			remove = true
		}
	}

	if !remove {
		// Clean
		if clean {
			if err := cli.Clean(); err != nil {
				fmt.Printf("The Clean command failed. This might be caused by: 1) Path too long, 2) permission issue, or 3) file in use. For further investigation, manually run 'git Clean -ffdx' on the directory '%s'.\n", repositoryPath)
				fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
				remove = true
			} else if err := cli.Reset(); err != nil {
				fmt.Println("Unable to prepare the existing Repository. The Repository will be recreated instead.")
				remove = true
			}
		}
	}

	if remove {
		// Delete the contents of the directory. Don't delete the directory itself
		// since it might be the current working directory.

		d, err := os.Open(repositoryPath)
		if err != nil {
			return err
		}
		defer (func() {
			err := d.Close()
			if err != nil && reterr == nil {
				reterr = err
			}
		})()
		names, err := d.Readdirnames(-1)
		if err != nil {
			return err
		}

		if len(names) == 0 {
			return nil
		}

		fmt.Printf("Deleting the contents of '%s'\n", repositoryPath)

		for _, name := range names {
			err = os.RemoveAll(filepath.Join(repositoryPath, name))
			if err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}
