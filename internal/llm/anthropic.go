package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicAnalyzer implements Analyzer for Anthropic
type AnthropicAnalyzer struct {
	endpoint string
	model    string
	apiKey   string
}

// NewAnthropicAnalyzer creates a new AnthropicAnalyzer
func NewAnthropicAnalyzer(endpoint, model, apiKey string) *AnthropicAnalyzer {
	return &AnthropicAnalyzer{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
	}
}

// Name returns the provider name
func (o *AnthropicAnalyzer) Name() string {
	return fmt.Sprintf("anthropic/%s", o.model)
}

// Analyze sends the issue context to Anthropic and returns guidance
func (o *AnthropicAnalyzer) Analyze(ctx context.Context, ic *IssueContext) (*Guidance, error) {
	prompt := BuildPrompt(ic)

	reqBody := map[string]interface{}{
		"model":      o.model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/v1/messages", o.endpoint),
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", o.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call anthropic: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse anthropic response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return nil, fmt.Errorf("no content in anthropic response")
	}

	return parseGuidance(anthropicResp.Content[0].Text)
}
