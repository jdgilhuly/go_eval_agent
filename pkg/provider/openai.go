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
	defaultOpenAIURL = "https://api.openai.com/v1/chat/completions"
)

// OpenAIOption configures an OpenAIProvider.
type OpenAIOption func(*OpenAIProvider)

// WithOpenAIHTTPClient sets a custom HTTP client (useful for testing).
func WithOpenAIHTTPClient(c *http.Client) OpenAIOption {
	return func(p *OpenAIProvider) { p.client = c }
}

// WithOpenAIBaseURL overrides the OpenAI API base URL.
func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(p *OpenAIProvider) { p.baseURL = url }
}

// WithOpenAIMaxRetries sets the maximum number of retry attempts.
func WithOpenAIMaxRetries(n int) OpenAIOption {
	return func(p *OpenAIProvider) { p.maxRetries = n }
}

// OpenAIProvider implements Provider for the OpenAI Chat Completions API.
type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	client     *http.Client
	maxRetries int
}

// NewOpenAIProvider creates a new OpenAI provider with the given API key.
func NewOpenAIProvider(apiKey string, opts ...OpenAIOption) *OpenAIProvider {
	p := &OpenAIProvider{
		apiKey:     apiKey,
		baseURL:    defaultOpenAIURL,
		client:     &http.Client{Timeout: 60 * time.Second},
		maxRetries: defaultMaxRetries,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns "openai".
func (p *OpenAIProvider) Name() string { return "openai" }

// openaiRequest is the OpenAI Chat Completions API request body.
type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	Tools       []openaiTool    `json:"tools,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
}

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiCallFunction `json:"function"`
}

type openaiCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openaiResponse is the OpenAI Chat Completions API response body.
type openaiResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Choices []openaiChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type openaiChoice struct {
	Index        int           `json:"index"`
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Complete sends a request to the OpenAI Chat Completions API.
func (p *OpenAIProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
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

	return nil, fmt.Errorf("openai API request failed after %d attempts: %w", p.maxRetries+1, lastErr)
}

func (p *OpenAIProvider) buildRequestBody(req *Request) ([]byte, error) {
	or := openaiRequest{
		Model:    req.Model,
		Messages: convertToOpenAIMessages(req.System, req.Messages),
	}

	if req.Temperature != 0 {
		t := req.Temperature
		or.Temperature = &t
	}

	if req.MaxTokens != 0 {
		m := req.MaxTokens
		or.MaxTokens = &m
	}

	for _, tool := range req.Tools {
		or.Tools = append(or.Tools, openaiTool{
			Type: "function",
			Function: openaiFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return json.Marshal(or)
}

func convertToOpenAIMessages(system string, msgs []Message) []openaiMessage {
	out := make([]openaiMessage, 0, len(msgs)+1)

	// OpenAI uses a system message in the messages array.
	if system != "" {
		s := system
		out = append(out, openaiMessage{Role: "system", Content: &s})
	}

	for _, m := range msgs {
		om := openaiMessage{Role: m.Role}

		if m.Content != "" {
			c := m.Content
			om.Content = &c
		}

		if m.Role == "tool" {
			om.ToolCallID = m.ToolCallID
		}

		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Parameters)
				om.ToolCalls = append(om.ToolCalls, openaiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openaiCallFunction{
						Name:      tc.Name,
						Arguments: string(args),
					},
				})
			}
		}

		out = append(out, om)
	}
	return out
}

func (p *OpenAIProvider) doRequest(ctx context.Context, body []byte) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

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
		var apiErr openaiErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, &retryableError{err: fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, apiErr.Error.Message)}
		}
		return nil, &retryableError{err: fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))}
	}

	if httpResp.StatusCode != http.StatusOK {
		var apiErr openaiErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var or openaiResponse
	if err := json.Unmarshal(respBody, &or); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return parseOpenAIResponse(&or), nil
}

func parseOpenAIResponse(or *openaiResponse) *Response {
	resp := &Response{
		Usage: Usage{
			InputTokens:  or.Usage.PromptTokens,
			OutputTokens: or.Usage.CompletionTokens,
		},
	}

	if len(or.Choices) == 0 {
		return resp
	}

	choice := or.Choices[0]
	resp.StopReason = choice.FinishReason

	if choice.Message.Content != nil {
		resp.Content = *choice.Message.Content
	}

	for _, tc := range choice.Message.ToolCalls {
		var params map[string]interface{}
		json.Unmarshal([]byte(tc.Function.Arguments), &params)
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:         tc.ID,
			Name:       tc.Function.Name,
			Parameters: params,
		})
	}

	return resp
}
