package policy

import (
	"context"

	"github.com/vinayprograms/agentkit/llm"
)

// chatProviderAdapter wraps an llm.Provider to satisfy the LLMProvider interface.
type chatProviderAdapter struct {
	provider llm.Provider
}

// LLMProviderFromChatProvider wraps an llm.Provider as a policy.LLMProvider.
// This avoids the need for consumers to write their own adapter between
// the full chat interface and the simpler generate interface used for policy checks.
func LLMProviderFromChatProvider(provider llm.Provider) LLMProvider {
	return &chatProviderAdapter{provider: provider}
}

func (a *chatProviderAdapter) Generate(ctx context.Context, prompt string) (*GenerateResult, error) {
	resp, err := a.provider.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return nil, err
	}
	return &GenerateResult{
		Content:      resp.Content,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
	}, nil
}
