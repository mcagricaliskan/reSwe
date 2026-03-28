package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cagri/reswe/internal/models"
)

type OllamaProvider struct {
	baseURL string
	client  *http.Client
}

func NewOllama(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (o *OllamaProvider) Name() string {
	return "ollama"
}

type ollamaChatReq struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResp struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Model string `json:"model"`
}

func (o *OllamaProvider) Chat(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
	var full strings.Builder
	resp, err := o.ChatStream(ctx, req, func(chunk string) {
		full.WriteString(chunk)
	})
	if err != nil {
		return nil, err
	}
	resp.Content = full.String()
	return resp, nil
}

func (o *OllamaProvider) ChatStream(ctx context.Context, req models.ChatRequest, cb StreamCallback) (*models.ChatResponse, error) {
	messages := make([]ollamaChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = ollamaChatMessage{Role: m.Role, Content: m.Content}
	}

	body := ollamaChatReq{
		Model:    req.Model,
		Messages: messages,
		Stream:   true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", httpResp.StatusCode, string(errBody))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(httpResp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk ollamaChatResp
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		if chunk.Message.Content != "" {
			fullContent.WriteString(chunk.Message.Content)
			if cb != nil {
				cb(chunk.Message.Content)
			}
		}

		if chunk.Done {
			return &models.ChatResponse{
				Content: fullContent.String(),
				Done:    true,
				Model:   chunk.Model,
			}, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}

	return &models.ChatResponse{
		Content: fullContent.String(),
		Done:    true,
		Model:   req.Model,
	}, nil
}

func (o *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	names := make([]string, len(result.Models))
	for i, m := range result.Models {
		names[i] = m.Name
	}
	return names, nil
}

func (o *OllamaProvider) Ping(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cannot reach ollama at %s: %w", o.baseURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	return nil
}
