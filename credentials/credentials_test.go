package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStandardPaths(t *testing.T) {
	paths := StandardPaths()
	if len(paths) < 2 {
		t.Errorf("expected at least 2 standard paths, got %d", len(paths))
	}
	if paths[0] != "credentials.toml" {
		t.Errorf("first path should be credentials.toml, got %s", paths[0])
	}
}

func TestLoadFile(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[anthropic]
api_key = "sk-ant-test123"

[openai]
api_key = "sk-openai-test456"
`
	os.WriteFile(credPath, []byte(content), 0400)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := creds.GetAPIKey("anthropic"); got != "sk-ant-test123" {
		t.Errorf("anthropic key = %q, want %q", got, "sk-ant-test123")
	}
	if got := creds.GetAPIKey("openai"); got != "sk-openai-test456" {
		t.Errorf("openai key = %q, want %q", got, "sk-openai-test456")
	}
}

func TestLoadFile_GenericLLMSection(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[llm]
api_key = "generic-llm-key"
`
	os.WriteFile(credPath, []byte(content), 0400)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Any provider should get the generic key
	if got := creds.GetAPIKey("anthropic"); got != "generic-llm-key" {
		t.Errorf("anthropic key = %q, want %q (from [llm])", got, "generic-llm-key")
	}
	if got := creds.GetAPIKey("openrouter"); got != "generic-llm-key" {
		t.Errorf("openrouter key = %q, want %q (from [llm])", got, "generic-llm-key")
	}
	if got := creds.GetAPIKey("my-custom-provider"); got != "generic-llm-key" {
		t.Errorf("my-custom-provider key = %q, want %q (from [llm])", got, "generic-llm-key")
	}
}

func TestLoadFile_ProviderOverridesLLM(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[llm]
api_key = "generic-key"

[anthropic]
api_key = "anthropic-specific-key"
`
	os.WriteFile(credPath, []byte(content), 0400)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Specific provider should use its own key
	if got := creds.GetAPIKey("anthropic"); got != "anthropic-specific-key" {
		t.Errorf("anthropic key = %q, want %q", got, "anthropic-specific-key")
	}
	// Other providers should use generic
	if got := creds.GetAPIKey("openai"); got != "generic-key" {
		t.Errorf("openai key = %q, want %q (from [llm])", got, "generic-key")
	}
}

func TestLoadFile_InsecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check not applicable on Windows")
	}

	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[llm]
api_key = "secret-key"
`
	os.WriteFile(credPath, []byte(content), 0644)

	_, err := LoadFile(credPath)
	if err == nil {
		t.Fatal("expected error for insecure permissions")
	}
	if !errors.Is(err, ErrInsecurePermissions) {
		t.Errorf("expected ErrInsecurePermissions, got %v", err)
	}
}

func TestLoadFile_SecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check not applicable on Windows")
	}

	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[llm]
api_key = "secret-key"
`
	os.WriteFile(credPath, []byte(content), 0400)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("0400 should be allowed: %v", err)
	}
	if creds.GetAPIKey("any-provider") != "secret-key" {
		t.Error("expected api_key to be loaded")
	}
}

func TestLoadFile_RejectGroupReadablePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check not applicable on Windows")
	}

	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[llm]
api_key = "secret-key"
`
	// 0644 allows group/others to read - should be rejected
	os.WriteFile(credPath, []byte(content), 0644)

	_, err := LoadFile(credPath)
	if err == nil {
		t.Fatal("expected error for 0644 permissions (group readable)")
	}
	if !errors.Is(err, ErrInsecurePermissions) {
		t.Errorf("expected ErrInsecurePermissions, got %v", err)
	}
}

func TestLoadFile_Accept0600Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check not applicable on Windows")
	}

	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[llm]
api_key = "secret-key"
`
	// 0600 allows owner read/write - needed for OAuth token updates
	os.WriteFile(credPath, []byte(content), 0600)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("0600 should be accepted: %v", err)
	}
	if creds.LLM == nil || creds.LLM.APIKey != "secret-key" {
		t.Error("expected to load credentials with 0600 permissions")
	}
}

func TestGetAPIKey_FallbackToEnv(t *testing.T) {
	os.Setenv("ANTHROPIC_API_KEY", "env-anthropic")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	creds := &Credentials{providers: make(map[string]*ProviderCreds)}

	if got := creds.GetAPIKey("anthropic"); got != "env-anthropic" {
		t.Errorf("GetAPIKey(anthropic) = %q, want %q (from env)", got, "env-anthropic")
	}
}

func TestGetAPIKey_CredentialsTakesPriority(t *testing.T) {
	os.Setenv("ANTHROPIC_API_KEY", "env-value")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	creds := &Credentials{
		providers: map[string]*ProviderCreds{
			"anthropic": {APIKey: "creds-value"},
		},
	}

	if got := creds.GetAPIKey("anthropic"); got != "creds-value" {
		t.Errorf("GetAPIKey(anthropic) = %q, want %q (creds should take priority)", got, "creds-value")
	}
}

func TestGetAPIKey_NilCredentials(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "env-openai")
	defer os.Unsetenv("OPENAI_API_KEY")

	var creds *Credentials

	if got := creds.GetAPIKey("openai"); got != "env-openai" {
		t.Errorf("GetAPIKey(openai) = %q, want %q (from env with nil creds)", got, "env-openai")
	}
}

func TestGetAPIKey_GenericEnvVar(t *testing.T) {
	// Unknown provider should generate PROVIDER_API_KEY env var
	os.Setenv("MYCUSTOM_API_KEY", "custom-env-value")
	defer os.Unsetenv("MYCUSTOM_API_KEY")

	creds := &Credentials{providers: make(map[string]*ProviderCreds)}

	if got := creds.GetAPIKey("mycustom"); got != "custom-env-value" {
		t.Errorf("GetAPIKey(mycustom) = %q, want %q", got, "custom-env-value")
	}
}

func TestLoad_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Also set HOME to tmpDir so StandardPaths doesn't find real config files
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	creds, path, err := Load()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if creds != nil {
		t.Error("expected nil credentials when no file exists")
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}

func TestLoad_FromCurrentDir(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	content := `
[llm]
api_key = "from-current-dir"
`
	os.WriteFile("credentials.toml", []byte(content), 0400)

	creds, path, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Fatal("expected credentials to be loaded")
	}
	if creds.GetAPIKey("any") != "from-current-dir" {
		t.Errorf("unexpected api key: %s", creds.GetAPIKey("any"))
	}
	if path != "credentials.toml" {
		t.Errorf("expected path 'credentials.toml', got %q", path)
	}
}

func TestLoadFile_AnyProviderSection(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[openrouter]
api_key = "openrouter-key"

[my-custom-llm]
api_key = "custom-key"
`
	os.WriteFile(credPath, []byte(content), 0400)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := creds.GetAPIKey("openrouter"); got != "openrouter-key" {
		t.Errorf("openrouter key = %q, want %q", got, "openrouter-key")
	}
	if got := creds.GetAPIKey("my-custom-llm"); got != "custom-key" {
		t.Errorf("my-custom-llm key = %q, want %q", got, "custom-key")
	}
}

func TestLoadFile_OAuthToken(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[anthropic]
api_key = "sk-ant-fallback"

[anthropic.oauth]
access_token = "oauth-access-token"
refresh_token = "oauth-refresh-token"
expires_at = "2030-01-01T00:00:00Z"
client_id = "test-client"
scopes = ["api", "read"]
`
	os.WriteFile(credPath, []byte(content), 0600)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// OAuth token should take priority
	if got := creds.GetAPIKey("anthropic"); got != "oauth-access-token" {
		t.Errorf("expected OAuth token, got %q", got)
	}

	// Check OAuth token details
	token := creds.GetOAuthToken("anthropic")
	if token == nil {
		t.Fatal("expected OAuth token")
	}
	if token.RefreshToken != "oauth-refresh-token" {
		t.Errorf("refresh_token = %q, want %q", token.RefreshToken, "oauth-refresh-token")
	}
	if token.ClientID != "test-client" {
		t.Errorf("client_id = %q, want %q", token.ClientID, "test-client")
	}
	if len(token.Scopes) != 2 || token.Scopes[0] != "api" {
		t.Errorf("scopes = %v, want [api, read]", token.Scopes)
	}
	if token.IsExpired() {
		t.Error("token should not be expired (expires 2030)")
	}
}

func TestOAuthToken_ExpiredFallsBackToAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	content := `
[anthropic]
api_key = "sk-ant-fallback"

[anthropic.oauth]
access_token = "expired-oauth-token"
expires_at = "2020-01-01T00:00:00Z"
`
	os.WriteFile(credPath, []byte(content), 0600)

	creds, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expired OAuth token should fall back to API key
	if got := creds.GetAPIKey("anthropic"); got != "sk-ant-fallback" {
		t.Errorf("expected fallback to API key, got %q", got)
	}
}

func TestCredentials_SaveFile(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "credentials.toml")

	creds := &Credentials{
		LLM: &ProviderCreds{APIKey: "llm-key"},
		providers: map[string]*ProviderCreds{
			"anthropic": {APIKey: "ant-key"},
		},
		oauthTokens: map[string]*OAuthToken{
			"github-copilot": {
				AccessToken:  "gho_token",
				RefreshToken: "ghr_token",
				ClientID:     "my-app",
			},
		},
	}

	if err := creds.SaveFile(credPath); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	// Reload and verify
	loaded, err := LoadFile(credPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if got := loaded.GetAPIKey("anthropic"); got != "ant-key" {
		t.Errorf("anthropic key = %q, want %q", got, "ant-key")
	}

	token := loaded.GetOAuthToken("github-copilot")
	if token == nil || token.AccessToken != "gho_token" {
		t.Errorf("github-copilot token not loaded correctly")
	}
}
