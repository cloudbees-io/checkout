package checkout

import (
	"fmt"
	"regexp"
	"strings"
)

const userAndHostRegex = `([a-zA-Z][-a-zA-Z0-9_]*@)?[a-z0-9][-a-z0-9_\.]*`

// Matches SSH URLs in the format: ssh://user@host[:port]/path
var sshURLRegexScheme = regexp.MustCompile(fmt.Sprintf(`^ssh://%s(:|/)(/?[\w_\-\.~]+)*$`, userAndHostRegex))

// Matches SSH URLs in the format: user@host:path
var sshURLRegexScp = regexp.MustCompile(fmt.Sprintf(`^%s:/?[\w_\-\.~]+(/?[\w_\-\.~]+)*$`, userAndHostRegex))
var sshURLRegexWithPort = regexp.MustCompile(fmt.Sprintf(`^ssh://%s:[^0-9]+`, userAndHostRegex))

func isSSHURL(urlStr string) bool {
	return sshURLRegexScheme.MatchString(urlStr) || sshURLRegexScp.MatchString(urlStr)
}

func normalizeSSHURL(urlStr string) string {
	if !isSSHURL(urlStr) {
		return urlStr
	}

	if !strings.HasPrefix(urlStr, "ssh://") {
		urlStr = fmt.Sprintf("ssh://%s", urlStr)
	}

	if sshURLRegexWithPort.MatchString(urlStr) {
		// Convert 'ssh://git@host:path' to 'ssh://git@host/path' format for normalization.
		// This is since `git ls-remote --symref URL` doesn't accept the URL otherwise.
		lastColonPos := strings.LastIndex(urlStr, ":")
		if lastColonPos > -1 {
			urlStr = fmt.Sprintf("%s/%s", urlStr[:lastColonPos], strings.TrimLeft(urlStr[lastColonPos+1:], "/"))
		}
	}

	return urlStr
}
