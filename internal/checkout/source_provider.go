package checkout

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	path2 "path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudbees-io/actions-checkout/internal/auth"
	"github.com/cloudbees-io/actions-checkout/internal/core"
	"github.com/cloudbees-io/actions-checkout/internal/git"
	"github.com/google/uuid"
)

type Config struct {
	Provider                     string
	Repository                   string
	Ref                          string
	Token                        string
	SSHKey                       string
	SSHKnownHosts                string
	SSHStrict                    bool
	PersistCredentials           bool
	Path                         string
	Clean                        bool
	SparseCheckout               string
	SparseCheckoutConeMode       bool
	FetchDepth                   int
	Lfs                          bool
	Submodules                   string
	SetSafeDirectory             bool
	GithubServerURL              string
	BitbucketServerURL           string
	GitlabServerURL              string
	Commit                       string
	githubWorkflowOrganizationId string
}

const (
	GitHubProvider    = "github"
	GitLabProvider    = "gitlab"
	BitbucketProvider = "bitbucket"
	CustomProvider    = "custom"
)

var shaRegex = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func (cfg *Config) validate() error {
	// Load event context
	eventContext := findEventContext()

	// CloudBees Workspace
	workspacePath, found := os.LookupEnv("CLOUDBEES_WORKSPACE")
	if !found {
		return fmt.Errorf("environment variable CLOUDBEES_WORKSPACE is not defined")
	}

	core.Debug("CLOUDBEES_WORKSPACE = %s", []any{workspacePath}...)

	if err := core.DirExists(workspacePath, true); err != nil {
		return err
	}

	// Provider
	if cfg.Provider == "" {
		cfg.Provider, _ = getStringFromMap(eventContext, "provider")
	}
	cfg.Provider = strings.TrimSpace(strings.ToLower(cfg.Provider))
	if cfg.Provider == "" {
		return fmt.Errorf("input required and not supplied: provider")
	}
	core.Debug("provider = %s", []any{cfg.Provider}...)

	// Repository
	if cfg.Provider != CustomProvider {
		splitRepository := strings.Split(cfg.Repository, "/")
		if len(splitRepository) != 2 || splitRepository[0] == "" || splitRepository[1] == "" {
			return fmt.Errorf("invalid repository '%s', expected format {owner}/{repo}", cfg.Repository)
		}
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
	isWorkflowRepository := cfg.isWorkflowRepository(eventContext)

	// source branch, source version
	if cfg.Ref == "" {
		if isWorkflowRepository {
			if r, found := getStringFromMap(eventContext, "ref"); found {
				cfg.Ref = r
			}
			if s, found := getStringFromMap(eventContext, "sha"); found {
				cfg.Commit = s
			}

			if cfg.Provider == GitHubProvider && cfg.Commit != "" && cfg.Ref != "" && !strings.HasPrefix(cfg.Ref, "refs/") {
				// For GITHUB only
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
	core.Debug("ref = %s", []any{cfg.Ref}...)
	core.Debug("commit = %s", []any{cfg.Commit}...)

	// Clean
	core.Debug("clean = %v", []any{cfg.Clean}...)

	// Sparse checkout
	core.Debug("sparse checkout = %s", []any{cfg.SparseCheckout}...)

	// Fetch depth
	core.Debug("fetch depth = %d", []any{cfg.FetchDepth}...)

	// LFS
	core.Debug("lfs = %v", []any{cfg.Lfs}...)

	// Submodules
	switch cfg.Submodules {
	case "true":
		core.Debug("submodules = true", []any{}...)
		core.Debug("recursive submodules = false", []any{}...)
	case "false":
		core.Debug("submodules = false", []any{}...)
		core.Debug("recursive submodules = false", []any{}...)
	case "recursive":
		core.Debug("submodules = true", []any{}...)
		core.Debug("recursive submodules = true", []any{}...)
	default:
		return fmt.Errorf("unsupported submodules: '%s', expected true/false/recursive", cfg.Submodules)
	}

	// Auth token
	if cfg.Token == "" {
		return fmt.Errorf("input required and not supplied: token")
	}

	// Workflow organization ID
	if cfg.Provider == GitHubProvider {
		raw, _ := getMapFromMap(eventContext, "raw")
		repo, _ := getMapFromMap(raw, "repository")
		owner, _ := getMapFromMap(repo, "owner")
		cfg.githubWorkflowOrganizationId, _ = getStringFromMap(owner, "id")
	}

	// Determine the provider URL that the repository is being hosted from
	switch cfg.Provider {
	case GitHubProvider:
		if cfg.GithubServerURL == "" {
			cfg.GithubServerURL = os.Getenv("GITHUB_SERVER_URL")
		}
		if cfg.GithubServerURL == "" {
			cfg.GithubServerURL = "https://github.com"
		}
		core.Debug("GitHub Host URL = %s", []any{cfg.GithubServerURL}...)
	case GitLabProvider:
		if cfg.GitlabServerURL == "" {
			cfg.GitlabServerURL = os.Getenv("GITLAB_SERVER_URL")
		}
		if cfg.GitlabServerURL == "" {
			cfg.GitlabServerURL = "https://gitlab.com"
		}
		core.Debug("GitLab Host URL = %s", []any{cfg.GitlabServerURL}...)
	case BitbucketProvider:
		if cfg.BitbucketServerURL == "" {
			cfg.BitbucketServerURL = os.Getenv("BITBUCKET_SERVER_URL")
		}
		if cfg.BitbucketServerURL == "" {
			cfg.BitbucketServerURL = "https://bitbucket.org"
		}
		core.Debug("Bitbucket Host URL = %s", []any{cfg.GitlabServerURL}...)
	}

	return nil
}

func findEventContext() map[string]interface{} {
	if eventPath, found := os.LookupEnv("CLOUDBEES_EVENT_PATH"); found {
		return safeLoadEventContext(eventPath)
	} else if homePath, found := os.LookupEnv("CLOUDBEES_HOME"); found {
		// TODO remove when CLOUDBEES_EVENT_PATH is exposed in the environment
		return safeLoadEventContext(filepath.Join(homePath, "event.json"))
	}
	return make(map[string]interface{})
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

	repositoryURL, err := cfg.fetchURL(useSSH)
	if err != nil {
		return err
	}
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

	tempHome := filepath.Join(temp, uniqueID)
	if err := os.MkdirAll(tempHome, os.ModePerm); err != nil {
		return err
	}

	fmt.Printf("Temporarily overriding HOME='%s' before making global git config changes\n", tempHome)
	cli.SetEnv("HOME", tempHome)

	if cfg.SetSafeDirectory {
		fmt.Println("Adding Repository directory to the temporary git global config as a safe directory")
		if err := cli.AddConfigStr(true, "safe.directory", workspacePath); err != nil {
			return err
		}
	}

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
		core.EndGroup()
	}

	// Disable automatic garbage collection
	core.StartGroup("Disabling automatic garbage collection")
	if err := cli.SetConfigInt(false, "gc.auto", 0); err != nil {
		fmt.Println("Unable to turn off git automatic garbage collection. The git fetch operation may trigger garbage collection and cause a delay.")
	}
	core.EndGroup()

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

	if err := auth.ConfigureToken(
		cli, "", false, cfg.serverURL(), cfg.Token); err != nil {
		return err
	}
	defer func() {
		if !cfg.PersistCredentials {
			if err := auth.RemoveToken(cli, "", false, cfg.serverURL()); err != nil && retErr == nil {
				retErr = err
			}
		}
	}()

	core.EndGroup()

	// Determine the default branch
	if cfg.Ref == "" && cfg.Commit == "" {
		core.StartGroup("Determining the default branch")
		cfg.Ref, err = cli.BranchGetDefault(repositoryURL)
		if err != nil {
			return err
		}
		core.EndGroup()
	}

	// LFS install
	if cfg.Lfs {
		if err := cli.LfsInstall(); err != nil {
			return err
		}
	}

	// Fetch the Repository
	core.StartGroup("Fetching the Repository")
	var fetchOptions git.FetchOptions
	if cfg.SparseCheckout != "" {
		fetchOptions.Filter = "blob:none"
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
	core.EndGroup()

	// Checkout info
	core.StartGroup("Determining the checkout info")
	checkoutInfo, err := getCheckoutInfo(cli, cfg.Ref, cfg.Commit)
	if err != nil {
		return err
	}
	core.EndGroup()

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
		core.EndGroup()
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
		core.EndGroup()
	}

	// Checkout
	core.StartGroup("Checking out the Ref")
	if err := cli.Checkout(checkoutInfo.ref, checkoutInfo.startPoint); err != nil {
		return err
	}
	core.EndGroup()

	// Submodules
	cfg.Submodules = strings.ToLower(strings.TrimSpace(cfg.Submodules))
	if cfg.Submodules == "true" || cfg.Submodules == "recursive" {
		// Temporarily override global config
		fmt.Println("Setting up auth for fetching submodules")

		if err := auth.ConfigureToken(cli, "", true, cfg.serverURL(), cfg.Token); err != nil {
			return err
		}

		var insteadOfKey string
		if cfg.Provider != CustomProvider {
			u, err := url.Parse(cfg.serverURL())
			if err != nil {
				return err
			}

			const insteadOfTemplate = "url.%s/.insteadOf"
			insteadOfKey = fmt.Sprintf(insteadOfTemplate, u.Scheme+"://"+u.Host)
			if _, err := cli.UnsetConfig(true, insteadOfKey); err != nil {
				return err
			}
			var insteadOfValues []string

			insteadOfValues = append(
				insteadOfValues,
				fmt.Sprintf("git@%s:", u.Host),
			)

			if cfg.Provider == GitHubProvider && cfg.githubWorkflowOrganizationId != "" {
				insteadOfValues = append(
					insteadOfValues,
					fmt.Sprintf("org-%s@%s:", cfg.githubWorkflowOrganizationId, u.Host),
				)
			}

			// Configure HTTPS instead of SSH
			if !useSSH {
				for _, v := range insteadOfValues {
					if err := cli.AddConfigStr(true, insteadOfKey, v); err != nil {
						return err
					}
				}
			}
		}
		core.EndGroup()

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
		core.EndGroup()

		if cfg.PersistCredentials {
			core.StartGroup("Persisting credentials for submodules")
			if insteadOfKey != "" {
				if _, err := cli.UnsetConfig(true, insteadOfKey); err != nil {
					return err
				}
			}
			if err := auth.ConfigureSubmoduleTokenAuth(cli, recursive, cfg.serverURL(), cfg.Token); err != nil {

			}
			core.EndGroup()
		}
	}

	// Get commit information
	var commitInfo string
	commitInfo, err = cli.Log1()
	if err != nil {
		return err
	}

	// Log commit sha
	_, err = cli.Log1("--format='%H'")
	if err != nil {
		return err
	}

	if err := cfg.checkCommitInfo(commitInfo); err != nil {
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

func (cfg *Config) checkCommitInfo(commitInfo string) error {
	if cfg.Provider != GitHubProvider {
		// this is a GitHub specific test
		return nil
	}
	eventContext := findEventContext()
	if t, ok := getStringFromMap(eventContext, "type"); !ok || t != "pull_request" {
		// check only applies to a pull request
	}
	raw, ok := getMapFromMap(eventContext, "raw")
	if !ok {
		// cannot check without the raw event, assuming ok
		return nil
	}
	repo, ok := getMapFromMap(raw, "repository")
	if !ok {
		// cannot check without the repo details from event, assuming ok
		return nil
	}
	if repoPriv, ok := getBoolFromMap(repo, "private"); !ok || repoPriv {
		// check is only valid for public PR synchronize
		return nil
	}
	if action, ok := getStringFromMap(raw, "action"); !ok || action != "synchronize" {
		// check is only valid for public PR synchronize
		return nil
	}
	if !cfg.isWorkflowRepository(eventContext) {
		// check is only valid when checking out the workflow repository
		return nil
	}
	if ref, ok := getStringFromMap(raw, "ref"); !ok || cfg.Ref != ref || !strings.HasPrefix(ref, "refs/pull/") {
		// check is only valid when checking out the workflow repository on the event ref
		return nil
	}
	if sha, ok := getStringFromMap(raw, "sha"); !ok || cfg.Commit != sha {
		// check is only valid when checking out the event sha
		return nil
	}

	// Head SHA
	expectedHeadSha, ok := getStringFromMap(raw, "after")
	if !ok || expectedHeadSha == "" {
		core.Debug("Unable to determine head sha")
		return nil
	}

	// Base SHA
	pr, _ := getMapFromMap(raw, "pull_request")
	bs, _ := getMapFromMap(pr, "base")
	expectedBaseSha, ok := getStringFromMap(bs, "sha")
	if !ok || expectedBaseSha == "" {
		core.Debug("Unable to determine base sha")
		return nil
	}

	expectedMessage := fmt.Sprintf("Merge %s into %s", expectedHeadSha, expectedBaseSha)
	if strings.Contains(commitInfo, expectedMessage) {
		// all good check is valid
		return nil
	}

	rex := regexp.MustCompile(`Merge ([0-9a-f]{40}) into ([0-9a-f]{40})`)
	match := rex.FindStringSubmatch(commitInfo)
	if match == nil {
		core.Debug("Unexpected message format")
		return nil
	}

	// Post telemetry
	actualHeadSha := match[1]
	if actualHeadSha != expectedHeadSha {
		core.Debug("Expected head sha %s; actual head sha %s", expectedHeadSha, actualHeadSha)
	}

	return nil
}

// safeLoadEventContext attempts to load the event context from the JSON file at the supplied path always returning
// a (possibly empty) map.
func safeLoadEventContext(path string) map[string]interface{} {
	c, err := loadEventContext(path)
	if err != nil {
		return make(map[string]interface{})
	}
	return c
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

// extractRefAndShaFromCloudBeesEventJson performs a best effort extraction of the SHA from event.json at the specified Path
// if anything goes wrong, we assume the sha is empty as some events may not have a sha or the event may correspond
// to a different Repository than the one we are checking out, in which case we return empty also
func (cfg *Config) extractRefAndShaFromCloudBeesEventJson(eventPath string) (string, string) {
	var bytes []byte
	var err error

	log.Printf("Reading event.json from '%s'", eventPath)
	if bytes, err = os.ReadFile(eventPath); err != nil {
		log.Printf("Could not read event.json from '%s': %v", eventPath, err)
		// this is a best effort extract, if we cannot extract it then we cannot provide defaults
		return "", ""
	}
	log.Printf("Event JSON: '%s'", string(bytes))
	var event map[string]interface{}
	if err := json.Unmarshal(bytes, &event); err != nil {
		log.Printf("Could not parse event.json from '%s': %v", eventPath, err)
		// this is a best effort extract, if we cannot extract it then we cannot provide defaults
		return "", ""
	}
	if p, found := getStringFromMap(event, "provider"); !found || p != cfg.Provider {
		// if the event doesn't tell us the Provider or tells us a different Provider then the event cannot tell us
		// anything about the Repository we are checking out so do not use the event's values for defaulting
		return "", ""
	}
	if r, found := getStringFromMap(event, "repository"); !found || r != cfg.Repository {
		// if the event doesn't tell us the Repository or tells us a different Repository then the event cannot tell us
		// anything about the Repository we are checking out so do not use the event's values for defaulting
		return "", ""
	}
	return extractRefFromCloudBeesEventJson(event), extractShaFromCloudBeesEventJson(event)
}

func (cfg *Config) isWorkflowRepository(eventContext map[string]interface{}) bool {
	ctxProvider, haveP := getStringFromMap(eventContext, "provider")
	ctxProvider = strings.ToLower(ctxProvider)

	ctxRepository, haveR := getStringFromMap(eventContext, "repository")

	return haveP && cfg.Provider == ctxProvider && haveR && cfg.Repository == ctxRepository
}

// extractShaFromCloudBeesEventJson performs a best effort extraction of the SHA from event.json at the specified Path
// if anything goes wrong, we assume the sha is empty as some events may not have a sha or the event may correspond
// to a different Repository than the one we are checking out, in which case we return empty also
func extractShaFromCloudBeesEventJson(event map[string]interface{}) string {
	s, _ := getStringFromMap(event, "sha")
	return s
}

// extractShaFromCloudBeesEventJson performs a best effort extraction of the SHA from event.json at the specified Path
// if anything goes wrong, we assume the sha is empty as some events may not have a sha or the event may correspond
// to a different Repository than the one we are checking out, in which case we return empty also
func extractRefFromCloudBeesEventJson(event map[string]interface{}) string {
	r, _ := getStringFromMap(event, "Ref")
	return r
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
				fmt.Println("The Clean command failed. This might be caused by: 1) Path too long, 2) permission issue, or 3) file in use. For further investigation, manually run 'git Clean -ffdx' on the directory '%s'.", repositoryPath)
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
