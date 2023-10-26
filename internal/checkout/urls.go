package checkout

import (
	"fmt"
	"net/url"
)

func (cfg *Config) serverURL() string {
	p := cfg.Provider
	switch p {
	case GitHubProvider:
		return cfg.GithubServerURL
	case BitbucketProvider:
		return cfg.BitbucketServerURL
	case GitLabProvider:
		return cfg.GitlabServerURL
	default:
		return ""
	}
}

// fetchURL returns the URL to use to clone the repository
func (cfg *Config) fetchURL(ssh bool) (string, error) {
	p := cfg.Provider
	switch p {
	case GitHubProvider:
		return cfg.githubCloneUrl(ssh)
	case BitbucketProvider:
		return cfg.bitbucketCloneUrl(ssh)
	case GitLabProvider:
		return cfg.gitlabCloneUrl(ssh)
	case CustomProvider:
		return cfg.Repository, nil
	default:
		return "", fmt.Errorf("unknown/unsupported SCM Provider: %s", p)
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
