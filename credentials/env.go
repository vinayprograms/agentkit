package credentials

import (
	"os"
)

// EnvStore reads credentials from environment variables.
type EnvStore map[string]EnvProvider
type EnvProvider struct {
	APIKey string
}

var providerEnvVars = map[string]string{
	"anthropic":     "ANTHROPIC_API_KEY",
	"openai":        "OPENAI_API_KEY",
	"openai-compat": "OPENAI_API_KEY",
	"google":        "GOOGLE_API_KEY",
	"mistral":       "MISTRAL_API_KEY",
	"groq":          "GROQ_API_KEY",
	"brave":         "BRAVE_API_KEY",
	"tavily":        "TAVILY_API_KEY",
}

// NewEnvStore creates a credential store backed by environment variables.
// Provider-specific environment variables are checked and mapped to providers. For example, "ANTHROPIC_API_KEY" maps to provider "anthropic".
func NewEnvStore() EnvStore {
	e := EnvStore{}
	for p, envVar := range providerEnvVars {
		if key := os.Getenv(envVar); key != "" {
			e[p] = EnvProvider{APIKey: key}
		}
	}
	return e
}

////////////////////////////////////////
// Interface implementation

// Get returns the API key from the environment variable for the provider.
func (e *EnvStore) Get(provider string) Credential {
	if key := os.Getenv(providerEnvVars[provider]); key != "" {
		return Credential(key)
	}
	return ""
}

// Providers returns an empty map; env vars cannot be enumerated by provider.
func (e *EnvStore) Providers() []string {
	providers := make([]string, 0, len(providerEnvVars))
	for provider := range providerEnvVars {
		providers = append(providers, provider)
	}
	return providers
}
