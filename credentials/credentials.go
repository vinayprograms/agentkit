// Package credentials provides credential store interfaces and implementations
// for API key and OAuth token management.
//
// Two Store implementations are provided:
//   - FileStore: reads credentials from a TOML file (file.go)
//   - EnvStore: reads credentials from environment variables (env.go)
//
// Use Chain to compose stores with priority ordering (e.g., file > env).
package credentials

// Credential holds a resolved credential (API key or OAuth token).
type Credential string

// Interface to look up a specific provider's credential.
type Lookup interface {

	// Get the credential for a provider, returning the credential and whether it was found.
	Get(provider string) (Credential, bool)

	// Invalidates the credential for a provider.
	Invalidate(provider string)

	// List all providers with available credentials.
	Providers() []string
}
