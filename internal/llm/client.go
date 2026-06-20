package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Result struct {
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int
}

type Client struct {
	cfg        config.PolzaConfig
	httpClient *http.Client
}

func NewClient(cfg config.PolzaConfig) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) Complete(ctx context.Context, messages []Message) (Result, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return Result{}, errors.New("POLZA_API_KEY is not configured")
	}

	var lastErr error
	attempts := c.cfg.RetryCount + 1
	for attempt := 0; attempt < attempts; attempt++ {
		result, err := c.completeOnce(ctx, messages)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt < attempts-1 {
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			}
		}
	}
	return Result{}, lastErr
}

func (c *Client) completeOnce(ctx context.Context, messages []Message) (Result, error) {
	payload := map[string]any{
		"model":       c.cfg.Model,
		"messages":    messages,
		"temperature": c.cfg.Temperature,
		"max_tokens":  c.cfg.MaxTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}

	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("polza http %d: %s", resp.StatusCode, string(data))
	}

	var response struct {
		Model   string `json:"model"`
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return Result{}, err
	}
	if len(response.Choices) == 0 {
		return Result{}, errors.New("polza returned no choices")
	}

	text := strings.TrimSpace(response.Choices[0].Message.Content)
	if text == "" {
		return Result{}, errors.New("polza returned an empty response")
	}

	model := response.Model
	if model == "" {
		model = c.cfg.Model
	}
	return Result{
		Text:         text,
		Model:        model,
		InputTokens:  response.Usage.PromptTokens,
		OutputTokens: response.Usage.CompletionTokens,
	}, nil
}
