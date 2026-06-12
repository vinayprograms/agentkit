# credentials

Credential resolution for API keys and OAuth tokens, with file, environment, and composable store backends.

## Usage

```go
// From a TOML file (requires 0400 or 0600 permissions).
fileStore, err := credentials.NewFileStore("credentials.toml")

// From environment variables (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.).
envStore := credentials.NewEnvStore()

// Compose with priority ordering (last store wins).
creds := credentials.NewChain(fileStore, envStore)

// Resolve a credential. Returns the usable token (API key or valid OAuth access token).
token := creds.Get("anthropic")

// List all available providers.
providers := creds.Providers()
```

## Credential File (credentials.toml)

File must have permissions `0400` or `0600` (security requirement).

```toml
[anthropic]
api_key = "sk-ant-..."

[google]
api_key = "AIza..."

[google.oauth]
access_token = "ya29.a0..."
refresh_token = "1//0e..."
expires_at = 2024-06-01T00:00:00Z
scopes = ["https://www.googleapis.com/auth/cloud-platform"]
refresh_url = "https://oauth2.googleapis.com/token"
```

## Environment Variables

`NewEnvStore()` checks these environment variables:

| Provider | Environment Variable |
|---|---|
| `anthropic` | `ANTHROPIC_API_KEY` |
| `openai` | `OPENAI_API_KEY` |
| `openai-compat` | `OPENAI_API_KEY` |
| `google` | `GOOGLE_API_KEY` |
| `mistral` | `MISTRAL_API_KEY` |
| `groq` | `GROQ_API_KEY` |
| `brave` | `BRAVE_API_KEY` |
| `tavily` | `TAVILY_API_KEY` |

## Priority and Composition

`Chain` checks stores in reverse order (last added = highest priority):

```go
creds := credentials.NewChain(
    globalFileStore,  // lowest priority
    localFileStore,   // overrides global
    envStore,         // highest priority
)
```

For providers with both API key and OAuth token, `Get()` prefers a valid (non-expired) OAuth token over the API key.

## Modifying Credentials

`FileStore` supports in-memory modification and persistence:

```go
store := credentials.FileStore{}
store.SetAPIKey("anthropic", "sk-ant-new-key")
store.SetOAuthToken("google", credentials.OAuthToken{
    AccessToken:  "ya29...",
    RefreshToken: "1//0e...",
    ExpiresAt:    time.Now().Add(1 * time.Hour),
    RefreshURL:   "https://oauth2.googleapis.com/token",
})
err := store.Save("credentials.toml") // writes with 0600 permissions
```

## Lookup Interface

All stores satisfy the `Lookup` interface:

```go
type Lookup interface {
    Get(provider string) Credential
    Providers() []string
}
```

`Credential` is a `string` type alias representing the resolved usable token.
