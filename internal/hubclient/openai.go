package hubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// probeOpenAI executes a probe against the openai /v1/chat/completions endpoint.
func (c *Client) probeOpenAI(ctx context.Context, baseURL, token, modelID string, streaming bool) Result {
	url := strings.TrimRight(baseURL, "/") + "/v1/chat/completions"

	payload := map[string]interface{}{
		"model":      modelID,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": probePrompt},
		},
	}
	if streaming {
		payload["stream"] = true
		payload["stream_options"] = map[string]bool{"include_usage": true}
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		msg := truncate("build request: " + err.Error())
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		msg := netErrorSummary("network error: ", err)
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg, LatencyMs: int(time.Since(start).Milliseconds())}
	}
	defer resp.Body.Close()

	if streaming {
		return c.readOpenAIStream(resp, start)
	}
	return c.readOpenAINonStream(resp, start)
}

// readOpenAINonStream parses a non-streaming openai response.
func (c *Client) readOpenAINonStream(resp *http.Response, start time.Time) Result {
	respBody, _ := io.ReadAll(resp.Body)
	latency := int(time.Since(start).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency}
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
		} `json:"usage"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency}
	}

	if len(parsed.Error) > 0 || len(parsed.Choices) == 0 {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency,
			InputTokens: parsed.Usage.PromptTokens, OutputTokens: parsed.Usage.CompletionTokens}
	}

	return Result{
		OK:           true,
		HTTPStatus:   resp.StatusCode,
		LatencyMs:    latency,
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
	}
}

// readOpenAIStream parses an openai SSE stream and measures TTFT.
func (c *Client) readOpenAIStream(resp *http.Response, start time.Time) Result {
	latency := func() int { return int(time.Since(start).Milliseconds()) }

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency()}
	}

	var ttft *int
	var inputTokens, outputTokens *int
	sawContent := false
	var streamErr string

	scanErr := scanSSE(resp.Body, func(data string) bool {
		if data == "[DONE]" {
			return false
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     *int `json:"prompt_tokens"`
				CompletionTokens *int `json:"completion_tokens"`
			} `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return true // skip unparseable chunks
		}

		if len(chunk.Error) > 0 {
			streamErr = extractUpstreamError([]byte(data))
			return false
		}

		if chunk.Usage != nil {
			if chunk.Usage.PromptTokens != nil {
				inputTokens = chunk.Usage.PromptTokens
			}
			if chunk.Usage.CompletionTokens != nil {
				outputTokens = chunk.Usage.CompletionTokens
			}
		}

		for _, ch := range chunk.Choices {
			// Reasoning models stream reasoning_content before (or instead
			// of) content; both count as first-token content.
			if ch.Delta.Content != "" || ch.Delta.ReasoningContent != "" {
				if ttft == nil {
					t := latency()
					ttft = &t
				}
				sawContent = true
			}
		}
		return true
	})

	total := latency()

	return finalizeStream(streamOutcome{
		statusCode:   resp.StatusCode,
		ttft:         ttft,
		inputTokens:  inputTokens,
		outputTokens: outputTokens,
		sawContent:   sawContent,
		streamErr:    streamErr,
		scanErr:      scanErr,
		totalMs:      total,
	})
}
