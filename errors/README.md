# errors

Structured errors with codes, categories, retryability, and metadata. A drop-in complement to stdlib `errors` — all standard functions (`As`, `Is`, `Unwrap`, `Join`) are re-exported so callers import only this package.

## Core types

**`Code`** — identifies the failure. Codes are grouped into four categories automatically:
- *Transient* (`Timeout`, `Unavailable`, `NetworkErr`, `RetryLater`, `AgentOffline`, …) — retry may succeed.
- *Permanent* (`NotFound`, `InvalidInput`, `Forbidden`, `Unsupported`, …) — retry will not help.
- *Resource* (`RateLimit`, `QuotaExceeded`, `ResourceBusy`, `Capacity`) — exhaustion or quota.
- *Internal* (`Internal`, `Corruption`, `Assertion`, `Panic`) — bugs or system failures.

**`Error`** — carries a code, category, message, optional cause, and optional key-value metadata. Implements `json.Marshaler` / `json.Unmarshaler`.

## Creating errors

```go
// from a code with default description
err := errors.From(errors.NotFound)

// with a custom message
err := errors.New(errors.Timeout, "upstream LLM did not respond")

// formatted message
err := errors.Newf(errors.InvalidInput, "model %q is not supported", name)

// with options
err := errors.New(errors.RateLimit, "too many requests",
    errors.WithRetryable(true),
    errors.WithMetadata("provider", "anthropic"),
)
```

## Wrapping and inspecting

```go
// wraps any error, preserving code/category if it's already an *Error
wrapped := errors.Wrap(err, "loading config")

// check code
if errors.Has(err, errors.NotFound) { ... }

// check retryability
if errors.IsRetryable(err) { ... }

// extract code
code := errors.CodeOf(err)
```

## Panic recovery

```go
defer func() {
    if r := recover(); r != nil {
        err = errors.RecoverPanic(r)
    }
}()
```
