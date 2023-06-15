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
	"strings"
	"text/template"

	"github.com/cloudbees-io/actions-checkout/internal/git"
	"gopkg.in/alessio/shellescape.v1"
)

const (
	tokenPlaceholderConfigValue = "Authorization: Basic ***"
	tokenConfigValue            = "Authorization: Basic %s"
	tokenConfigKey              = "http.%s/.extraheader"
	authTemplate                = "x-access-token:%s"
)

//go:embed ssh_known_hosts.tmpl
var sshKnownHostsTemplate string

func ConfigureToken(cli *git.GitCLI, configPath string, globalConfig bool, serverURL string, token string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	if configPath != "" && globalConfig {
		return fmt.Errorf("unexpected ConfigureToken parameter combination")
	}

	if configPath == "" && !globalConfig {
		configPath = filepath.Join(cli.Cwd(), ".git", "config")
	}

	if globalConfig {
		if configPath, err = cli.GlobalConfigPath(); err != nil {
			return err
		}
	}

	key := fmt.Sprintf(tokenConfigKey, u.Scheme+"://"+u.Host)
	if err := cli.SetConfigStr(globalConfig, key, tokenPlaceholderConfigValue); err != nil {
		return err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(authTemplate, token)))
	return replaceTokenPlaceholder(configPath, auth)
}

func RemoveToken(cli *git.GitCLI, configPath string, globalConfig bool, serverURL string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	if configPath != "" && globalConfig {
		return fmt.Errorf("unexpected ConfigureToken parameter combination")
	}

	if configPath == "" && !globalConfig {
		configPath = filepath.Join(cli.Cwd(), ".git", "config")
	}

	if globalConfig {
		if configPath, err = cli.GlobalConfigPath(); err != nil {
			return err
		}
	}

	_, err = cli.UnsetConfig(globalConfig, fmt.Sprintf(tokenConfigKey, u.Scheme+"://"+u.Host))

	return err
}

func ConfigureSubmoduleTokenAuth(cli *git.GitCLI, recursive bool, serverURL string, token string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	key := fmt.Sprintf(tokenConfigKey, u.Scheme+"://"+u.Host)

	output, err := cli.SubmoduleForeach(recursive, "sh", "-c", fmt.Sprintf(`%s config --local '%s' '%s' && %s config --local --show-origin --name-only --get-regexp remote.origin.url`, cli.Executable(), key, tokenPlaceholderConfigValue, cli.Executable()))
	if err != nil {
		return err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(authTemplate, token)))

	configPathRegex := regexp.MustCompile(`^file:([^\t]+)\tremote\.origin\.url$`)

	for _, line := range strings.Split(output, "\n") {
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
			return "", fmt.Errorf("cannot find icacls.exe", err)
		} else if errors.Is(err, exec.ErrDot) {
			if icacls, err = filepath.Abs(icacls); err != nil {
				return "", fmt.Errorf("cannot find icacls.exe", err)
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
		return "", fmt.Errorf("cannot find ssh", err)
	} else if errors.Is(err, exec.ErrDot) {
		if ssh, err = filepath.Abs(ssh); err != nil {
			return "", fmt.Errorf("cannot find ssh", err)
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
