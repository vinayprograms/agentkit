# Simple LLM Agent

The simplest possible agent built with agentkit: an LLM provider, custom tools, and an agentic loop.

## What it demonstrates

1. **LLM Provider** -- create a provider from an API key (supports Anthropic, OpenAI, Google)
2. **Tool definitions** -- define tools as `llm.ToolDef` with JSON Schema parameters
3. **Agentic loop** -- the core pattern: send messages to the LLM, execute tool calls, feed results back, repeat until the LLM produces a final text response

## Running

```bash
export ANTHROPIC_API_KEY=sk-...   # or OPENAI_API_KEY / GOOGLE_API_KEY
go run main.go
```

Try prompts like:
- "What is 2^10 + 365?"
- "What time is it in Tokyo?"
- "Calculate 100 factorial divided by 99 factorial"

## The agentic loop pattern

```
User input
    |
    v
+-> LLM.Chat(messages, tools)
|       |
|       +-- No tool calls? --> Return text response
|       |
|       +-- Tool calls? --> Execute each tool
|               |
|               +-- Add tool results to messages
|               |
+---------------+
```

This is the fundamental building block for all agents. More complex agents add memory, multi-agent coordination, or streaming -- but they all share this core loop.
