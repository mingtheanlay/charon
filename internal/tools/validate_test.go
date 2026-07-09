package tools

import "testing"

func TestValidateEndpoint(t *testing.T) {
	valid := []string{
		"",   // blank accepts the tool's default
		"  ", // whitespace-only is trimmed to blank
		"https://api.example.com/v1",
		"http://localhost:8080",
		"https://api.example.com:8443/v1/",
	}
	for _, ep := range valid {
		if err := ValidateEndpoint(ep); err != nil {
			t.Errorf("ValidateEndpoint(%q) = %v, want nil", ep, err)
		}
	}

	invalid := []string{
		"not a url",
		"ftp://api.example.com", // wrong scheme
		"api.example.com",       // missing scheme
		"https://",              // missing host
		"://broken",             // unparseable scheme
		"javascript:alert(1)",   // no host
	}
	for _, ep := range invalid {
		if err := ValidateEndpoint(ep); err == nil {
			t.Errorf("ValidateEndpoint(%q) = nil, want error", ep)
		}
	}
}

func TestValidateKey(t *testing.T) {
	if err := ValidateKey("sk-test"); err != nil {
		t.Errorf("ValidateKey(sk-test) = %v, want nil", err)
	}
	for _, key := range []string{"", "   "} {
		if err := ValidateKey(key); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want error", key)
		}
	}
}
