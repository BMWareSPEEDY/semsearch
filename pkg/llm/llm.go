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

// LLMClient is the interface used at index time for description generation.
// The query path never touches it.
type LLMClient interface {
	Complete(ctx context.Context, prompt string, opts CompletionOpts) (string, error)
	IsAvailable() bool
}

type CompletionOpts struct {
	MaxTokens   int
	Temperature float64
}

// OllamaClient talks to a local Ollama server (default http://127.0.0.1:11434).
type OllamaClient struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

func NewOllamaClient(model string) *OllamaClient {
	return &OllamaClient{
		BaseURL: "http://127.0.0.1:11434",
		Model:   model,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *OllamaClient) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type ollamaGenerateRequest struct {
	Model   string           `json:"model"`
	Prompt  string           `json:"prompt"`
	Stream  bool             `json:"stream"`
	Think   *bool            `json:"think,omitempty"`
	Options ollamaGenOptions `json:"options"`
}

type ollamaGenOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

func (c *OllamaClient) Complete(ctx context.Context, prompt string, opts CompletionOpts) (string, error) {
	// Disable reasoning on thinking-capable models (e.g. qwen3.x) so the token
	// budget goes to the final description rather than internal "thinking" text.
	thinkOff := false
	reqBody := ollamaGenerateRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: false,
		Think:  &thinkOff,
		Options: ollamaGenOptions{
			Temperature: opts.Temperature,
			NumPredict:  opts.MaxTokens,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, truncate(body, 200))
	}

	var out ollamaGenerateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse ollama response: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama error: %s", out.Error)
	}

	return out.Response, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
