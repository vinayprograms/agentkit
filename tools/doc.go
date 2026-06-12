// Package tools provides LLM-callable tools and a registry to dispatch them.
//
// # Policy is not built in
//
// Tools enforce only their own constructor-time confinement (a filesystem
// workspace plus any extra allowed roots). They are unaware of package policy:
// a Registry does not consult any Policy, and Registry.Definitions returns
// every registered tool. Selecting which tools an agent may see or call, and
// enforcing path/domain policy, is the consumer's job.
//
// Two seams exist for this:
//
//   - Filtering: build the full registry, then narrow it with Registry.Subset
//     (or expose only a chosen Definitions slice to the model). Definitions is
//     not policy-aware, so filter before handing definitions to the LLM.
//
//   - Per-call enforcement: wrap a tool with a Guard via Entry.With at
//     registration. A Guard sees the validated Args for one tool, so close the
//     tool name into the guard and call your policy checks there. See package
//     policy for a copy-paste example adapting policy.CheckPath into a Guard.
package tools
