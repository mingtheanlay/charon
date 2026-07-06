// Package models queries an AI provider's HTTP API for the list of models
// available to a given endpoint + key.
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Provider selects the wire format used to list models.
type Provider string

const (
	OpenAI    Provider = "openai"    // GET {base}/models, Authorization: Bearer
	Anthropic Provider = "anthropic" // GET {base}/models, x-api-key + anthropic-version
)

// modelsURL turns a user-supplied endpoint into the models-list URL, tolerating
// bases given with or without a trailing "/v1".
func modelsURL(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
		return base + "/models"
	}
	return base + "/v1/models"
}

// Fetch returns the sorted model IDs offered by endpoint for the given key.
func Fetch(provider Provider, endpoint, key string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL(endpoint), nil)
	if err != nil {
		return nil, err
	}
	switch provider {
	case Anthropic:
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API returned %s (check endpoint and key)", resp.Status)
	}

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("could not parse model list: %w", err)
	}

	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no models returned by %s", endpoint)
	}
	sort.Strings(ids)
	return ids, nil
}
