package helper

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

// GitCredential represents the input/output format of the git credential helper API.
// See https://git-scm.com/docs/git-credential#IOFMT
type GitCredential struct {
	// Protocol The protocol over which the credential will be used
	Protocol string
	// Host The remote hostname for a network credential. This includes the port number if one was specified
	Host string
	// Path The path with which the credential will be used. E.g., for accessing a remote https repository, this will
	// be the repository’s path on the server.
	Path string
	// Username The credential’s username
	Username string
	// Password The credential’s password
	Password string
	// PasswordExpiry Generated passwords such as an OAuth access token may have an expiry date
	PasswordExpiry *time.Time
	// OAuthRefreshToken An OAuth refresh token may accompany a password that is an OAuth access token. Helpers must
	// treat this attribute as confidential like the password attribute. Git itself has no special behaviour for this
	// attribute.
	OAuthRefreshToken string
	// WwwAuth When an HTTP response is received by Git that includes one or more WWW-Authenticate authentication
	// headers, these will be passed by Git to credential helpers. The order of the attributes is the same as they
	// appear in the HTTP response. This attribute is one-way from Git to pass additional information to credential
	// helpers.
	WwwAuth []string
}

func (c *GitCredential) WriteTo(w io.Writer) (int64, error) {
	var n int64

	if isValidGitCredentialHelperValue(c.Protocol) {
		return n, fmt.Errorf("protocol cannot contain NUL character or newline")
	}

	if isValidGitCredentialHelperValue(c.Host) {
		return n, fmt.Errorf("host cannot contain NUL character or newline")
	}

	if isValidGitCredentialHelperValue(c.Path) {
		return n, fmt.Errorf("path cannot contain NUL character or newline")
	}

	if isValidGitCredentialHelperValue(c.Username) {
		return n, fmt.Errorf("username cannot contain NUL character or newline")
	}

	if isValidGitCredentialHelperValue(c.Password) {
		return n, fmt.Errorf("password cannot contain NUL character or newline")
	}

	if isValidGitCredentialHelperValue(c.OAuthRefreshToken) {
		return n, fmt.Errorf("oauth_refresh_token cannot contain NUL character or newline")
	}

	if c.Protocol != "" {
		i, err := io.WriteString(w, fmt.Sprintf("protocol=%s\n", c.Protocol))

		n += int64(i)

		if err != nil {
			return n, err
		}
	}

	if c.Host != "" {
		i, err := io.WriteString(w, fmt.Sprintf("host=%s\n", c.Host))

		n += int64(i)

		if err != nil {
			return n, err
		}
	}

	if c.Path != "" {
		i, err := io.WriteString(w, fmt.Sprintf("path=%s\n", c.Path))

		n += int64(i)

		if err != nil {
			return n, err
		}
	}

	if c.Username != "" {
		i, err := io.WriteString(w, fmt.Sprintf("username=%s\n", c.Username))

		n += int64(i)

		if err != nil {
			return n, err
		}
	}

	if c.Password != "" {
		i, err := io.WriteString(w, fmt.Sprintf("password=%s\n", c.Password))

		n += int64(i)

		if err != nil {
			return n, err
		}
	}

	if c.PasswordExpiry != nil {
		i, err := io.WriteString(w, fmt.Sprintf("password_expiry_utc=%d\n", c.PasswordExpiry.Unix()))

		n += int64(i)

		if err != nil {
			return n, err
		}
	}

	if c.OAuthRefreshToken != "" {
		i, err := io.WriteString(w, fmt.Sprintf("oauth_refresh_token=%s\n", c.OAuthRefreshToken))

		n += int64(i)

		if err != nil {
			return n, err
		}
	}

	// url is an alternative to protocol and host, we have parsed urls so no need to write back

	// wwwauth[] is one-way from git to the helper, so we should never write it out

	return n, nil
}

// isValidGitCredentialHelperValue verifies that a value meets the constraints of a value that can be exposed
// via the Git Credential Helper API contract, i.e. cannot contain a null or newline character
func isValidGitCredentialHelperValue(s string) bool {
	return strings.Contains(s, "\x00") || strings.Contains(s, "\n")
}

func ReadCredential(r io.Reader) (*GitCredential, error) {
	rd := bufio.NewReader(r)
	c := &GitCredential{}
	for {
		key, err := rd.ReadString('=')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if key == "" {
					return c, nil
				}
				return nil, io.ErrUnexpectedEOF
			}

			return nil, err
		}

		key = strings.TrimSuffix(key, "=")

		val, err := rd.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, io.ErrUnexpectedEOF
			}

			return nil, err
		}

		val = strings.TrimSuffix(val, "\n")

		switch key {
		case "protocol":
			c.Protocol = val
		case "host":
			c.Host = val
		case "path":
			c.Path = val
		case "username":
			c.Username = val
		case "password":
			c.Password = val
		case "password_expiry_utc":
			v, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return nil, err
			}

			t := time.Unix(v, 0)
			c.PasswordExpiry = &t
		case "oauth_refresh_token":
			c.OAuthRefreshToken = val
		case "url":
			ep, err := transport.NewEndpoint(val)
			if err != nil {
				return nil, err
			}

			c.Protocol = ep.Protocol

			if ep.Port == 0 {
				c.Host = ep.Host
			} else {
				c.Host = fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			}

			c.Path = strings.TrimPrefix(ep.Path, "/")
			if c.Path == "" {
				c.Path = "/"
			}
		case "wwwauth[]":
			if val == "" {
				c.WwwAuth = nil
			} else {
				c.WwwAuth = append(c.WwwAuth, val)
			}
		}
	}
}
