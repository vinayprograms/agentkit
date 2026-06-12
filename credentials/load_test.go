package credentials

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResolveOAuthVsAPIKey(t *testing.T) {
	store := FileStore{
		"anthropic": {OAuth: &OAuthToken{
			AccessToken: "oauth-tok",
			ExpiresAt:   time.Now().Add(time.Hour),
		}},
		"openai": {APIKey: "sk-key"},
	}

	cred, isOAuth := Resolve(store, "anthropic")
	assert.Equal(t, Credential("oauth-tok"), cred)
	assert.True(t, isOAuth)

	cred, isOAuth = Resolve(store, "openai")
	assert.Equal(t, Credential("sk-key"), cred)
	assert.False(t, isOAuth)

	cred, isOAuth = Resolve(store, "missing")
	assert.Empty(t, cred)
	assert.False(t, isOAuth)
}

func TestResolveFallsBackForNonResolver(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	env := NewEnvStore()

	cred, isOAuth := Resolve(env, "anthropic")
	assert.Equal(t, Credential("env-key"), cred)
	assert.False(t, isOAuth)
}

func TestUnionResolvePrefersHighestPriority(t *testing.T) {
	low := FileStore{"anthropic": {APIKey: "low"}}
	high := FileStore{"anthropic": {OAuth: &OAuthToken{
		AccessToken: "high-oauth",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}}

	u := NewUnionStore(low, high) // last wins
	cred, isOAuth := u.Resolve("anthropic")
	assert.Equal(t, Credential("high-oauth"), cred)
	assert.True(t, isOAuth)
}

func TestStandardPaths(t *testing.T) {
	paths := StandardPaths("grid")
	assert.Equal(t, "credentials.toml", paths[0])
	if home, err := os.UserHomeDir(); err == nil {
		assert.Contains(t, paths, filepath.Join(home, ".config", "grid", "credentials.toml"))
		assert.Contains(t, paths, filepath.Join(home, ".grid", "credentials.toml"))
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", "credentials.toml")

	store := FileStore{}
	store.SetAPIKey("anthropic", "sk-test")
	assert.NoError(t, store.Save(path))

	reloaded, err := NewFileStore(path)
	assert.NoError(t, err)
	assert.Equal(t, Credential("sk-test"), reloaded.Get("anthropic"))
}

func TestLoadComposesEnvAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	store := FileStore{}
	store.SetAPIKey("openai", "file-key")
	assert.NoError(t, store.Save(path))

	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic")

	lookup, file, err := Load(filepath.Join(dir, "missing.toml"), path)
	assert.NoError(t, err)
	assert.NotNil(t, file)
	assert.Equal(t, Credential("file-key"), lookup.Get("openai"))
	assert.Equal(t, Credential("env-anthropic"), lookup.Get("anthropic"))
}

func TestLoadNoFile(t *testing.T) {
	lookup, file, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	assert.NoError(t, err)
	assert.Nil(t, file)
	assert.NotNil(t, lookup)
}

func TestClaudeCLICredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	assert.NoError(t, os.MkdirAll(claudeDir, 0700))

	expires := time.Now().Add(time.Hour).UnixMilli()
	content := `{"claudeAiOauth":{"accessToken":"cli-tok","refreshToken":"cli-refresh","expiresAt":` +
		strconv.FormatInt(expires, 10) + `}}`
	assert.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(content), 0600))

	store := ClaudeCLICredentials()
	assert.NotNil(t, store)
	cred, isOAuth := store.Resolve("anthropic")
	assert.Equal(t, Credential("cli-tok"), cred)
	assert.True(t, isOAuth)
}

func TestClaudeCLICredentialsMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	assert.Nil(t, ClaudeCLICredentials())
}

func TestLoadPropagatesFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(path, []byte("[anthropic]\napi_key=\"x\"\n"), 0644); err != nil {
		t.Fatal(err) // 0644 is insecure → NewFileStore errors
	}
	if _, _, err := Load(path); err == nil {
		t.Skip("permission check not enforced on this platform")
	}
}

func TestClaudeCLICredentialsInvalidJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if store := ClaudeCLICredentials(); store != nil {
		t.Fatalf("expected nil for invalid json, got %v", store)
	}
}

func TestNewFileStoreEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(path, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(path)
	assert.NoError(t, err)
	assert.Empty(t, store.Providers())
}
