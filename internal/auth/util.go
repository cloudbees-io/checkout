package auth

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"github.com/cloudbees-io/checkout/internal/core"
	"github.com/cloudbees-io/checkout/internal/git"
	"github.com/cloudbees-io/checkout/internal/helper"

	"al.essio.dev/pkg/shellescape"
)

const (
	tokenPlaceholderConfigValue = "Authorization: Basic ***"
	tokenConfigValue            = "Authorization: Basic %s"
	tokenConfigKey              = "http.%s/.extraheader"
	authTemplate                = "x-access-token:%s"

	GitHubProvider              = "github"
	GitLabProvider              = "gitlab"
	BitbucketProvider           = "bitbucket"
	CustomProvider              = "custom"
	BitbucketDatacenterProvider = "bitbucket_datacenter"
)

//go:embed ssh_known_hosts.tmpl
var sshKnownHostsTemplate string

func noOpClean() error {
	return nil
}

type TokenAuth struct {
	Provider      string
	ScmToken      string
	TokenAuthType string
	ApiURL        string
	ApiToken      string
}

func (a *TokenAuth) providerUsername() string {
	switch a.Provider {
	case GitHubProvider:
		// GHA checkout action uses this username
		return "x-access-token"
	case GitLabProvider:
		// https://docs.gitlab.com/ee/user/project/settings/project_access_tokens.html
		// Any non-blank value as a username
		return "x-access-token"
	case BitbucketProvider:
		// this is what they suggest when you go through https://bitbucket.org/{org}/{repo}/admin/access-tokens
		return "x-token-auth"
	case CustomProvider:
		return "x-access-token"
	default:
		return "git"
	}
}

func (a *TokenAuth) options() map[string][]string {
	options := make(map[string][]string)
	options["username"] = []string{a.providerUsername()}
	if a.ScmToken != "" {
		encodedToken := base64.StdEncoding.EncodeToString([]byte(a.ScmToken))
		switch strings.ToLower(a.TokenAuthType) {
		case "basic":
			options["password"] = []string{encodedToken}
		case "bearer":
			// https://github.com/cloudbees-io/checkout/blob/9986681fa9b267f4282399c4e55806af0ba97aa6/internal/helper/credential.go#L42
			options["credential"] = []string{encodedToken}
		default:
			options["password"] = []string{encodedToken}
		}
	} else if a.ApiToken != "" && a.ApiURL != "" {
		options["cloudBeesApiUrl"] = []string{a.ApiURL}
		options["cloudBeesApiToken"] = []string{base64.StdEncoding.EncodeToString([]byte(a.ApiToken))}
	}
	return options
}

func credentialHelperWithUserProvidedCreds(cli *git.GitCLI, globalConfig bool, serverURL string, token TokenAuth) (func() error, string, error) {
	helperCommand, cleaner, err := helper.InstallHelperFor(serverURL, token.options())
	if err != nil {
		return cleaner, "", err
	}

	oldHelper, _ := cli.GetConfig(globalConfig, "credential.helper")
	oldUseHttpPath, _ := cli.GetConfig(globalConfig, "credential.useHttpPath")

	fullCleaner := func() error {
		var errs []error
		if oldHelper == "" {
			if _, err := cli.UnsetConfig(globalConfig, "credential.helper"); err != nil {
				errs = append(errs, err)
			}
		} else if err := cli.SetConfigStr(globalConfig, "credential.helper", oldHelper); err != nil {
			errs = append(errs, err)
		}
		useHttpPath, _ := strconv.ParseBool(oldUseHttpPath)
		if oldUseHttpPath == "" {
			if _, err := cli.UnsetConfig(globalConfig, "credential.useHttpPath"); err != nil {
				errs = append(errs, err)
			}
		} else if err := cli.SetConfigBool(globalConfig, "credential.useHttpPath", useHttpPath); err != nil {
			errs = append(errs, err)
		}
		if err := cleaner(); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	}

	if err := cli.SetConfigStr(globalConfig, "credential.helper", helperCommand); err != nil {
		return fullCleaner, "", err
	}
	if err := cli.SetConfigBool(globalConfig, "credential.useHttpPath", true); err != nil {
		return fullCleaner, "", err
	}

	return fullCleaner, helperCommand, nil
}

func ConfigureToken(cli *git.GitCLI, configPath string, globalConfig bool, serverURL string, token TokenAuth) (func() error, string, error) {
	if configPath != "" && globalConfig {
		return noOpClean, "", fmt.Errorf("unexpected ConfigureToken parameter combination")
	}

	if configPath == "" && !globalConfig {
		configPath = filepath.Join(cli.Cwd(), ".git", "config")
	}

	var err error
	if globalConfig {
		if configPath, err = cli.GlobalConfigPath(); err != nil {
			return noOpClean, "", err
		}
	}

	path, err := exec.LookPath("git-credential-cloudbees")
	if err != nil || len(token.ScmToken) > 0 { // if token.ScmToken is set, that means the user wants to use his own token, so we fallback to the old style helper
		if err != nil {
			core.Debug("Could not find git-credential-cloudbees on the path, falling back to old-style helper")
		} else {
			core.Debug("Using user-provided token, falling back to old-style helper")
		}

		return credentialHelperWithUserProvidedCreds(cli, globalConfig, serverURL, token)
	}

	core.Debug("Found git-credential-cloudbees on the path at %s", path)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return noOpClean, "", err
	}

	helperConfig := filepath.Join(homeDir, ".git-credential-cloudbees-config")

	const tokenEnv = "CLOUDBEES_API_TOKEN"

	cmd := exec.Command(path,
		"init",
		"--config", helperConfig,
		"--cloudbees-api-token-env-var", tokenEnv,
		"--cloudbees-api-url", token.ApiURL,
		"--git-config-file-path", configPath)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println(cmd.String())

	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", tokenEnv, token.ApiToken))

	err = cmd.Run()
	if err != nil {
		return noOpClean, "", err
	}

	return noOpClean, fmt.Sprintf("%s helper --config %s", shellescape.Quote(path), shellescape.Quote(helperConfig)), nil
}

func ConfigureSubmoduleTokenAuth(cli *git.GitCLI, recursive bool, serverURL string, token string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	var submoduleOutput string
	_, err = exec.LookPath("git-credential-cloudbees")
	if err != nil {
		core.Debug("Could not find git-credential-cloudbees on the path, using token placeholder replacement for submodules")
		key := fmt.Sprintf(tokenConfigKey, u.Scheme+"://"+u.Host)

		submoduleOutput, err = cli.SubmoduleForeach(recursive, "sh", "-c",
			fmt.Sprintf(`%s config --local '%s' '%s' && %s config --local --show-origin --name-only --get-regexp remote.origin.url`,
				cli.Executable(), key, tokenPlaceholderConfigValue, cli.Executable()))
		if err != nil {
			return err
		}
	}

	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(authTemplate, token)))

	configPathRegex := regexp.MustCompile(`^file:([^\t]+)\tremote\.origin\.url$`)

	for _, line := range strings.Split(submoduleOutput, "\n") {
		matches := configPathRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		configPath := matches[1]
		if configPath != "" {
			if !filepath.IsAbs(configPath) {
				configPath = filepath.Join(cli.Cwd(), configPath)
			}
			if err := replaceTokenPlaceholder(configPath, auth); err != nil {
				return err
			}
		}
	}

	return nil
}

func replaceTokenPlaceholder(configPath string, token string) error {
	stat, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("could not find file '%s': %v", configPath, err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	placeholdeReplacement := fmt.Sprintf(tokenConfigValue, token)

	content = bytes.ReplaceAll(content, []byte(tokenPlaceholderConfigValue), []byte(placeholdeReplacement))

	return os.WriteFile(configPath, content, stat.Mode())
}

func GenerateSSHKey(ctx context.Context, tempDir string, prefix string, inputKey string) (string, error) {
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return "", err
	}
	keyPath := filepath.Join(tempDir, prefix+"_key")
	if err := os.WriteFile(keyPath, []byte(inputKey), 0600); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		// remove inherited permissions on windows
		icacls, err := exec.LookPath("icacls.exe")
		if err != nil && !errors.Is(err, exec.ErrDot) {
			return "", fmt.Errorf("cannot find icacls.exe: %v", err)
		} else if errors.Is(err, exec.ErrDot) {
			if icacls, err = filepath.Abs(icacls); err != nil {
				return "", fmt.Errorf("cannot find icacls.exe: %v", err)
			}
		}

		c := exec.CommandContext(ctx, icacls, keyPath, "/grant:r", os.Getenv("USERDOMAIN")+"\\"+os.Getenv("USERNAME")+":F")
		c.Dir = os.Getenv(tempDir)

		if err = c.Start(); err != nil {
			return "", err
		}

		if err = c.Wait(); err != nil {
			return "", err
		}

		c = exec.CommandContext(ctx, icacls, keyPath, "/inheritance:r")
		c.Dir = os.Getenv(tempDir)

		if err = c.Start(); err != nil {
			return "", err
		}

		if err = c.Wait(); err != nil {
			return "", err
		}
	}

	return keyPath, nil
}

func GenerateSSHCommand(sshKeyPath string, sshStrict bool, sshKnownHostsPath string) (string, error) {
	ssh, err := exec.LookPath("ssh")
	if err != nil && !errors.Is(err, exec.ErrDot) {
		return "", fmt.Errorf("cannot find ssh: %v", err)
	} else if errors.Is(err, exec.ErrDot) {
		if ssh, err = filepath.Abs(ssh); err != nil {
			return "", fmt.Errorf("cannot find ssh: %v", err)
		}
	}
	cmd := fmt.Sprintf("%s -i %s", shellescape.Quote(ssh), shellescape.Quote(sshKeyPath))
	if sshStrict {
		cmd = cmd + " -o StrictHostKeyChecking=yes -o CheckHostIP=no"
	}
	cmd = cmd + " -o UserKnownHostsFile=$RUNNER_TEMP/" + filepath.Base(sshKnownHostsPath)
	return cmd, nil
}

func GenerateSSHKnownHosts(home string, tempDir string, prefix string, inputKnownHosts string) (_ string, retErr error) {
	tmpl := template.New("ssh_known_hosts")
	tmpl, err := tmpl.Parse(sshKnownHostsTemplate)
	if err != nil {
		return "", err
	}

	userKnownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	userKnownHosts := ""
	if info, err := os.Stat(userKnownHostsPath); err == nil && !info.IsDir() {
		if bytes, err := os.ReadFile(userKnownHostsPath); err == nil {
			userKnownHosts = string(bytes)
		}
		// if we couldn't read it, treat it as an empty file
	}

	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return "", err
	}
	knownHostsPath := filepath.Join(tempDir, prefix+"_known_hosts")
	f, err := os.Create(knownHostsPath)
	if err != nil {
		return "", err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil && retErr == nil {
			retErr = err
		}
	}(f)

	err = tmpl.Execute(f, struct {
		UserKnownHosts     string
		UserKnownHostsPath string
		SSHKnownHosts      string
	}{
		UserKnownHosts:     userKnownHosts,
		UserKnownHostsPath: userKnownHostsPath,
		SSHKnownHosts:      inputKnownHosts,
	})
	return knownHostsPath, err
}
