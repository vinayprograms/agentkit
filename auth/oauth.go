// Package auth provides OAuth2 authentication flows for LLM providers.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vinayprograms/agentkit/credentials"
)

// httpClient is a shared HTTP client with timeout for OAuth requests.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// DeviceAuthConfig configures the OAuth2 device authorization flow.
type DeviceAuthConfig struct {
	// Provider name (e.g., "anthropic", "github-copilot")
	Provider string

	// ClientID for the OAuth application
	ClientID string

	// DeviceAuthURL is the device authorization endpoint
	DeviceAuthURL string

	// TokenURL is the token endpoint
	TokenURL string

	// Scopes to request
	Scopes []string

	// PollInterval for token polling (default 5s)
	PollInterval time.Duration

	// Timeout for the entire flow (default 5 minutes)
	Timeout time.Duration
}

// DeviceCodeResponse is returned from the device authorization endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse is returned from the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// DeviceFlowCallbacks allows customization of user interaction.
type DeviceFlowCallbacks struct {
	// OnUserCode is called when the user code is available.
	// Implementations should display the verification URL and code to the user.
	OnUserCode func(verificationURI, userCode string)

	// OnPollAttempt is called on each poll attempt (optional).
	OnPollAttempt func(attempt int)

	// OnSuccess is called when authentication succeeds (optional).
	OnSuccess func()
}

// DefaultCallbacks returns callbacks that print to stdout.
func DefaultCallbacks() *DeviceFlowCallbacks {
	return &DeviceFlowCallbacks{
		OnUserCode: func(uri, code string) {
			fmt.Printf("\n🔐 OAuth Authentication Required\n")
			fmt.Printf("   Visit: %s\n", uri)
			fmt.Printf("   Enter code: %s\n\n", code)
			fmt.Printf("   Waiting for authorization...\n")
		},
		OnPollAttempt: func(attempt int) {
			fmt.Printf(".")
		},
		OnSuccess: func() {
			fmt.Printf("\n✓ Authentication successful!\n")
		},
	}
}

// DeviceAuth performs the OAuth2 device authorization flow.
// Returns a token that can be stored in credentials.toml.
func DeviceAuth(ctx context.Context, cfg DeviceAuthConfig, callbacks *DeviceFlowCallbacks) (*credentials.OAuthToken, error) {
	if callbacks == nil {
		callbacks = DefaultCallbacks()
	}

	// Set defaults
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}

	// Step 1: Request device code
	deviceCode, err := requestDeviceCode(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}

	// Notify user
	if callbacks.OnUserCode != nil {
		callbacks.OnUserCode(deviceCode.VerificationURI, deviceCode.UserCode)
	}

	// Use interval from response if provided
	pollInterval := cfg.PollInterval
	if deviceCode.Interval > 0 {
		pollInterval = time.Duration(deviceCode.Interval) * time.Second
	}

	// Step 2: Poll for token
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("authentication timed out")
		case <-time.After(pollInterval):
			attempt++
			if callbacks.OnPollAttempt != nil {
				callbacks.OnPollAttempt(attempt)
			}

			token, err := pollForToken(ctx, cfg, deviceCode.DeviceCode)
			if err != nil {
				// Check for expected "pending" errors
				if strings.Contains(err.Error(), "authorization_pending") {
					continue
				}
				if strings.Contains(err.Error(), "slow_down") {
					pollInterval += 5 * time.Second
					continue
				}
				return nil, err
			}

			if callbacks.OnSuccess != nil {
				callbacks.OnSuccess()
			}

			return token, nil
		}
	}
}

// requestDeviceCode initiates the device flow.
func requestDeviceCode(ctx context.Context, cfg DeviceAuthConfig) (*DeviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", cfg.ClientID)
	if len(cfg.Scopes) > 0 {
		data.Set("scope", strings.Join(cfg.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.DeviceAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device auth failed: %s", string(body))
	}

	var deviceCode DeviceCodeResponse
	if err := json.Unmarshal(body, &deviceCode); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	return &deviceCode, nil
}

// pollForToken polls the token endpoint.
func pollForToken(ctx context.Context, cfg DeviceAuthConfig, deviceCode string) (*credentials.OAuthToken, error) {
	data := url.Values{}
	data.Set("client_id", cfg.ClientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Check for OAuth errors
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	// Build credentials token
	token := &credentials.OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ClientID:     cfg.ClientID,
	}

	if tokenResp.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	if tokenResp.Scope != "" {
		token.Scopes = strings.Split(tokenResp.Scope, " ")
	} else {
		token.Scopes = cfg.Scopes
	}

	return token, nil
}

// RefreshToken refreshes an expired OAuth token.
func RefreshToken(ctx context.Context, tokenURL, clientID string, token *credentials.OAuthToken) (*credentials.OAuthToken, error) {
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("refresh failed: %s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	newToken := &credentials.OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ClientID:     clientID,
		Scopes:       token.Scopes,
	}

	// Keep old refresh token if new one not provided
	if newToken.RefreshToken == "" {
		newToken.RefreshToken = token.RefreshToken
	}

	if tokenResp.ExpiresIn > 0 {
		newToken.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return newToken, nil
}
