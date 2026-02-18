// Provider-specific OAuth configurations.
package auth

// Provider constants
const (
	ProviderAnthropic     = "anthropic"
	ProviderGitHubCopilot = "github-copilot"
)

// AnthropicConfig returns the OAuth config for Anthropic Console.
// Note: Client ID should be registered with Anthropic.
func AnthropicConfig(clientID string) DeviceAuthConfig {
	return DeviceAuthConfig{
		Provider:      ProviderAnthropic,
		ClientID:      clientID,
		DeviceAuthURL: "https://console.anthropic.com/oauth/device/code",
		TokenURL:      "https://console.anthropic.com/oauth/token",
		Scopes:        []string{"api"},
	}
}

// GitHubCopilotConfig returns the OAuth config for GitHub Copilot.
// Uses GitHub's device flow with Copilot scope.
func GitHubCopilotConfig(clientID string) DeviceAuthConfig {
	// Default to GitHub's known Copilot client ID if not provided
	if clientID == "" {
		// This is the VS Code Copilot extension client ID
		// For production, you should register your own OAuth app
		clientID = "Iv1.b507a08c87ecfe98"
	}

	return DeviceAuthConfig{
		Provider:      ProviderGitHubCopilot,
		ClientID:      clientID,
		DeviceAuthURL: "https://github.com/login/device/code",
		TokenURL:      "https://github.com/login/oauth/access_token",
		Scopes:        []string{"copilot"},
	}
}

// GetProviderConfig returns the OAuth config for a known provider.
// Returns nil if provider is not recognized.
func GetProviderConfig(provider, clientID string) *DeviceAuthConfig {
	switch provider {
	case ProviderAnthropic:
		cfg := AnthropicConfig(clientID)
		return &cfg
	case ProviderGitHubCopilot:
		cfg := GitHubCopilotConfig(clientID)
		return &cfg
	default:
		return nil
	}
}

// GetTokenURL returns the token endpoint for a provider (for refresh).
func GetTokenURL(provider string) string {
	switch provider {
	case ProviderAnthropic:
		return "https://console.anthropic.com/oauth/token"
	case ProviderGitHubCopilot:
		return "https://github.com/login/oauth/access_token"
	default:
		return ""
	}
}

// SupportsOAuth returns true if the provider supports OAuth authentication.
func SupportsOAuth(provider string) bool {
	switch provider {
	case ProviderAnthropic, ProviderGitHubCopilot:
		return true
	default:
		return false
	}
}
