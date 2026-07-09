package tools

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
)

// makeJWT builds an unsigned JWT whose payload carries the given claims.
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
}

func TestDecodeJWTEmail(t *testing.T) {
	cases := []struct {
		name, token, want string
	}{
		{"email claim", makeJWT(t, map[string]any{"email": "alice@work.com"}), "alice@work.com"},
		{"nested openai profile claim", makeJWT(t, map[string]any{
			"https://api.openai.com/profile": map[string]any{"email": "bob@work.com"},
		}), "bob@work.com"},
		{"top-level email wins over nested profile claim", makeJWT(t, map[string]any{
			"email":                          "top@work.com",
			"https://api.openai.com/profile": map[string]any{"email": "nested@work.com"},
		}), "top@work.com"},
		{"no email claim", makeJWT(t, map[string]any{"sub": "123"}), ""},
		{"empty", "", ""},
		{"one segment", "abc", ""},
		{"bad base64", "aa.!!!.bb", ""},
		{"payload not json", "aa." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".bb", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decodeJWTEmail(c.token); got != c.want {
				t.Errorf("decodeJWTEmail = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCodexAccountFromIDToken(t *testing.T) {
	home := sandboxHome(t)
	idToken := makeJWT(t, map[string]any{"email": "alice@work.com"})
	writeFile(t, filepath.Join(home, ".codex", "auth.json"),
		`{"auth_mode":"chatgpt","tokens":{"id_token":"`+idToken+`","account_id":"acc_1"}}`)

	info, _ := Find("codex").Describe()
	if info.Account != "alice@work.com" {
		t.Errorf("account = %q, want alice@work.com", info.Account)
	}
}

func TestCodexAccountFallsBackToAccountID(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".codex", "auth.json"),
		`{"auth_mode":"chatgpt","tokens":{"account_id":"acc_1"}}`)

	info, _ := Find("codex").Describe()
	if info.Account != "acc_1" {
		t.Errorf("account = %q, want acc_1", info.Account)
	}
}

func TestClaudeAccountFromClaudeJSON(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{}`)
	writeFile(t, filepath.Join(home, ".claude.json"),
		`{"oauthAccount":{"emailAddress":"bob@personal.com"}}`)

	info, _ := Find("claude").Describe()
	if info.Account != "bob@personal.com" {
		t.Errorf("account = %q, want bob@personal.com", info.Account)
	}
}

func TestClaudeAccountAbsent(t *testing.T) {
	home := sandboxHome(t)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{}`)

	info, _ := Find("claude").Describe()
	if info.Account != "" {
		t.Errorf("account = %q, want empty", info.Account)
	}
}
