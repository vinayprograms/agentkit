# Memory Package Design

## What This Package Does

The `memory` package gives agents persistent memory that survives restarts. Agents can store observations (things they learned) and recall them later based on relevance to the current task.

## Why It Exists

LLMs have limited context windows. An agent can't keep everything it's ever learned in the prompt. But agents need to remember:

- **Findings** — Facts discovered ("API rate limit is 100/min")
- **Insights** — Conclusions reached ("REST works better here than GraphQL")
- **Lessons** — Knowledge for future ("Avoid library X, it lacks TypeScript support")

This package provides:
1. Storage for observations (persists to disk)
2. Relevance-based recall (find memories related to current task)
3. Categorization (distinguish findings from insights from lessons)

The agent stores knowledge as it works. Later, when facing a similar task, it retrieves relevant memories to inform its approach.

## When to Use It

**Use memory for:**
- Agents that run multiple sessions and should learn over time
- Long-running agents that need to offload knowledge from context
- Agents that should remember past mistakes and successes

**Don't use memory for:**
- Short-lived scripts (no persistence needed)
- Shared knowledge across agents (use `state` package)
- Structured data storage (use a proper database)

## Core Concepts

### Observations (FIL Model)

The package organizes knowledge into three categories:

**Findings** — Factual discoveries. Things the agent observed to be true.
- "The user prefers Python over JavaScript"
- "This API requires authentication"

**Insights** — Conclusions or inferences. Things the agent figured out.
- "Batch processing is faster than one-by-one"
- "The error is caused by a race condition"

**Lessons** — Learnings for the future. Things to remember next time.
- "Always check file permissions before writing"
- "This library has a memory leak, use alternative"

This FIL model helps agents recall the right type of knowledge for the situation.

### BM25 Search

Recall uses BM25 text search (via Bleve). You ask "what do I know about database connections?" and get ranked results based on text relevance.

Why BM25 instead of vector embeddings?
- No embedding model required (simpler, cheaper)
- Fast and well-understood algorithm
- Good enough for most agent memory use cases

### Scratchpad

Besides long-term observations, the package provides a simple key-value scratchpad for working memory. Store temporary values during a task, retrieve them later.

## Architecture


### Implementations

**BleveStore** — Production implementation using Bleve search engine. Observations are indexed for full-text search. Data persists to disk.

**InMemoryStore** — Testing implementation. Same interface but data lives only in memory. No persistence.

### Storage Layout

The store manages two types of data:
- **Observations** — Full-text indexed in Bleve
- **Key-value pairs** — JSON file for scratchpad

## Common Patterns

### Remember After Processing

As the agent completes tasks, extract observations and store them:
1. Agent finishes a task step
2. Observation extraction identifies findings/insights/lessons
3. Store observations with source metadata
4. Observations are now searchable

### Recall Before Acting

Before starting work, check what the agent knows:
1. Identify key concepts in the current task
2. Query memory for relevant observations
3. Include relevant memories in the prompt
4. Agent benefits from past experience

### Session Consolidation

At session end, consolidate important learnings:
1. Review transcript of what happened
2. Extract significant observations
3. Store for future sessions
4. Agent learns from each session

### Scratchpad for Working Memory

During a complex task:
1. Store intermediate results in scratchpad
2. Retrieve as needed within the task
3. Clear when task completes
4. Keeps context window focused

## Observation Extraction

The package can automatically extract observations from agent outputs. Given text from a step's completion, it identifies:
- What facts were discovered
- What conclusions were drawn
- What should be remembered

This is typically done via an LLM call — ask a small model to extract FIL from the text.

## Error Scenarios

| Situation | What Happens | What to Do |
|-----------|--------------|------------|
| No relevant memories | Recall returns empty | Normal, agent proceeds without past knowledge |
| Store unavailable | Operations fail | Check disk permissions, disk space |
| Corrupted index | Search may fail | Rebuild index from observations |
| Query too broad | Many irrelevant results | Use more specific queries |

## Design Decisions

### Why FIL categories?

Different situations need different knowledge:
- Debugging? Recall lessons about past bugs
- Planning? Recall insights about approaches
- Researching? Recall findings about facts

Categories enable targeted recall.

### Why not vector embeddings?

Simplicity and cost:
- No embedding model to run or pay for
- BM25 is fast and works offline
- For agent memory, text search is usually sufficient

Vector search could be added as an alternative backend if needed.

### Why separate from state?

Memory is single-agent, long-term, semantic. State is multi-agent, transient, key-value. Different use cases, different designs.

## Integration Notes

### With LLM Package

Observation extraction uses an LLM. Configure a small, fast model for extraction to minimize cost.

### With Telemetry

Memory operations can be traced — store and recall become spans with query details.

### With Shutdown

Call `Close()` to flush any pending writes and release resources.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Store/recall operations, FIL categorization |
| Search | BM25 relevance, query handling |
| Persistence | Data survives restart |
| Extraction | Observation extraction from text |
| Integration | Full memory workflow |
