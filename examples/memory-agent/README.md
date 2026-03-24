# Memory Agent

An agent with persistent BM25 memory, built on the simple-llm-agent pattern.

## What it demonstrates

1. **BleveStore** -- persistent memory with full-text search (BM25 ranking)
2. **Remember/Recall tools** -- the agent stores observations during conversation and retrieves them later
3. **FIL categorization** -- memories are classified as Findings (facts), Insights (inferences), or Lessons (rules learned)

## Running

```bash
export ANTHROPIC_API_KEY=sk-...   # or OPENAI_API_KEY / GOOGLE_API_KEY
go run main.go
```

Try a multi-turn conversation:
1. "Remember that our API rate limit is 100 requests per minute"
2. "Remember that exceeding the rate limit returns HTTP 429"
3. "What do you know about rate limits?"

Memories persist across restarts (stored in a temp directory).

## How it works

This example uses `memory.BleveStore` for on-disk BM25 search. Observations are stored with three categories:

- **Finding** -- a factual observation ("the server is at 10.0.0.42")
- **Insight** -- an inference drawn from findings ("the server seems overloaded during peak hours")
- **Lesson** -- a rule learned from experience ("always check server load before deploying")

The `RecallFIL` method searches all three categories in parallel and returns grouped results, giving the LLM structured context for its response.

For production agents, wire the store into agentkit's tool registry via `memory.NewToolsAdapter` and `registry.SetSemanticMemory` -- this gives the LLM native `remember` and `recall` tools automatically.
