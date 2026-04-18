package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
// Analyze runs single issue analysis
func (a *AnthropicAnalyzer) Analyze(ctx context.Context, ic *IssueContext) (*Guidance, error) {
	results, err := a.AnalyzeMultiple(ctx, ic)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no guidance returned")
	}
	return results[0], nil
}

// AnalyzeMultiple runs analysis for all issues in one call
func (a *AnthropicAnalyzer) AnalyzeMultiple(ctx context.Context, ic *IssueContext) ([]*Guidance, error) {
	prompt := BuildPrompt(ic)

	reqBody := map[string]interface{}{
		"model":      a.model,
		"max_tokens": 2048,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		a.endpoint+"/v1/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse anthropic response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	return parseGuidanceArray(anthropicResp.Content[0].Text)
}
