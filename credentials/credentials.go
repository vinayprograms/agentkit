// Package credentials provides credential store interfaces and implementations
// for API key and OAuth token management.
//
// Two Store implementations are provided:
//   - FileStore: reads credentials from a TOML file (file.go)
//   - EnvStore: reads credentials from environment variables (env.go)
//
// Use Chain (NewChain) to compose stores with priority ordering (e.g., file > env):
// it tries each store in turn and returns the first credential found.
package credentials

// Credential holds a resolved credential (API key or OAuth token).
type Credential string

// Interface to look up a specific provider's credential.
type Lookup interface {

	// Get returns the credential for a provider, or an empty credential if not found.
	Get(provider string) Credential

	// List all providers with available credentials.
	Providers() []string
}

// OAuthResolver is an optional interface a Lookup may implement to report
// whether a resolved credential is an OAuth access token (as opposed to a
// static API key). LLM providers such as Anthropic authenticate OAuth tokens
// with a Bearer scheme rather than an API-key header, so callers wiring a
// resolved credential into llm.Config need to know which it is.
type OAuthResolver interface {
	// Resolve returns the credential for a provider and whether it is an OAuth
	// access token. The credential is empty if none is found.
	Resolve(provider string) (cred Credential, isOAuth bool)
}

// Resolve returns the credential for a provider and whether it is an OAuth
// access token. If the Lookup implements OAuthResolver that path is used;
// otherwise the credential is resolved via Get and isOAuth is reported false.
//
// Use this instead of Get when the result feeds llm.Config.IsOAuthToken.
func Resolve(l Lookup, provider string) (cred Credential, isOAuth bool) {
	if r, ok := l.(OAuthResolver); ok {
		return r.Resolve(provider)
	}
	return l.Get(provider), false
}
