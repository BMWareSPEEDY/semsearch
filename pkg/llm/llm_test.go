package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ashishpaliwal/semsearch/pkg/llm"
)

func TestOllamaClientComplete(t *testing.T) {
	var gotBody ollamaBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if gotBody.Stream {
			t.Errorf("expected stream=false")
		}
		if gotBody.Options.Temperature != 0 {
			t.Errorf("expected temperature 0, got %f", gotBody.Options.Temperature)
		}
		if gotBody.Options.NumPredict != 60 {
			t.Errorf("expected num_predict 60, got %d", gotBody.Options.NumPredict)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"response": "serves static assets from disk"})
	}))
	defer srv.Close()

	client := llm.NewOllamaClient("qwen3.5:0.8b")
	client.BaseURL = srv.URL

	desc, err := client.Complete(context.Background(), "describe the code", llm.CompletionOpts{MaxTokens: 60, Temperature: 0})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if strings.TrimSpace(desc) != "serves static assets from disk" {
		t.Errorf("expected canned response, got %q", desc)
	}
	if gotBody.Model != "qwen3.5:0.8b" {
		t.Errorf("expected model qwen3.5:0.8b, got %s", gotBody.Model)
	}
}

func TestOllamaClientIsAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := llm.NewOllamaClient("qwen3.5:0.8b")
	client.BaseURL = srv.URL
	if !client.IsAvailable() {
		t.Errorf("expected IsAvailable true against healthy server")
	}
}

func TestOllamaClientUnavailable(t *testing.T) {
	client := llm.NewOllamaClient("qwen3.5:0.8b")
	client.BaseURL = "http://127.0.0.1:1" // closed port
	if client.IsAvailable() {
		t.Errorf("expected IsAvailable false against unreachable server")
	}
}

type ollamaBody struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Stream  bool   `json:"stream"`
	Options struct {
		Temperature float64 `json:"temperature"`
		NumPredict  int     `json:"num_predict"`
	} `json:"options"`
}
