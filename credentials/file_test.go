package credentials

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMissingCredentialFile(t *testing.T) {
	_, err := NewFileStore("nonexistent.toml")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestCredentialFileWithInsecurePermissions(t *testing.T) {
	// Create temporary empty file
	tmpFile, err := os.CreateTemp("", "empty-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set permissions to 0644 (insecure)
	err = os.Chmod(tmpFile.Name(), 0644)
	assert.NoError(t, err)

	_, err = NewFileStore(tmpFile.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insecure permissions")
}

func TestEmptyCredentialFile(t *testing.T) {
	// Create temporary empty file
	tmpFile, err := os.CreateTemp("", "empty-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	store, err := NewFileStore(tmpFile.Name())
	assert.NoError(t, err)
	assert.Nil(t, store)
}

func TestInvalidCredentialFile(t *testing.T) {
	// Create temporary file with invalid TOML
	tmpFile, err := os.CreateTemp("", "invalid-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	contents := `
[anthropic
api_key = "anthropic-key"
`
	_, err = tmpFile.WriteString(contents)
	assert.NoError(t, err)

	_, err = NewFileStore(tmpFile.Name())
	assert.Error(t, err) // Expecting TOML parsing error. The exact error message may vary, so we just check that an error occurred.
}

func TestValidCredentialFile(t *testing.T) {
	// Create temporary file with valid credentials
	tmpFile, err := os.CreateTemp("", "valid-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	contents := `
[anthropic]
api_key = "anthropic-key"

[google]
api_key = "google-key"

[google.oauth]
access_token = "oauth-access-token"
refresh_token = "oauth-refresh-token"
expires_at = 2024-01-01T00:00:00Z
scopes = ["scope1", "scope2"]
refresh_url = "https://oauth2.googleapis.com/token"
`
	_, err = tmpFile.WriteString(contents)
	assert.NoError(t, err)

	store, err := NewFileStore(tmpFile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, store)

	anthropic, ok := store["anthropic"]
	assert.True(t, ok)
	assert.Equal(t, "anthropic-key", anthropic.APIKey)

	google, ok := store["google"]
	assert.True(t, ok)
	assert.Equal(t, "google-key", google.APIKey)
	assert.NotNil(t, google.OAuth)
	assert.Equal(t, "oauth-access-token", google.OAuth.AccessToken)
	assert.Equal(t, "oauth-refresh-token", google.OAuth.RefreshToken)
	assert.Equal(t, "https://oauth2.googleapis.com/token", google.OAuth.RefreshURL)
	assert.Equal(t, []string{"scope1", "scope2"}, google.OAuth.Scopes)
}

func TestGetProviderAPIKey(t *testing.T) {
	store := FileStore{
		"anthropic": TomlProvider{
			APIKey: "anthropic-key",
		},
	}

	apiKey := store.Get("anthropic")
	assert.Equal(t, "anthropic-key", string(apiKey))
}

func TestGetProviderAPIKeyMissingProvider(t *testing.T) {
	store := FileStore{}

	apiKey := store.Get("nonexistent")
	assert.Empty(t, string(apiKey))
}

func TestGetProviderAPIKeyMissingAPIKey(t *testing.T) {
	store := FileStore{
		"anthropic": TomlProvider{},
	}

	apiKey := store.Get("anthropic")
	assert.Empty(t, string(apiKey))
}

func TestGetProviderOAuthToken(t *testing.T) {
	store := FileStore{
		"google": TomlProvider{
			OAuth: &OAuthToken{
				AccessToken:  "oauth-access-token",
				RefreshToken: "oauth-refresh-token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scopes:       []string{"scope1", "scope2"},
				RefreshURL:   "https://oauth2.googleapis.com/token",
			},
		},
	}

	token := store.Get("google")
	assert.Equal(t, "oauth-access-token", string(token))
}

func TestGetProviderOAuthTokenExpired(t *testing.T) {
	store := FileStore{
		"google": TomlProvider{
			OAuth: &OAuthToken{
				AccessToken:  "oauth-access-token",
				RefreshToken: "oauth-refresh-token",
				ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired token
				Scopes:       []string{"scope1", "scope2"},
				RefreshURL:   "https://oauth2.googleapis.com/token",
			},
		},
	}

	token := store.Get("google")
	assert.Empty(t, string(token))
}

func TestGetProviderOAuthTokenMissingOAuth(t *testing.T) {
	store := FileStore{
		"google": TomlProvider{},
	}

	token := store.Get("google")
	assert.Empty(t, string(token))
}

func TestGetProviderOAuthTokenMissingProvider(t *testing.T) {
	store := FileStore{}

	token := store.Get("nonexistent")
	assert.Empty(t, string(token))
}

func TestProviders(t *testing.T) {
	store := FileStore{
		"anthropic": TomlProvider{
			APIKey: "anthropic-key",
		},
		"google": TomlProvider{
			OAuth: &OAuthToken{
				AccessToken:  "oauth-access-token",
				RefreshToken: "oauth-refresh-token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scopes:       []string{"scope1", "scope2"},
				RefreshURL:   "https://oauth2.googleapis.com/token",
			},
		},
	}

	providers := store.Providers()
	assert.ElementsMatch(t, []string{"anthropic", "google"}, providers)
}

func TestAddProviderWithAPIKey(t *testing.T) {
	store := FileStore{}

	store.SetAPIKey("anthropic", "anthropic-key")

	apiKey := store.Get("anthropic")
	assert.Equal(t, "anthropic-key", string(apiKey))

	// Save to file and verify file contents
	tmpFile, err := os.CreateTemp("", "credentials-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	err = store.Save(tmpFile.Name())
	assert.NoError(t, err)

	data, err := os.ReadFile(tmpFile.Name())
	assert.NoError(t, err)
	expectedContents := `[anthropic]
api_key = "anthropic-key"
`
	assert.Equal(t, expectedContents, string(data))
}

func TestUpdateProviderAPIKey(t *testing.T) {
	store := FileStore{
		"anthropic": TomlProvider{
			APIKey: "old-key",
		},
	}

	store.SetAPIKey("anthropic", "new-key")
	apiKey := store.Get("anthropic")
	assert.Equal(t, "new-key", string(apiKey))
}

func TestAddProviderWithOAuthToken(t *testing.T) {
	store := FileStore{}

	expiresAt := time.Now().Add(1 * time.Hour)
	token := OAuthToken{
		AccessToken:  "oauth-access-token",
		RefreshToken: "oauth-refresh-token",
		ExpiresAt:    expiresAt,
		Scopes:       []string{"scope1", "scope2"},
		RefreshURL:   "https://oauth2.googleapis.com/token",
	}

	store.SetOAuthToken("google", token)
	retrievedToken := store.Get("google")
	assert.Equal(t, "oauth-access-token", string(retrievedToken))
}

func TestUpdateProviderOAuthToken(t *testing.T) {
	store := FileStore{
		"google": TomlProvider{
			OAuth: &OAuthToken{
				AccessToken:  "old-access-token",
				RefreshToken: "old-refresh-token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scopes:       []string{"old-scope"},
				RefreshURL:   "https://oauth2.googleapis.com/token",
			},
		},
	}

	expiresAt := time.Now().Add(2 * time.Hour)
	newToken := OAuthToken{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		ExpiresAt:    expiresAt,
		Scopes:       []string{"new-scope"},
		RefreshURL:   "https://oauth2.googleapis.com/token",
	}

	store.SetOAuthToken("google", newToken)
	retrievedToken := store.Get("google")
	assert.Equal(t, "new-access-token", string(retrievedToken))
}

func TestSaveAPIKey(t *testing.T) {
	store := FileStore{
		"anthropic": TomlProvider{
			APIKey: "anthropic-key",
		},
	}

	tmpFile, err := os.CreateTemp("", "credentials-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	err = store.Save(tmpFile.Name())
	assert.NoError(t, err)

	data, err := os.ReadFile(tmpFile.Name())
	assert.NoError(t, err)
	expectedContents := `[anthropic]
api_key = "anthropic-key"
`
	assert.Equal(t, expectedContents, string(data))
}

func TestSaveMissingAPIKey(t *testing.T) {
	store := FileStore{
		"anthropic": TomlProvider{},
	}

	tmpFile, err := os.CreateTemp("", "credentials-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	err = store.Save(tmpFile.Name())
	assert.NoError(t, err)

	data, err := os.ReadFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, "", string(data))
}

func TestSaveOAuthToken(t *testing.T) {
	store := FileStore{
		"google": TomlProvider{
			OAuth: &OAuthToken{
				AccessToken:  "oauth-access-token",
				RefreshToken: "oauth-refresh-token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scopes:       []string{"scope1", "scope2"},
				RefreshURL:   "https://oauth2.googleapis.com/token",
			},
		},
	}

	tmpFile, err := os.CreateTemp("", "credentials-*.toml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	err = store.Save(tmpFile.Name())
	assert.NoError(t, err)

	data, err := os.ReadFile(tmpFile.Name())
	assert.NoError(t, err)
	expectedContents := `[google.oauth]
access_token = "oauth-access-token"
refresh_token = "oauth-refresh-token"
expires_at = ` + store["google"].OAuth.ExpiresAt.Format("2006-01-02T15:04:05-07:00") + `
scopes = ["scope1", "scope2"]
refresh_url = "https://oauth2.googleapis.com/token"
`

	assert.Equal(t, expectedContents, string(data))
}
