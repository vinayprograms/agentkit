// Claude CLI credential discovery.
// Reads OAuth tokens from Claude CLI's credential store.
package credentials

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ClaudeCliCredentialsPath returns the path to Claude CLI's credentials file.
func ClaudeCliCredentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

// claudeCliFile represents the structure of ~/.claude/.credentials.json
type claudeCliFile struct {
	ClaudeAiOauth *claudeCliOAuth `json:"claudeAiOauth"`
}

type claudeCliOAuth struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"` // Unix timestamp in milliseconds
}

// ReadClaudeCliCredentials reads OAuth credentials from Claude CLI's credential store.
// Returns nil if credentials file doesn't exist or can't be parsed.
func ReadClaudeCliCredentials() *OAuthToken {
	credPath := ClaudeCliCredentialsPath()
	if credPath == "" {
		return nil
	}

	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil
	}

	var cliFile claudeCliFile
	if err := json.Unmarshal(data, &cliFile); err != nil {
		return nil
	}

	if cliFile.ClaudeAiOauth == nil || cliFile.ClaudeAiOauth.AccessToken == "" {
		return nil
	}

	oauth := cliFile.ClaudeAiOauth
	token := &OAuthToken{
		AccessToken:  oauth.AccessToken,
		RefreshToken: oauth.RefreshToken,
	}

	// Convert milliseconds to time.Time
	if oauth.ExpiresAt > 0 {
		token.ExpiresAt = time.UnixMilli(oauth.ExpiresAt)
	}

	return token
}

// HasClaudeCliCredentials returns true if valid Claude CLI credentials exist.
func HasClaudeCliCredentials() bool {
	token := ReadClaudeCliCredentials()
	return token != nil && token.IsValid()
}
