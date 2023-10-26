package helper

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGitCredential_WriteTo(t *testing.T) {
	testDate := time.Unix(987654321, 0) // Thursday 19 April 2001 04:25:21 UTC just a test date
	tests := []struct {
		name    string
		fields  GitCredential
		want    string
		wantErr bool
	}{
		{
			name:    "empty",
			fields:  GitCredential{},
			want:    "",
			wantErr: false,
		},
		{
			name: "full",
			fields: GitCredential{
				Protocol:          "https",
				Host:              "git.example.com:8443",
				Path:              "~/git/example.git",
				Username:          "git",
				Password:          "secr3t",
				PasswordExpiry:    &testDate,
				OAuthRefreshToken: "cafebabe-deadbeef",
				WwwAuth:           nil,
			},
			want: `protocol=https
host=git.example.com:8443
path=~/git/example.git
username=git
password=secr3t
password_expiry_utc=987654321
oauth_refresh_token=cafebabe-deadbeef
`,
			wantErr: false,
		},
		{
			name: "bad-protocol",
			fields: GitCredential{
				Protocol: "ht\ntps",
			},
			want:    ``,
			wantErr: true,
		},
		{
			name: "bad-host",
			fields: GitCredential{
				Host: "ht\x00tps",
			},
			want:    ``,
			wantErr: true,
		},
		{
			name: "bad-path",
			fields: GitCredential{
				Path: "ht\ntps",
			},
			want:    ``,
			wantErr: true,
		},
		{
			name: "bad-username",
			fields: GitCredential{
				Username: "ht\x00tps",
			},
			want:    ``,
			wantErr: true,
		},
		{
			name: "bad-password",
			fields: GitCredential{
				Password: "ht\ntps",
			},
			want:    ``,
			wantErr: true,
		},
		{
			name: "bad-oauth",
			fields: GitCredential{
				OAuthRefreshToken: "ht\x00tps",
			},
			want:    ``,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &GitCredential{
				Protocol:          tt.fields.Protocol,
				Host:              tt.fields.Host,
				Path:              tt.fields.Path,
				Username:          tt.fields.Username,
				Password:          tt.fields.Password,
				PasswordExpiry:    tt.fields.PasswordExpiry,
				OAuthRefreshToken: tt.fields.OAuthRefreshToken,
				WwwAuth:           tt.fields.WwwAuth,
			}
			w := &bytes.Buffer{}
			got, err := c.WriteTo(w)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.want, w.String())

			require.Equal(t, int64(len(tt.want)), got)
		})
	}
}

func TestReadCredential(t *testing.T) {
	testDate := time.Unix(987654321, 0) // Thursday 19 April 2001 04:25:21 UTC just a test date
	tests := []struct {
		name    string
		input   string
		want    *GitCredential
		wantErr bool
	}{
		{
			name:  "empty",
			input: "",
			want:  &GitCredential{},
		},
		{
			name: "full",
			input: `protocol=https
host=git.example.com:8443
path=~/git/example.git
username=git
password=secr3t
password_expiry_utc=987654321
oauth_refresh_token=cafebabe-deadbeef
`,
			want: &GitCredential{
				Protocol:          "https",
				Host:              "git.example.com:8443",
				Path:              "~/git/example.git",
				Username:          "git",
				Password:          "secr3t",
				PasswordExpiry:    &testDate,
				OAuthRefreshToken: "cafebabe-deadbeef",
				WwwAuth:           nil,
			},
		},
		{
			name: "full-url",
			input: `url=https://git.example.com:8443/~/git/example.git
username=git
password=secr3t
password_expiry_utc=987654321
oauth_refresh_token=cafebabe-deadbeef
`,
			want: &GitCredential{
				Protocol:          "https",
				Host:              "git.example.com:8443",
				Path:              "~/git/example.git",
				Username:          "git",
				Password:          "secr3t",
				PasswordExpiry:    &testDate,
				OAuthRefreshToken: "cafebabe-deadbeef",
				WwwAuth:           nil,
			},
		},
		{
			name: "full-url-no-port",
			input: `url=https://github.com
username=git
password=secr3t
password_expiry_utc=987654321
oauth_refresh_token=cafebabe-deadbeef
`,
			want: &GitCredential{
				Protocol:          "https",
				Host:              "github.com",
				Path:              "/",
				Username:          "git",
				Password:          "secr3t",
				PasswordExpiry:    &testDate,
				OAuthRefreshToken: "cafebabe-deadbeef",
				WwwAuth:           nil,
			},
		},
		{
			name: "full-wwwauth",
			input: `protocol=https
host=git.example.com:8443
path=~/git/example.git
username=git
password=secr3t
password_expiry_utc=987654321
oauth_refresh_token=cafebabe-deadbeef
wwwauth[]=foo:bar
wwwauth[]=fu:manchu
`,
			want: &GitCredential{
				Protocol:          "https",
				Host:              "git.example.com:8443",
				Path:              "~/git/example.git",
				Username:          "git",
				Password:          "secr3t",
				PasswordExpiry:    &testDate,
				OAuthRefreshToken: "cafebabe-deadbeef",
				WwwAuth: []string{
					"foo:bar",
					"fu:manchu",
				},
			},
		},
		{
			name:    "key-must-always-be-followed-by-equals",
			input:   "standalone-key",
			wantErr: true,
		},
		{
			name:    "value-must-always-be-followed-by-newline",
			input:   "key=value-no-newline",
			wantErr: true,
		},
		{
			name: "empty-multi-value-clears-previous",
			input: `wwwauth[]=eat:my
wwwauth[]=
wwwauth[]=foo:bar
`,
			want: &GitCredential{
				WwwAuth: []string{"foo:bar"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadCredential(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}
