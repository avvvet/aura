package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaAnalyzer implements Analyzer for local ollama
type OllamaAnalyzer struct {
	endpoint string
	model    string
}

// NewOllamaAnalyzer creates a new OllamaAnalyzer
func NewOllamaAnalyzer(endpoint, model string) *OllamaAnalyzer {
	return &OllamaAnalyzer{
		endpoint: endpoint,
		model:    model,
	}
}

// Name returns the provider name
func (o *OllamaAnalyzer) Name() string {
	return fmt.Sprintf("ollama/%s", o.model)
}

// Analyze sends the issue context to ollama and returns guidance
func (o *OllamaAnalyzer) Analyze(ctx context.Context, ic *IssueContext) (*Guidance, error) {
	prompt := BuildPrompt(ic)

	reqBody := map[string]interface{}{
		"model":  o.model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1, // low temperature for consistent structured output
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/generate", o.endpoint),
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call ollama: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// parse ollama response
	var ollamaResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse ollama response: %w", err)
	}

	return parseGuidance(ollamaResp.Response)
}

// parseGuidance parses the JSON guidance from LLM response
func parseGuidance(response string) (*Guidance, error) {
	// strip any markdown fences if present
	clean := strings.TrimSpace(response)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	// find JSON object
	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}
	clean = clean[start : end+1]

	var guidance Guidance
	if err := json.Unmarshal([]byte(clean), &guidance); err != nil {
		return nil, fmt.Errorf("failed to parse guidance JSON: %w", err)
	}

	return &guidance, nil
}
