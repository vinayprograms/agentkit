package credentials

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// TOML Structure for 'CredentialStore':
// [anthropic]
// api_key = "anthropic-key"
//
// [google]
// api_key = "google-key"
//
// [google.oauth]
// access_token = "oauth-access-token"
// refresh_token = "oauth-refresh-token"
// expires_at = 2024-01-01T00:00:00Z
// scopes = ["scope1", "scope2"]
// refresh_url = "https://oauth2.googleapis.com/token"

type FileStore map[string]TomlProvider
type TomlProvider struct {
	APIKey string      `toml:"api_key,omitempty"`
	OAuth  *OAuthToken `toml:"oauth,omitempty"`
}

// NewFileStore creates an empty file-based credentials container.
func NewFileStore(filepath string) (FileStore, error) {
	// Check file permissions before loading, if file exists.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath)
		if err != nil {
			return nil, err
		}
		mode := info.Mode().Perm()
		if mode != 0400 && mode != 0600 {
			return nil, fmt.Errorf("credentials file has insecure permissions: %v", mode)
		}
	}

	// Load file
	var contents string
	if filepath != "" {
		data, err := os.ReadFile(filepath)
		if err != nil {
			return nil, err
		}
		contents = string(data)
		if contents == "" {
			return nil, nil
		}
	}

	var store FileStore
	if _, err := toml.Decode(contents, &store); err != nil {
		return FileStore{}, err
	}
	return store, nil
}

////////////////////////////////////////
// Interface implementation

// Get resolves a credential for a provider from file data.
// Priority: [provider.oauth] > [provider].api_key.
func (c FileStore) Get(provider string) Credential {
	if token := c[provider].OAuth; token != nil && token.IsValid() {
		return Credential(token.AccessToken)
	}

	if p, ok := c[provider]; ok && p.APIKey != "" {
		return Credential(p.APIKey)
	}

	return ""
}

func (c FileStore) Providers() []string {

	var providers []string

	for provider := range c {
		providers = append(providers, provider)
	}

	return providers
}

////////////////////////////////////////
// Additional methods

// SetAPIKey stores/updates an API key for a provider.
func (c *FileStore) SetAPIKey(provider, apiKey string) {
	if p, ok := (*c)[provider]; ok {
		p.APIKey = apiKey
		(*c)[provider] = p
	} else {
		(*c)[provider] = TomlProvider{APIKey: apiKey}
	}
}

func (c *FileStore) SetOAuthToken(provider string, token OAuthToken) {
	if p, ok := (*c)[provider]; ok {
		p.OAuth = &token
		(*c)[provider] = p
	} else {
		(*c)[provider] = TomlProvider{OAuth: &token}
	}
}

// Save writes credentials to a specific file.
func (c *FileStore) Save(path string) error {
	var sb strings.Builder

	for provider, creds := range *c {
		if creds.APIKey == "" && (creds.OAuth == nil || !creds.OAuth.IsValid()) {
			continue
		}

		if creds.APIKey != "" {
			type apiKeyOnly struct {
				APIKey string `toml:"api_key"`
			}
			encoder := toml.NewEncoder(&sb)
			encoder.Indent = ""
			if err := encoder.Encode(map[string]apiKeyOnly{provider: {creds.APIKey}}); err != nil {
				return fmt.Errorf("failed to encode credentials for provider %q: %w", provider, err)
			}
		}

		if creds.OAuth != nil && creds.OAuth.IsValid() {
			fmt.Fprintf(&sb, "[%s.oauth]\n", provider)
			oauth := *creds.OAuth
			oauth.ExpiresAt = oauth.ExpiresAt.Truncate(time.Second)
			encoder := toml.NewEncoder(&sb)
			encoder.Indent = ""
			if err := encoder.Encode(&oauth); err != nil {
				return fmt.Errorf("failed to encode OAuth for provider %q: %w", provider, err)
			}
		}
	}

	return os.WriteFile(path, []byte(sb.String()), 0600)
}
