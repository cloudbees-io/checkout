package checkout

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cloudbees-io/checkout/internal/auth"
)

const userAndHostRegex = `([a-zA-Z][-a-zA-Z0-9_]*@)?[a-z0-9][-a-z0-9_\.]*`

var sshURLRegex = regexp.MustCompile(fmt.Sprintf(`^ssh://%[1]s(:|/)(/?[\w_\-\.~]+)*$|^%[1]s:/?[\w_\-\.~]+(/?[\w_\-\.~]+)*$`, userAndHostRegex))
var sshURLRegexWithPort = regexp.MustCompile(fmt.Sprintf(`^ssh://%[1]s:[^0-9]+`, userAndHostRegex))

func isSSHURL(urlStr string) bool {
	return sshURLRegex.MatchString(urlStr)
}

func normalizeSSHURL(urlStr string) string {
	if !isSSHURL(urlStr) {
		return urlStr
	}

	if !strings.HasPrefix(urlStr, "ssh://") {
		urlStr = fmt.Sprintf("ssh://%s", urlStr)
	}

	if sshURLRegexWithPort.MatchString(urlStr) {
		lastColonPos := strings.LastIndex(urlStr, ":")
		if lastColonPos > -1 {
			urlStr = fmt.Sprintf("%s/%s", urlStr[:lastColonPos], strings.TrimLeft(urlStr[lastColonPos+1:], "/"))
		}
	}

	return urlStr
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
