package credentials

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllEnvStore(t *testing.T) {
	// Set environment variables for all providers
	for _, envVar := range providerEnvVars {
		os.Setenv(envVar, envVar+"-value")
		defer os.Unsetenv(envVar)
	}

	store := NewEnvStore()

	for provider, envVar := range providerEnvVars {
		expectedKey := envVar + "-value"
		assert.Equal(t, expectedKey, string(store.Get(provider)))
	}
}

func TestEnvStoreMissingVars(t *testing.T) {
	// Ensure no relevant environment variables are set
	for _, envVar := range providerEnvVars {
		os.Unsetenv(envVar)
	}

	store := NewEnvStore()

	for provider := range providerEnvVars {
		assert.Equal(t, "", string(store.Get(provider)))
	}
}

func TestEnvStorePartialVars(t *testing.T) {
	// Set environment variables for some providers
	os.Setenv("OPENAI_API_KEY", "openai-value")
	defer os.Unsetenv("OPENAI_API_KEY")

	store := NewEnvStore()

	assert.Equal(t, "openai-value", string(store.Get("openai")))
	assert.Equal(t, "", string(store.Get("anthropic")))
	assert.Equal(t, "", string(store.Get("google")))
}

func TestEnvStoreProviders(t *testing.T) {
	store := NewEnvStore()
	providers := store.Providers()

	expectedProviders := []string{"anthropic", "openai", "openai-compat", "google", "mistral", "groq", "brave", "tavily"}
	assert.ElementsMatch(t, expectedProviders, providers)
}
