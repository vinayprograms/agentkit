package credentials

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllEnvStore(t *testing.T) {
	os.Clearenv()                            // Clear all env vars to ensure a clean test environment
	for _, envVar := range providerEnvVars { // Set environment variables for all providers
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
	os.Clearenv() // Clear all env vars to ensure a clean test environment
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
	os.Clearenv() // Clear all env vars to ensure a clean test environment
	// Set environment variables for some providers
	os.Setenv("OPENAI_API_KEY", "openai-value")
	defer os.Unsetenv("OPENAI_API_KEY")

	store := NewEnvStore()

	assert.Equal(t, "openai-value", string(store.Get("openai")))
	assert.Equal(t, "", string(store.Get("anthropic")))
	assert.Equal(t, "", string(store.Get("google")))
}

func TestEnvStoreProviders(t *testing.T) {
	os.Clearenv() // Clear all env vars to ensure a clean test environment
	// Set environment variables for some providers
	os.Setenv("ANTHROPIC_API_KEY", "anthropic-value")
	defer os.Unsetenv("ANTHROPIC_API_KEY")
	os.Setenv("GOOGLE_API_KEY", "google-value")
	defer os.Unsetenv("GOOGLE_API_KEY")
	store := NewEnvStore()
	providers := store.Providers()

	expectedProviders := []string{"anthropic", "google"}
	assert.ElementsMatch(t, expectedProviders, providers)
}
