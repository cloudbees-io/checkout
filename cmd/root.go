package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/cloudbees-io/checkout/internal/checkout"
	"github.com/spf13/cobra"
)

var (
	cmd = &cobra.Command{
		Use:          "checkout",
		Short:        "Implements the actions/checkout",
		Long:         "Implements the actions/checkout",
		SilenceUsage: true,
		RunE:         doCheckout,
	}
	cfg checkout.Config

	// Ensure warning is printed only once
	warningOnce sync.Once
)

func Execute() error {
	return cmd.Execute()
}

func init() {
	cmd.Flags().StringVar(&cfg.Provider, "provider", "", "SCM provider that is hosting the repository")
	cmd.Flags().StringVar(&cfg.Repository, "repository", "", "Repository name with owner")
	cmd.Flags().StringVar(&cfg.Ref, "ref", "", "The branch, tag or SHA to checkout")
	cmd.Flags().StringVar(&cfg.CloudBeesApiToken, "cloudbees-api-token", "", "CloudBees API token used to fetch authentication")
	cmd.Flags().StringVar(&cfg.CloudBeesApiURL, "cloudbees-api-url", "", "CloudBees API root URL to fetch authentication from")
	cmd.Flags().StringVar(&cfg.Token, "token", "", "Personal access token (PAT) used to fetch the repository")
	cmd.Flags().StringVar(&cfg.TokenAuthtype, "token-auth-type", "", "Auth type of the token, one of `basic` or `bearer`")
	cmd.Flags().StringVar(&cfg.SSHKey, "ssh-key", "", "SSH key used to fetch the repository")
	cmd.Flags().StringVar(&cfg.SSHKnownHosts, "ssh-known-hosts", "", "Known hosts in addition to the user and global host key database")
	cmd.Flags().BoolVar(&cfg.SSHStrict, "ssh-strict", true, "Whether to perform strict host key checking")
	cmd.Flags().BoolVar(&cfg.PersistCredentials, "persist-credentials", true, "Whether to configure the token or SSH key with the local git config")
	cmd.Flags().StringVar(&cfg.Path, "path", "", "Relative path under $CLOUDBEES_WORKSPACE to place the repository")
	cmd.Flags().BoolVar(&cfg.Clean, "clean", false, "Whether to execute git clean -ffdx && git reset --hard HEAD before fetching")
	cmd.Flags().StringVar(&cfg.SparseCheckout, "sparse-checkout", "", "Do a sparse checkout on given patterns. Each pattern should be separated with new lines")
	cmd.Flags().BoolVar(&cfg.SparseCheckoutConeMode, "sparse-checkout-cone-mode", false, "Specifies whether to use cone-mode when doing a sparse checkout.")
	cmd.Flags().IntVar(&cfg.FetchDepth, "fetch-depth", 1, "Number of commits to fetch")
	cmd.Flags().BoolVar(&cfg.Lfs, "lfs", false, "Whether to download Git-LFS files")
	cmd.Flags().StringVar(&cfg.Submodules, "submodules", "false", "Whether to checkout submodules, one of `true`, `false`, or `recursive`")
	cmd.Flags().BoolVar(&cfg.SetSafeDirectory, "set-safe-directory", true, "Add repository path as safe.directory for Git global config")
	cmd.Flags().StringVar(&cfg.GithubServerURL, "github-server-url", "", "The base URL for the GitHub instance that you are trying to clone from")
	cmd.Flags().StringVar(&cfg.BitbucketServerURL, "bitbucket-server-url", "", "The base URL for the Bitbucket instance that you are trying to clone from")
	cmd.Flags().StringVar(&cfg.GitlabServerURL, "gitlab-server-url", "", "The base URL for the GitLab instance that you are trying to clone from")

	cmd.AddCommand(helperCmd)
}

func cliContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel() // exit gracefully
		<-c
		os.Exit(1) // exit immediately on 2nd signal
	}()
	return ctx
}

func doCheckout(command *cobra.Command, args []string) error {
	// Display v1 deprecation warning (only once)
	warningOnce.Do(printV1DeprecationWarning)

	ctx := cliContext()
	return cfg.Run(ctx)
}

func printV1DeprecationWarning() {
	warningMessage := `
==================================================================================================
                       				 ⚠️ DEPRECATION WARNING ⚠️
==================================================================================================

You are using the DEPRECATED v1 version of the checkout action.

Version v1 is no longer maintained and will be removed in the future.

⚡ Please migrate to v2 as soon as possible.

📖 Migration Guide:
https://docs.cloudbees.com/docs/cloudbees-platform/latest/source-code-management/migrate-v1-to-v2

Update your workflow file:
  Change: uses: cloudbees-io/checkout@v1
  To:     uses: cloudbees-io/checkout@v2

==================================================================================================
`
	fmt.Fprint(os.Stderr, warningMessage)
}
