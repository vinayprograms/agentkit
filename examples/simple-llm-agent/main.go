// Package main demonstrates the simplest possible agent using agentkit:
// create an LLM provider, register tools, and run an agentic loop.
//
// The agent reads user input, sends it to the LLM with tool definitions,
// executes any tool calls, feeds results back, and repeats until the LLM
// produces a final text response.
//
// Usage:
//
//	export ANTHROPIC_API_KEY=sk-...    # or OPENAI_API_KEY, GOOGLE_API_KEY
//	go run main.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vinayprograms/agentkit/llm"
)

func main() {
	provider, err := newProvider()
	if err != nil {
		log.Fatalf("Failed to create LLM provider: %v", err)
	}

	// Register custom tools — these are the capabilities the agent can use.
	customTools := []llm.ToolDef{
		calculatorToolDef(),
		currentTimeToolDef(),
	}

	fmt.Println("Simple LLM Agent (type 'quit' to exit)")
	fmt.Println("Try: \"What is 2^10 + 365?\" or \"What time is it?\"")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	var history []llm.Message

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

		// Add user message to conversation history.
		history = append(history, llm.Message{Role: "user", Content: input})

		// Run the agentic loop: send to LLM, handle tool calls, repeat.
		response, err := agentLoop(context.Background(), provider, history, customTools)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		// Add assistant response to history and print it.
		history = append(history, llm.Message{Role: "assistant", Content: response})
		fmt.Printf("\n%s\n\n", response)
	}
}

// agentLoop sends messages to the LLM and executes tool calls until the LLM
// produces a final response with no tool calls. This is the core pattern for
// building agents with agentkit.
func agentLoop(ctx context.Context, provider llm.Model, history []llm.Message, tools []llm.ToolDef) (string, error) {
	messages := make([]llm.Message, len(history))
	copy(messages, history)

	for i := 0; i < 10; i++ { // safety limit on iterations
		resp, err := provider.Chat(ctx, llm.ChatRequest{
			Messages:  messages,
			Tools:     tools,
			MaxTokens: 4096,
		})
		if err != nil {
			return "", fmt.Errorf("LLM chat failed: %w", err)
		}

		// No tool calls — the LLM is done, return the text response.
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// The LLM wants to call tools. Add its response (with tool calls) to
		// the message history, then execute each tool and add results.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			result := executeTool(tc.Name, tc.Args)
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("agent loop exceeded maximum iterations")
}

// executeTool dispatches a tool call to the appropriate handler.
func executeTool(name string, args map[string]interface{}) string {
	switch name {
	case "calculator":
		return executeCalculator(args)
	case "current_time":
		return executeCurrentTime(args)
	default:
		return fmt.Sprintf("unknown tool: %s", name)
	}
}

// --- Tool: calculator ---

func calculatorToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        "calculator",
		Description: "Evaluate a mathematical expression. Supports +, -, *, /, ^ (power), and parentheses via Go math.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"expression": map[string]interface{}{
					"type":        "string",
					"description": "A simple math expression, e.g. '2^10 + 365'",
				},
			},
			"required": []string{"expression"},
		},
	}
}

func executeCalculator(args map[string]interface{}) string {
	expr, _ := args["expression"].(string)
	if expr == "" {
		return "error: missing expression"
	}

	// Simple expression evaluator for demo purposes.
	// In production, use a proper expression parser.
	result, err := evalSimple(expr)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	// Format nicely: no decimal for integers.
	if result == math.Trunc(result) {
		return fmt.Sprintf("%.0f", result)
	}
	return fmt.Sprintf("%g", result)
}

// evalSimple handles expressions the LLM is likely to produce.
// It tokenizes and evaluates with proper operator precedence.
func evalSimple(expr string) (float64, error) {
	// Let the LLM format expressions and we parse individual operations.
	// For this demo, we handle "a op b" patterns via recursive splitting.
	expr = strings.TrimSpace(expr)

	// Try to parse as a plain number first.
	if val, err := strconv.ParseFloat(expr, 64); err == nil {
		return val, nil
	}

	// Handle parentheses by evaluating innermost first.
	for strings.Contains(expr, "(") {
		start := strings.LastIndex(expr, "(")
		end := strings.Index(expr[start:], ")") + start
		if end <= start {
			return 0, fmt.Errorf("mismatched parentheses")
		}
		inner, err := evalSimple(expr[start+1 : end])
		if err != nil {
			return 0, err
		}
		expr = expr[:start] + fmt.Sprintf("%g", inner) + expr[end+1:]
	}

	// Split by lowest-precedence operators first (+ and -), then * and /, then ^.
	// Addition/subtraction (left to right).
	if idx := findLastOp(expr, '+', '-'); idx > 0 {
		left, err := evalSimple(expr[:idx])
		if err != nil {
			return 0, err
		}
		right, err := evalSimple(expr[idx+1:])
		if err != nil {
			return 0, err
		}
		if expr[idx] == '+' {
			return left + right, nil
		}
		return left - right, nil
	}

	// Multiplication/division.
	if idx := findLastOp(expr, '*', '/'); idx > 0 {
		left, err := evalSimple(expr[:idx])
		if err != nil {
			return 0, err
		}
		right, err := evalSimple(expr[idx+1:])
		if err != nil {
			return 0, err
		}
		if expr[idx] == '*' {
			return left * right, nil
		}
		if right == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return left / right, nil
	}

	// Power.
	if idx := findLastOp(expr, '^', 0); idx > 0 {
		left, err := evalSimple(expr[:idx])
		if err != nil {
			return 0, err
		}
		right, err := evalSimple(expr[idx+1:])
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}

	return strconv.ParseFloat(strings.TrimSpace(expr), 64)
}

// findLastOp finds the last occurrence of op1 or op2 at the top level (not inside parens).
func findLastOp(expr string, op1, op2 byte) int {
	depth := 0
	lastIdx := -1
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && i > 0 && (expr[i] == op1 || (op2 != 0 && expr[i] == op2)) {
				lastIdx = i
			}
		}
	}
	return lastIdx
}

// --- Tool: current_time ---

func currentTimeToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        "current_time",
		Description: "Returns the current date and time in the specified timezone. Defaults to UTC.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"timezone": map[string]interface{}{
					"type":        "string",
					"description": "IANA timezone name, e.g. 'America/New_York'. Defaults to 'UTC'.",
				},
			},
		},
	}
}

func executeCurrentTime(args map[string]interface{}) string {
	tz, _ := args["timezone"].(string)
	if tz == "" {
		tz = "UTC"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Sprintf("error: unknown timezone %q", tz)
	}

	now := time.Now().In(loc)
	result := map[string]string{
		"datetime": now.Format(time.RFC3339),
		"timezone": tz,
	}
	b, _ := json.Marshal(result)
	return string(b)
}

// newProvider creates an LLM provider from environment variables.
func newProvider() (llm.Model, error) {
	// Try providers in order of preference.
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
