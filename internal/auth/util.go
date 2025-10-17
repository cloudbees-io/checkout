package auth

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"

	"github.com/cloudbees-io/checkout/internal/core"
	"github.com/cloudbees-io/checkout/internal/git"

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
	ApiURL   string
	ApiToken string
}

func ConfigureToken(cli *git.GitCLI, configPath string, globalConfig bool, token TokenAuth) (func() error, string, error) {
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
	if err != nil {
		return noOpClean, "", err
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
