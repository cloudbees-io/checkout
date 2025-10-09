package checkout

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/cloudbees-io/checkout/internal/auth"
)

var sshURLRegex = regexp.MustCompile(`^(ssh://)?([a-zA-Z][-a-zA-Z0-9_]*@)?[a-z0-9][-a-z0-9_\.]*:(/?[\w_\-\.~]+)*$`)

func isSSHURL(urlStr string) bool {
	return sshURLRegex.MatchString(urlStr)
}

func (cfg *Config) serverURL() string {
	p := cfg.Provider
	switch p {
	case auth.GitHubProvider:
		return cfg.GithubServerURL
	case auth.BitbucketProvider:
		return cfg.BitbucketServerURL
	case auth.BitbucketDatacenterProvider:
		return cfg.BitbucketServerURL
	case auth.GitLabProvider:
		return cfg.GitlabServerURL
	default:
		return ""
	}
}

// fetchURL returns the URL to use to clone the repository
func (cfg *Config) fetchURL(ssh bool) (string, error) {
	p := cfg.Provider
	switch p {
	case auth.GitHubProvider:
		return cfg.githubCloneUrl(ssh)
	case auth.BitbucketProvider:
		return cfg.bitbucketCloneUrl(ssh)
	case auth.BitbucketDatacenterProvider:
		return cfg.bitbucketCloneUrl(ssh)
	case auth.GitLabProvider:
		return cfg.gitlabCloneUrl(ssh)
	case auth.CustomProvider:
		return cfg.Repository, nil
	default:
		if v, ok := getStringFromMap(cfg.eventContext, "repositoryUrl"); ok {
			return v, nil
		}
		return "", fmt.Errorf("clone url not found for provider: %s", p)
	}
}

func (cfg *Config) githubCloneUrl(ssh bool) (string, error) {
	parsed, err := url.Parse(cfg.GithubServerURL)
	if err != nil {
		return "", err
	}
	clone := parsed.JoinPath(cfg.Repository + ".git")
	if !ssh {
		return clone.String(), nil
	}
	return "git@" + clone.Hostname() + ":" + clone.Path, nil
}

func (cfg *Config) bitbucketCloneUrl(ssh bool) (string, error) {
	parsed, err := url.Parse(cfg.BitbucketServerURL)
	if err != nil {
		return "", err
	}
	clone := parsed.JoinPath(cfg.Repository + ".git")
	if !ssh {
		return clone.String(), nil
	}
	return "git@" + clone.Hostname() + ":" + clone.Path, nil

}

func (cfg *Config) gitlabCloneUrl(ssh bool) (string, error) {
	parsed, err := url.Parse(cfg.GitlabServerURL)
	if err != nil {
		return "", err
	}
	clone := parsed.JoinPath(cfg.Repository + ".git")
	if !ssh {
		return clone.String(), nil
	}
	return "git@" + clone.Hostname() + ":" + clone.Path, nil
}
