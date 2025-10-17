package cmd

import (
	"context"
	"os"
	"os/signal"
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
)

func Execute() error {
	return cmd.Execute()
}

func init() {
	cmd.Flags().StringVar(&cfg.Repository, "repository", "", "Repository name with owner")
	cmd.Flags().StringVar(&cfg.Ref, "ref", "", "The branch, tag or SHA to checkout")
	cmd.Flags().StringVar(&cfg.CloudBeesApiToken, "cloudbees-api-token", "", "CloudBees API token used to fetch authentication")
	cmd.Flags().StringVar(&cfg.CloudBeesApiURL, "cloudbees-api-url", "", "CloudBees API root URL to fetch authentication from")
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
	ctx := cliContext()
	return cfg.Run(ctx)
}
