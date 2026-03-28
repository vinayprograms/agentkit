package credentials

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUnionStore_EmptyInputs(t *testing.T) {
	union := NewUnionStore()
	key := union.Get("any")
	assert.Equal(t, Credential(""), key, "Expected empty string for any provider when no stores are present")
	providers := union.Providers()
	assert.Empty(t, providers, "Expected no providers when no stores are present")
}

func TestUnionStore_GlobalAndLocalStore(t *testing.T) {
	// Use two filestores one at /tmp and another at /tmp/local to simulate global and local stores.

	// Write credentials to the global store
	global_Contents := `[anthropic]
api_key = "global_key1"

[google.oauth]
access_token = "oauth-access-token"
refresh_token = "oauth-refresh-token"
expires_at = ` + time.Now().Add(1*time.Hour).Format(time.RFC3339) + `
scopes = ["scope1", "scope2"]
refresh_url = "https://oauth2.googleapis.com/token"
`
	err := os.WriteFile("/tmp/credentials.toml", []byte(global_Contents), 0600)
	assert.NoError(t, err, "Failed to write global credentials")

	// Write credentials to the local store
	local_Contents := `[anthropic]
api_key = "local_key1"

[google.oauth]
access_token = "local-oauth-access-token"
refresh_token = "local-oauth-refresh-token"
expires_at = ` + time.Now().Add(5*time.Hour).Format(time.RFC3339) + `
scopes = ["scope1", "scope2"]
refresh_url = "https://oauth2.googleapis.com/token"
`
	err = os.MkdirAll("/tmp/local", 0700)
	assert.NoError(t, err, "Failed to create local directory")
	err = os.WriteFile("/tmp/local/credentials.toml", []byte(local_Contents), 0600)
	assert.NoError(t, err, "Failed to write local credentials")

	globalStore, err := NewFileStore("/tmp/credentials.toml")
	assert.NoError(t, err, "Failed to create global file store")
	localStore, err := NewFileStore("/tmp/local/credentials.toml")
	assert.NoError(t, err, "Failed to create local file store")
	union := NewUnionStore(&globalStore, &localStore)

	// Test that the union store returns the local credential (higher priority) for "anthropic"
	key := union.Get("anthropic")
	assert.Equal(t, Credential("local_key1"), key, "Expected local store credential to take priority over global store")

	// Test that the union store returns the local OAuth token for "google"
	googleKey := union.Get("google")
	assert.Equal(t, Credential("local-oauth-access-token"), googleKey, "Expected local store OAuth token to take priority over global store")

	// Test that Providers() returns both providers without duplicates
	providers := union.Providers()
	assert.ElementsMatch(t, []string{"anthropic", "google"}, providers, "Expected Providers() to return both providers without duplicates")
}

func TestUnionStore_TwoFileAndOneEnvStore(t *testing.T) {
	// Use two filestores one at /tmp and another at /tmp/local to simulate global and local stores.

	// Write credentials to the global store
	global_Contents := `[anthropic]
api_key = "global_key1"

[google.oauth]
access_token = "oauth-access-token"
refresh_token = "oauth-refresh-token"
expires_at = ` + time.Now().Add(1*time.Hour).Format(time.RFC3339) + `
scopes = ["scope1", "scope2"]
refresh_url = "https://oauth2.googleapis.com/token"
`
	err := os.WriteFile("/tmp/credentials.toml", []byte(global_Contents), 0600)
	assert.NoError(t, err, "Failed to write global credentials")

	// Write credentials to the local store
	local_Contents := `[anthropic]
api_key = "local_key1"

[google.oauth]
access_token = "local-oauth-access-token"
refresh_token = "local-oauth-refresh-token"
expires_at = ` + time.Now().Add(5*time.Hour).Format(time.RFC3339) + `
scopes = ["scope1", "scope2"]
refresh_url = "https://oauth2.googleapis.com/token"
`
	err = os.MkdirAll("/tmp/local", 0700)
	assert.NoError(t, err, "Failed to create local directory")
	err = os.WriteFile("/tmp/local/credentials.toml", []byte(local_Contents), 0600)
	assert.NoError(t, err, "Failed to write local credentials")

	globalStore, err := NewFileStore("/tmp/credentials.toml")
	assert.NoError(t, err, "Failed to create global file store")
	localStore, err := NewFileStore("/tmp/local/credentials.toml")
	assert.NoError(t, err, "Failed to create local file store")

	// Set environment variable for the env store
	os.Setenv("ANTHROPIC_API_KEY", "env_key1")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	envStore := NewEnvStore()
	union := NewUnionStore(&globalStore, &localStore, envStore)

	// Test that the union store returns the env credential (highest priority) for "anthropic"
	key := union.Get("anthropic")
	assert.Equal(t, Credential("env_key1"), key, "Expected env store credential to take priority over local and global stores")

	// Test that the union store returns the local OAuth token for "google" (since env store doesn't have it)
	googleKey := union.Get("google")
	assert.Equal(t, Credential("local-oauth-access-token"), googleKey, "Expected local store OAuth token to take priority over global store for google")

	// Test that Providers() returns both providers without duplicates
	providers := union.Providers()
	assert.ElementsMatch(t, []string{"anthropic", "google"}, providers, "Expected Providers() to return both providers without duplicates")
}
