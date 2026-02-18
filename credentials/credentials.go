// Package credentials loads API keys and OAuth tokens from standard locations.
package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ErrInsecurePermissions is returned when credentials file has overly permissive permissions.
var ErrInsecurePermissions = fmt.Errorf("credentials file has insecure permissions")

// ErrTokenExpired is returned when an OAuth token has expired and no refresh token is available.
var ErrTokenExpired = fmt.Errorf("oauth token expired")

// Credentials holds API keys and OAuth tokens loaded from credentials.toml
// Uses a generic map to support any provider without hardcoding.
type Credentials struct {
	// LLM is the generic LLM API key (used when provider-specific key not found)
	LLM *ProviderCreds `toml:"llm"`

	// Provider-specific sections (loaded dynamically)
	providers map[string]*ProviderCreds

	// OAuth tokens (loaded dynamically)
	oauthTokens map[string]*OAuthToken

	// sourcePath is the file this was loaded from (for saving)
	sourcePath string
}

// ProviderCreds holds credentials for a single provider
type ProviderCreds struct {
	APIKey string `toml:"api_key"`
}

// OAuthToken holds OAuth2 token credentials for a provider
type OAuthToken struct {
	AccessToken  string    `toml:"access_token"`
	RefreshToken string    `toml:"refresh_token,omitempty"`
	ExpiresAt    time.Time `toml:"expires_at,omitempty"`
	ClientID     string    `toml:"client_id,omitempty"`
	Scopes       []string  `toml:"scopes,omitempty"`
}

// IsExpired returns true if the token has expired or will expire within the buffer period.
func (t *OAuthToken) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false // No expiry set, assume valid
	}
	// Consider expired if within 5 minutes of expiry
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// IsValid returns true if the token exists and is not expired.
func (t *OAuthToken) IsValid() bool {
	return t != nil && t.AccessToken != "" && !t.IsExpired()
}

// rawCredentials is used for TOML unmarshaling
type rawCredentials struct {
	LLM      *ProviderCreds            `toml:"llm"`
	Sections map[string]*ProviderCreds `toml:"-"`
}

// StandardPaths returns the standard credential file locations in order of priority
func StandardPaths() []string {
	paths := []string{}

	// 1. Current directory
	paths = append(paths, "credentials.toml")

	// 2. ~/.config/grid/credentials.toml
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "grid", "credentials.toml"))
	}

	// 3. ~/.grid/credentials.toml (fallback)
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".grid", "credentials.toml"))
	}

	return paths
}

// Load loads credentials from the first available standard location
func Load() (*Credentials, string, error) {
	for _, path := range StandardPaths() {
		if _, err := os.Stat(path); err == nil {
			creds, err := LoadFile(path)
			if err != nil {
				return nil, path, err
			}
			return creds, path, nil
		}
	}
	return nil, "", nil // No credentials file found (not an error)
}

// DefaultPath returns the default credentials file path (~/.config/grid/credentials.toml)
func DefaultPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "grid", "credentials.toml")
	}
	return "credentials.toml"
}

// LoadFile loads credentials from a specific file.
// Returns ErrInsecurePermissions if file is readable by group or others.
func LoadFile(path string) (*Credentials, error) {
	// Check file permissions (Unix only)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		mode := info.Mode().Perm()
		// Credentials must be 0400 or 0600 (owner read-only or read-write)
		if mode != 0400 && mode != 0600 {
			return nil, fmt.Errorf("%w: %s has mode %04o (must be 0400 or 0600)",
				ErrInsecurePermissions, path, mode)
		}
	}

	// First pass: decode into a generic map to get all sections
	var rawData map[string]interface{}
	if _, err := toml.DecodeFile(path, &rawData); err != nil {
		return nil, err
	}

	creds := &Credentials{
		providers:   make(map[string]*ProviderCreds),
		oauthTokens: make(map[string]*OAuthToken),
		sourcePath:  path,
	}

	// Extract provider sections
	for key, value := range rawData {
		section, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		// Check for OAuth subsection (e.g., [anthropic.oauth])
		if oauthData, ok := section["oauth"].(map[string]interface{}); ok {
			token := parseOAuthToken(oauthData)
			if token != nil {
				creds.oauthTokens[key] = token
			}
		}

		// Check for API key
		apiKey, _ := section["api_key"].(string)
		if apiKey != "" {
			provCreds := &ProviderCreds{APIKey: apiKey}
			if key == "llm" {
				creds.LLM = provCreds
			} else {
				creds.providers[key] = provCreds
			}
		}
	}

	return creds, nil
}

// parseOAuthToken extracts an OAuthToken from a map
func parseOAuthToken(data map[string]interface{}) *OAuthToken {
	accessToken, _ := data["access_token"].(string)
	if accessToken == "" {
		return nil
	}

	token := &OAuthToken{
		AccessToken: accessToken,
	}

	if rt, ok := data["refresh_token"].(string); ok {
		token.RefreshToken = rt
	}
	if clientID, ok := data["client_id"].(string); ok {
		token.ClientID = clientID
	}

	// Parse expires_at (can be string or time.Time depending on TOML lib)
	if expiresStr, ok := data["expires_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, expiresStr); err == nil {
			token.ExpiresAt = t
		}
	} else if expiresTime, ok := data["expires_at"].(time.Time); ok {
		token.ExpiresAt = expiresTime
	}

	// Parse scopes
	if scopesRaw, ok := data["scopes"].([]interface{}); ok {
		for _, s := range scopesRaw {
			if scope, ok := s.(string); ok {
				token.Scopes = append(token.Scopes, scope)
			}
		}
	}

	return token
}

// GetAPIKey returns the API key or OAuth access token for a provider.
// Priority: [provider.oauth] (if valid) > [provider].api_key > [llm].api_key > env var
func (c *Credentials) GetAPIKey(provider string) string {
	if c != nil {
		// Normalize provider name (lowercase, no dashes)
		normalized := strings.ToLower(strings.ReplaceAll(provider, "-", ""))

		// Check OAuth token first (highest priority if valid)
		if token := c.GetOAuthToken(provider); token != nil && token.IsValid() {
			return token.AccessToken
		}

		// Check provider-specific section
		if creds, ok := c.providers[provider]; ok && creds.APIKey != "" {
			return creds.APIKey
		}
		// Try normalized name
		if creds, ok := c.providers[normalized]; ok && creds.APIKey != "" {
			return creds.APIKey
		}

		// Fall back to generic [llm] section
		if c.LLM != nil && c.LLM.APIKey != "" {
			return c.LLM.APIKey
		}
	}

	// Fallback to environment variable
	return os.Getenv(envVarForProvider(provider))
}

// GetOAuthToken returns the OAuth token for a provider, if any.
func (c *Credentials) GetOAuthToken(provider string) *OAuthToken {
	if c == nil || c.oauthTokens == nil {
		return nil
	}

	normalized := strings.ToLower(strings.ReplaceAll(provider, "-", ""))

	if token, ok := c.oauthTokens[provider]; ok {
		return token
	}
	if token, ok := c.oauthTokens[normalized]; ok {
		return token
	}
	return nil
}

// SetOAuthToken stores or updates an OAuth token for a provider.
func (c *Credentials) SetOAuthToken(provider string, token *OAuthToken) {
	if c.oauthTokens == nil {
		c.oauthTokens = make(map[string]*OAuthToken)
	}
	c.oauthTokens[provider] = token
}

// HasValidOAuthToken returns true if the provider has a valid (non-expired) OAuth token.
func (c *Credentials) HasValidOAuthToken(provider string) bool {
	token := c.GetOAuthToken(provider)
	return token != nil && token.IsValid()
}

// NeedsRefresh returns true if the provider has an OAuth token that needs refreshing.
func (c *Credentials) NeedsRefresh(provider string) bool {
	token := c.GetOAuthToken(provider)
	return token != nil && token.RefreshToken != "" && token.IsExpired()
}

// Save writes the credentials back to the source file (or default path).
func (c *Credentials) Save() error {
	path := c.sourcePath
	if path == "" {
		path = DefaultPath()
	}
	return c.SaveFile(path)
}

// SaveFile writes credentials to a specific file.
func (c *Credentials) SaveFile(path string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Build TOML content
	var sb strings.Builder

	// Write [llm] section if present
	if c.LLM != nil && c.LLM.APIKey != "" {
		sb.WriteString("[llm]\n")
		sb.WriteString(fmt.Sprintf("api_key = %q\n\n", c.LLM.APIKey))
	}

	// Write provider sections (API keys)
	for provider, creds := range c.providers {
		if creds.APIKey == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s]\n", provider))
		sb.WriteString(fmt.Sprintf("api_key = %q\n", creds.APIKey))

		// Check if there's also an OAuth token for this provider
		if token, ok := c.oauthTokens[provider]; ok && token.AccessToken != "" {
			writeOAuthSection(&sb, provider, token)
		}
		sb.WriteString("\n")
	}

	// Write OAuth-only providers (no API key)
	for provider, token := range c.oauthTokens {
		if _, hasAPIKey := c.providers[provider]; hasAPIKey {
			continue // Already written above
		}
		if token.AccessToken == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s.oauth]\n", provider))
		writeOAuthToken(&sb, token)
		sb.WriteString("\n")
	}

	// Write with secure permissions
	if err := os.WriteFile(path, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}

	c.sourcePath = path
	return nil
}

// writeOAuthSection writes an oauth subsection
func writeOAuthSection(sb *strings.Builder, provider string, token *OAuthToken) {
	sb.WriteString(fmt.Sprintf("\n[%s.oauth]\n", provider))
	writeOAuthToken(sb, token)
}

// writeOAuthToken writes OAuth token fields
func writeOAuthToken(sb *strings.Builder, token *OAuthToken) {
	sb.WriteString(fmt.Sprintf("access_token = %q\n", token.AccessToken))
	if token.RefreshToken != "" {
		sb.WriteString(fmt.Sprintf("refresh_token = %q\n", token.RefreshToken))
	}
	if !token.ExpiresAt.IsZero() {
		sb.WriteString(fmt.Sprintf("expires_at = %q\n", token.ExpiresAt.Format(time.RFC3339)))
	}
	if token.ClientID != "" {
		sb.WriteString(fmt.Sprintf("client_id = %q\n", token.ClientID))
	}
	if len(token.Scopes) > 0 {
		sb.WriteString("scopes = [")
		for i, scope := range token.Scopes {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%q", scope))
		}
		sb.WriteString("]\n")
	}
}

// envVarForProvider returns the environment variable name for a provider.
func envVarForProvider(provider string) string {
	// Known providers with standard env vars
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai", "openai-compat":
		return "OPENAI_API_KEY"
	case "google":
		return "GOOGLE_API_KEY"
	case "mistral":
		return "MISTRAL_API_KEY"
	case "groq":
		return "GROQ_API_KEY"
	case "brave":
		return "BRAVE_API_KEY"
	case "tavily":
		return "TAVILY_API_KEY"
	default:
		// Generic: PROVIDER_API_KEY
		return strings.ToUpper(strings.ReplaceAll(provider, "-", "_")) + "_API_KEY"
	}
}
