// Package errors provides structured error types for agentkit.
//
// Every error carries a [Code] and [Category] that enable consistent
// handling across distributed agent systems.
//
// # Creating errors
//
//	err := errors.New(errors.Timeout, "fetching agent state")
//	err := errors.From(errors.NotFound)                           // uses default description
//	err := errors.New(errors.Internal, "bad state", errors.WithMetadata("key", "val"))
//
// # Wrapping errors
//
//	wrapped := errors.Wrap(err, "loading config")
//
// # Inspecting errors
//
//	if errors.Has(err, errors.Timeout) { ... }
//	if errors.IsRetryable(err) { ... }
//	code := errors.CodeOf(err)
//
// # JSON serialization
//
//	data, _ := json.Marshal(err)
//	var restored errors.Error
//	json.Unmarshal(data, &restored)
package errors
