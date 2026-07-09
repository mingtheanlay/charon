package tools

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateEndpoint reports whether ep is usable as an API base URL. A blank
// value is fine (the tool's own default applies); otherwise it must parse as
// an absolute http(s) URL with a host, so a typo is caught before it's written
// into a tool's live config.
func ValidateEndpoint(ep string) error {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return nil
	}
	u, err := url.Parse(ep)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid URL %q — expected e.g. https://api.example.com/v1", ep)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL %q must start with http:// or https://", ep)
	}
	return nil
}

// ValidateKey reports whether key is a non-empty API key/token once trimmed.
func ValidateKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("API key is required")
	}
	return nil
}
