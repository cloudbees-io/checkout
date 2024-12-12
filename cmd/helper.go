package cmd

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudbees-io/checkout/internal/helper"
	format "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/cobra"
)

var (
	helperCmd = &cobra.Command{
		Use:                "credential-helper",
		Short:              "Implements the Git Credentials Helper API",
		Long:               "Implements the Git Credentials Helper API",
		SilenceUsage:       true,
		DisableSuggestions: true,
		Run: func(cmd *cobra.Command, args []string) {
			// From the https://git-scm.com/docs/gitcredentials specification
			//
			// If a helper receives any other operation, it should silently ignore the request.
			// This leaves room for future operations to be added (older helpers will just
			// ignore the new requests).
		},
	}
	eraseCmd = &cobra.Command{
		Use:          "erase",
		Short:        "Remove a matching credential, if any, from the helper’s storage",
		Long:         "Remove a matching credential, if any, from the helper’s storage",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			// we do not support the erase operation
		},
	}
	storeCmd = &cobra.Command{
		Use:          "store",
		Short:        "Store the credential, if applicable to the helper",
		Long:         "Store the credential, if applicable to the helper",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			// we do not support the store operation
		},
	}
	getCmd = &cobra.Command{
		Use:          "get",
		Short:        "Return matching credential, if any exists",
		Long:         "Return matching credential, if any exists",
		SilenceUsage: true,
		RunE:         doGet,
	}

	helperConfigFile string
)

func init() {
	helperCmd.AddCommand(getCmd, eraseCmd, storeCmd)
	helperCmd.PersistentFlags().StringVarP(&helperConfigFile, "config-file", "c", "", "path to the helper configuration file to use")
}

func doGet(command *cobra.Command, args []string) error {
	_ = cliContext()

	if helperConfigFile == "" {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot infer config file from executable name: %w", err)
		}
		helperConfigFile = self + ".cfg"
	}

	bs, err := os.ReadFile(helperConfigFile)
	if err != nil {
		return fmt.Errorf("could not read configuration from %s: %w", helperConfigFile, err)
	}

	cfg := format.Config{}

	d := format.NewDecoder(bytes.NewReader(bs))

	if err := d.Decode(&cfg); err != nil {
		return fmt.Errorf("could not parse configuration file %s: %w", helperConfigFile, err)
	}

	r := bufio.NewReader(os.Stdin)

	req, err := helper.ReadCredential(r)
	if err != nil {
		return fmt.Errorf("could not read credentials request: %w", err)
	}

	section := cfg.Section(req.Protocol)

	target := (&transport.Endpoint{
		Host: req.Host,
		Path: req.Path,
	}).String()

	var closest *format.Subsection
	for _, ss := range section.Subsections {
		if strings.HasPrefix(target, ss.Name) {
			if closest == nil || (len(closest.Name) < len(ss.Name)) {
				closest = ss
			}
		}
	}

	if closest == nil {
		// nothing to contribute
		return nil
	}

	rsp := &helper.GitCredential{}

	if closest.HasOption("username") {
		rsp.Username = closest.Option("username")
	}

	if closest.HasOption("password") {
		if b, err := base64.StdEncoding.DecodeString(closest.Option("password")); err == nil {
			rsp.Password = string(b)
		} else {
			return err
		}
	}

	if closest.HasOption("cloudBeesApiToken") && closest.HasOption("cloudBeesApiUrl") {
		var token string
		if b, err := base64.StdEncoding.DecodeString(closest.Option("cloudBeesApiToken")); err == nil {
			token = string(b)
		} else {
			return err
		}

		baseURL := closest.Option("cloudBeesApiUrl")

		var resourceId string
		if resourceId, err = getResourceIdFromAutomationToken(token); err != nil {
			return err
		}

		if o := os.Getenv("RESOURCE_ID_OVERRIDE"); o != "" {
			resourceId = o
		}

		body := map[string]string{
			"scmRepoUrl": (&transport.Endpoint{
				Protocol: req.Protocol,
				Host:     req.Host,
				Path:     req.Path,
			}).String(),
		}

		var bodyBytes []byte
		if bodyBytes, err = json.Marshal(&body); err != nil {
			return err
		}

		var reqURL string
		if reqURL, err = url.JoinPath(baseURL, "reserved/v1/resources", resourceId, "scm-access-token"); err != nil {
			return err
		}

		client := &http.Client{}

		var apiReq *http.Request
		if apiReq, err = http.NewRequest(
			"POST",
			reqURL,
			bytes.NewReader(bodyBytes),
		); err != nil {
			return err
		}

		apiReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		apiReq.Header.Set("Content-Type", "application/json")
		apiReq.Header.Set("Accept", "application/json")

		var res *http.Response
		if res, err = client.Do(apiReq); err != nil {
			return err
		}

		defer func() { _ = res.Body.Close() }()

		if bodyBytes, err = io.ReadAll(res.Body); err != nil {
			return err
		}

		if res.StatusCode != 200 {
			return fmt.Errorf("could not fetch SCM token: \nPOST %s\nHTTP/%d %s\n%s", reqURL, res.StatusCode, res.Status, string(bodyBytes))
		}

		if err = json.Unmarshal(bodyBytes, &body); err != nil {
			return err
		}

		if v, ok := body["tokenType"]; ok && v == "TOKEN_TYPE_BEARER" {
			rsp.AuthType = "Bearer"
			rsp.Credential = body["accessToken"]
		} else {
			rsp.AuthType = "Basic"
			rsp.Password = body["accessToken"]
		}

		if expires, ok := body["expiresAt"]; ok && expires != "" {
			// we need to parse the time but without pulling in all the swagger deps
			re := regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2}).*`)
			if matches := re.FindStringSubmatch(expires); matches != nil {
				// we already confirmed that each submatch is a number so Atoi cannot error out
				year, _ := strconv.Atoi(matches[1])
				month, _ := strconv.Atoi(matches[2])
				day, _ := strconv.Atoi(matches[3])
				hour, _ := strconv.Atoi(matches[4])
				minute, _ := strconv.Atoi(matches[5])
				sec, _ := strconv.Atoi(matches[6])
				expiresAt := time.Date(year, time.Month(month), day, hour, minute, sec, 0, time.UTC)
				rsp.PasswordExpiry = &expiresAt
			}
		}
	}

	w := bufio.NewWriter(os.Stdout)

	if _, err = rsp.WriteTo(w); err != nil {
		return err
	}

	return w.Flush()
}

func getResourceIdFromAutomationToken(token string) (string, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return "", err
	}

	automationRaw, ok := claims["https://www.cloudbees.com/automation"]
	if !ok {
		return "", fmt.Errorf("cloudbees api token is not an automation token: missing discriminator claim")
	}

	automation, ok := automationRaw.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("cloudbees api token is not an automation token: discriminator claim is not a map")
	}

	identityRaw, ok := automation["identity"]
	if !ok {
		return "", fmt.Errorf("cloudbees api token is not an automation token: missing identity sub-claim")
	}

	identity, ok := identityRaw.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("cloudbees api token is not an automation token: identity sub-claim is not a map")
	}

	resourceIdRaw, ok := identity["resource_id"]
	if !ok {
		return "", fmt.Errorf("cloudbees api token is not an automation token: missing resource_id claim")
	}

	resourceId, ok := resourceIdRaw.(string)
	if !ok {
		return "", fmt.Errorf("cloudbees api token is not an automation token: resource_id claim is not a string")
	}
	return resourceId, nil
}
