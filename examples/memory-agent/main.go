// Package main demonstrates an agent with persistent BM25 memory using agentkit.
//
// This extends the simple-llm-agent pattern with a BleveStore for persistent
// memory. The agent can remember facts during conversation and recall them
// later using full-text search.
//
// Usage:
//
//	export ANTHROPIC_API_KEY=sk-...
//	go run main.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/vinayprograms/agentkit/llm"
	"github.com/vinayprograms/agentkit/memory"
)

func main() {
	provider, err := newProvider()
	if err != nil {
		log.Fatalf("Failed to create LLM provider: %v", err)
	}

	// Set up persistent memory with BM25 search.
	// Data is stored on disk and survives restarts.
	dataDir := filepath.Join(os.TempDir(), "agentkit-memory-example")
	store, err := memory.NewBleveStore(memory.BleveStoreConfig{
		BasePath: dataDir,
	})
	if err != nil {
		log.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	fmt.Printf("Memory Agent (data stored in %s)\n", dataDir)
	fmt.Println("Type 'quit' to exit.")
	fmt.Println()
	fmt.Println("Try:")
	fmt.Println("  \"Remember that the deployment password is hunter2\"")
	fmt.Println("  \"Remember that the staging server is at 10.0.0.42\"")
	fmt.Println("  \"What do you know about the deployment?\"")
	fmt.Println()

	tools := []llm.ToolDef{
		rememberToolDef(),
		recallToolDef(),
	}

	scanner := bufio.NewScanner(os.Stdin)
	var history []llm.Message

	// Add a system prompt that tells the LLM about its memory capabilities.
	history = append(history, llm.Message{
		Role: "system",
		Content: `You are a helpful assistant with persistent memory. You have two tools:
- remember: Store important facts, decisions, and observations for later retrieval.
- recall: Search your memory for previously stored information.

When the user tells you something worth remembering, use the remember tool.
When the user asks about something you might have stored, use recall first.
Always tell the user what you remembered or found.`,
	})

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "quit" {
			break
		}

		history = append(history, llm.Message{Role: "user", Content: input})

		response, err := agentLoop(context.Background(), provider, history, tools, store)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		history = append(history, llm.Message{Role: "assistant", Content: response})
		fmt.Printf("\n%s\n\n", response)
	}
}

// agentLoop is the same core pattern as simple-llm-agent, with tool dispatch
// wired to memory operations.
func agentLoop(ctx context.Context, provider llm.Model, history []llm.Message, tools []llm.ToolDef, store memory.Store) (string, error) {
	messages := make([]llm.Message, len(history))
	copy(messages, history)

	for i := 0; i < 10; i++ {
		resp, err := provider.Chat(ctx, llm.ChatRequest{
			Messages:  messages,
			Tools:     tools,
			MaxTokens: 4096,
		})
		if err != nil {
			return "", fmt.Errorf("LLM chat failed: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			result := executeTool(ctx, tc.Name, tc.Args, store)
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("agent loop exceeded maximum iterations")
}

func executeTool(ctx context.Context, name string, args map[string]interface{}, store memory.Store) string {
	switch name {
	case "remember":
		return executeRemember(ctx, args, store)
	case "recall":
		return executeRecall(ctx, args, store)
	default:
		return fmt.Sprintf("unknown tool: %s", name)
	}
}

// --- Tool: remember ---

func rememberToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        "remember",
		Description: "Store an observation in persistent memory. Categorize it as a finding (fact), insight (inference), or lesson (rule learned).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to remember.",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"finding", "insight", "lesson"},
					"description": "Category: finding (a fact), insight (an inference), or lesson (a rule).",
				},
			},
			"required": []string{"content", "category"},
		},
	}
}

func executeRemember(ctx context.Context, args map[string]interface{}, store memory.Store) string {
	content, _ := args["content"].(string)
	category, _ := args["category"].(string)
	if content == "" || category == "" {
		return "error: content and category are required"
	}

	id, err := store.RememberObservation(ctx, content, category, "conversation")
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	return fmt.Sprintf("Stored %s (id: %s)", category, id)
}

// --- Tool: recall ---

func recallToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        "recall",
		Description: "Search persistent memory for previously stored observations. Returns findings, insights, and lessons matching the query.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query to find relevant memories.",
				},
			},
			"required": []string{"query"},
		},
	}
}

func executeRecall(ctx context.Context, args map[string]interface{}, store memory.Store) string {
	query, _ := args["query"].(string)
	if query == "" {
		return "error: query is required"
	}

	results, err := store.RecallFIL(ctx, query, 5)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	// Format results for the LLM.
	type recallResponse struct {
		Findings []string `json:"findings,omitempty"`
		Insights []string `json:"insights,omitempty"`
		Lessons  []string `json:"lessons,omitempty"`
	}

	resp := recallResponse{
		Findings: results.Findings,
		Insights: results.Insights,
		Lessons:  results.Lessons,
	}

	if len(resp.Findings) == 0 && len(resp.Insights) == 0 && len(resp.Lessons) == 0 {
		return "No memories found matching that query."
	}

	b, _ := json.Marshal(resp)
	return string(b)
}

// newProvider creates an LLM provider from environment variables.
func newProvider() (llm.Model, error) {
	providers := []struct {
		envKey   string
		provider string
		model    string
	}{
		{"ANTHROPIC_API_KEY", "anthropic", "claude-sonnet-4-20250514"},
		{"OPENAI_API_KEY", "openai", "gpt-4o"},
		{"GOOGLE_API_KEY", "google", "gemini-2.0-flash"},
	}

	for _, p := range providers {
		key := os.Getenv(p.envKey)
		if key != "" {
			return llm.New(llm.Config{
				Service:  p.provider,
				Model:     p.model,
				APIKey:    key,
				MaxTokens: 4096,
			})
		}
	}

	return nil, fmt.Errorf("set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GOOGLE_API_KEY")
}
