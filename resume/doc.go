// Package resume provides agent resume generation and matching for swarm coordination.
//
// A resume describes what an agent can do, derived from its Agentfile.
// Capabilities use dot-separated hierarchy (e.g., "code.golang", "test.unit").
// Resumes include embedding vectors for semantic matching via cosine similarity.
package resume
