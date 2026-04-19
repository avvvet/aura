package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaAnalyzer implements Analyzer for local ollama
type OllamaAnalyzer struct {
	endpoint string
	model    string
}

// NewOllamaAnalyzer creates a new OllamaAnalyzer
func NewOllamaAnalyzer(endpoint, model string) *OllamaAnalyzer {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "mistral"
	}
	return &OllamaAnalyzer{endpoint: endpoint, model: model}
}

// Name returns the provider name
func (o *OllamaAnalyzer) Name() string {
	return fmt.Sprintf("ollama/%s", o.model)
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

// Analyze runs LLM analysis for a single issue (legacy)
func (o *OllamaAnalyzer) Analyze(ctx context.Context, ic *IssueContext) (*Guidance, error) {
	results, err := o.AnalyzeMultiple(ctx, ic)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no guidance returned")
	}
	return results[0], nil
}

// AnalyzeMultiple runs LLM analysis for all issues in one call
func (o *OllamaAnalyzer) AnalyzeMultiple(ctx context.Context, ic *IssueContext) ([]*Guidance, error) {
	prompt := BuildPrompt(ic)
	reqBody := ollamaRequest{
		Model:  o.model,
		Prompt: prompt,
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		o.endpoint+"/api/generate",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse ollama response: %w", err)
	}

	return parseGuidanceArray(ollamaResp.Response)
}

// parseGuidanceArray parses LLM response as array of guidance
func parseGuidanceArray(response string) ([]*Guidance, error) {
	clean := strings.TrimSpace(response)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	// find JSON array
	start := strings.Index(clean, "[")
	end := strings.LastIndex(clean, "]")
	if start == -1 || end == -1 {
		// fallback: try single object
		start = strings.Index(clean, "{")
		end = strings.LastIndex(clean, "}")
		if start == -1 || end == -1 {
			return nil, fmt.Errorf("no JSON found in response")
		}
		clean = clean[start : end+1]
		var g Guidance
		if err := json.Unmarshal([]byte(clean), &g); err != nil {
			return nil, fmt.Errorf("failed to parse guidance: %w", err)
		}
		return []*Guidance{&g}, nil
	}

	clean = clean[start : end+1]
	var guidances []*Guidance
	if err := json.Unmarshal([]byte(clean), &guidances); err != nil {
		return nil, fmt.Errorf("failed to parse guidance array: %w", err)
	}

	return guidances, nil
}
