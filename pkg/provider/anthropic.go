package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

const (
	defaultAnthropicURL    = "https://api.anthropic.com/v1/messages"
	defaultAnthropicVersion = "2023-06-01"
	defaultMaxRetries      = 3
	baseBackoff            = 500 * time.Millisecond
)

// AnthropicOption configures an AnthropicProvider.
type AnthropicOption func(*AnthropicProvider)

// WithHTTPClient sets a custom HTTP client (useful for testing).
func WithHTTPClient(c *http.Client) AnthropicOption {
	return func(p *AnthropicProvider) { p.client = c }
}

// WithBaseURL overrides the Anthropic API base URL.
func WithBaseURL(url string) AnthropicOption {
	return func(p *AnthropicProvider) { p.baseURL = url }
}

// WithMaxRetries sets the maximum number of retry attempts for retryable errors.
func WithMaxRetries(n int) AnthropicOption {
	return func(p *AnthropicProvider) { p.maxRetries = n }
}

// AnthropicProvider implements Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey     string
	baseURL    string
	client     *http.Client
	maxRetries int
}

// NewAnthropicProvider creates a new Anthropic provider with the given API key.
func NewAnthropicProvider(apiKey string, opts ...AnthropicOption) *AnthropicProvider {
	p := &AnthropicProvider{
		apiKey:     apiKey,
		baseURL:    defaultAnthropicURL,
		client:     &http.Client{Timeout: 60 * time.Second},
		maxRetries: defaultMaxRetries,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns "anthropic".
func (p *AnthropicProvider) Name() string { return "anthropic" }

// anthropicRequest is the Anthropic Messages API request body.
type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// anthropicResponse is the Anthropic Messages API response body.
type anthropicResponse struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Role       string                 `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                 `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends a request to the Anthropic Messages API.
func (p *AnthropicProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("building request body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := baseBackoff * time.Duration(math.Pow(2, float64(attempt-1)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := p.doRequest(ctx, body)
		if err != nil {
			if !isRetryable(err) {
				return nil, err
			}
			lastErr = err
			continue
		}
		return resp, nil
	}

	return nil, fmt.Errorf("anthropic API request failed after %d attempts: %w", p.maxRetries+1, lastErr)
}

func (p *AnthropicProvider) buildRequestBody(req *Request) ([]byte, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	ar := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    req.System,
		Messages:  convertMessages(req.Messages),
	}

	if req.Temperature != 0 {
		t := req.Temperature
		ar.Temperature = &t
	}

	for _, tool := range req.Tools {
		ar.Tools = append(ar.Tools, anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		})
	}

	return json.Marshal(ar)
}

func convertMessages(msgs []Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		am := anthropicMessage{Role: m.Role}

		if m.Role == "tool" {
			// Tool result messages use structured content for Anthropic.
			am.Role = "user"
			am.Content = []map[string]interface{}{
				{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				},
			}
		} else if len(m.ToolCalls) > 0 {
			// Assistant messages with tool calls use content blocks.
			blocks := make([]map[string]interface{}, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Parameters,
				})
			}
			am.Content = blocks
		} else {
			am.Content = m.Content
		}

		out = append(out, am)
	}
	return out
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body []byte) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", p.apiKey)
	httpReq.Header.Set("Anthropic-Version", defaultAnthropicVersion)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("sending HTTP request: %w", err)}
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("reading response body: %w", err)}
	}

	if httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= 500 {
		var apiErr anthropicErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, &retryableError{err: fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, apiErr.Error.Message)}
		}
		return nil, &retryableError{err: fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))}
	}

	if httpResp.StatusCode != http.StatusOK {
		var apiErr anthropicErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var ar anthropicResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return parseAnthropicResponse(&ar), nil
}

func parseAnthropicResponse(ar *anthropicResponse) *Response {
	resp := &Response{
		StopReason: ar.StopReason,
		Usage: Usage{
			InputTokens:  ar.Usage.InputTokens,
			OutputTokens: ar.Usage.OutputTokens,
		},
	}

	var textParts []byte
	for _, block := range ar.Content {
		switch block.Type {
		case "text":
			if len(textParts) > 0 {
				textParts = append(textParts, '\n')
			}
			textParts = append(textParts, block.Text...)
		case "tool_use":
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:         block.ID,
				Name:       block.Name,
				Parameters: block.Input,
			})
		}
	}
	resp.Content = string(textParts)

	return resp
}

// retryableError wraps errors that should trigger a retry.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// isRetryable returns true if the error should trigger a retry.
func isRetryable(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}
