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

// OpenAIAnalyzer implements Analyzer for OpenAI
type OpenAIAnalyzer struct {
	endpoint string
	model    string
	apiKey   string
}

// NewOpenAIAnalyzer creates a new OpenAIAnalyzer
func NewOpenAIAnalyzer(endpoint, model, apiKey string) *OpenAIAnalyzer {
	return &OpenAIAnalyzer{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
	}
}

// Name returns the provider name
func (o *OpenAIAnalyzer) Name() string {
	return fmt.Sprintf("openai/%s", o.model)
}

// Analyze sends the issue context to OpenAI and returns guidance
func (o *OpenAIAnalyzer) Analyze(ctx context.Context, ic *IssueContext) (*Guidance, error) {
	prompt := BuildPrompt(ic)

	reqBody := map[string]interface{}{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature":     0.1,
		"response_format": map[string]string{"type": "json_object"},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/chat/completions", o.endpoint),
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call openai: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to parse openai response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in openai response")
	}

	return parseGuidance(openaiResp.Choices[0].Message.Content)
}
