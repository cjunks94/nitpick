package ghapp

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// InstallationTokenSource mints + caches installation tokens. Tokens are
// valid for ~1 hour; we cache until 5 minutes before expiry to absorb clock
// skew and avoid mid-request expiration. Safe for concurrent use.
type InstallationTokenSource struct {
	AppID      string
	PrivateKey *rsa.PrivateKey
	BaseURL    string // defaults to https://api.github.com
	HTTPClient *http.Client

	mu    sync.Mutex
	cache map[int64]cachedToken // installationID -> token
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// NewInstallationTokenSource returns a source wired with reasonable defaults.
func NewInstallationTokenSource(appID string, key *rsa.PrivateKey) *InstallationTokenSource {
	return &InstallationTokenSource{
		AppID:      appID,
		PrivateKey: key,
		BaseURL:    "https://api.github.com",
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		cache:      make(map[int64]cachedToken),
	}
}

// Token returns a valid installation access token for the given installation.
// Mints a fresh one via the App JWT if no cached token is valid.
func (s *InstallationTokenSource) Token(ctx context.Context, installationID int64) (string, error) {
	s.mu.Lock()
	if t, ok := s.cache[installationID]; ok && time.Until(t.expiresAt) > 5*time.Minute {
		s.mu.Unlock()
		return t.token, nil
	}
	s.mu.Unlock()

	appJWT, err := MintAppJWT(s.AppID, s.PrivateKey)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", s.BaseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange JWT for installation token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("installation token exchange: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse installation token response: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("installation token response missing token field")
	}

	s.mu.Lock()
	s.cache[installationID] = cachedToken{token: out.Token, expiresAt: out.ExpiresAt}
	s.mu.Unlock()
	return out.Token, nil
}
