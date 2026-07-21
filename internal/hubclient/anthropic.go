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

// callAnthropic executes a request against the anthropic /v1/messages
// endpoint with the given prompt and token budget.
func (c *Client) callAnthropic(ctx context.Context, baseURL, token, modelID, prompt string, maxTok int, streaming bool) Result {
	url := strings.TrimRight(baseURL, "/") + "/v1/messages"

	payload := map[string]interface{}{
		"model":      modelID,
		"max_tokens": maxTok,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if streaming {
		payload["stream"] = true
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		msg := truncate("build request: " + err.Error())
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", token)
	req.Header.Set("anthropic-version", "2023-06-01")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		msg := netErrorSummary("network error: ", err)
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg, LatencyMs: int(time.Since(start).Milliseconds())}
	}
	defer resp.Body.Close()

	if streaming {
		return c.readAnthropicStream(resp, start)
	}
	return c.readAnthropicNonStream(resp, start)
}

// readAnthropicNonStream parses a non-streaming anthropic response.
func (c *Client) readAnthropicNonStream(resp *http.Response, start time.Time) Result {
	respBody, _ := io.ReadAll(resp.Body)
	latency := int(time.Since(start).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency}
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  *int `json:"input_tokens"`
			OutputTokens *int `json:"output_tokens"`
		} `json:"usage"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency}
	}

	// A 200 body may still carry an error field.
	if len(parsed.Error) > 0 {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency,
			InputTokens: parsed.Usage.InputTokens, OutputTokens: parsed.Usage.OutputTokens}
	}

	// Concatenate all text blocks into the answer text.
	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	return Result{
		OK:           true,
		HTTPStatus:   resp.StatusCode,
		LatencyMs:    latency,
		InputTokens:  parsed.Usage.InputTokens,
		OutputTokens: parsed.Usage.OutputTokens,
		Text:         text.String(),
	}
}

// readAnthropicStream parses an anthropic SSE stream and measures TTFT.
func (c *Client) readAnthropicStream(resp *http.Response, start time.Time) Result {
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
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"delta"`
			Message struct {
				Usage struct {
					InputTokens  *int `json:"input_tokens"`
					OutputTokens *int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				InputTokens  *int `json:"input_tokens"`
				OutputTokens *int `json:"output_tokens"`
			} `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return true // skip unparseable events
		}

		if len(event.Error) > 0 {
			streamErr = extractUpstreamError([]byte(data))
			return false
		}

		// message_start carries input tokens
		if event.Message.Usage.InputTokens != nil {
			inputTokens = event.Message.Usage.InputTokens
		}
		if event.Message.Usage.OutputTokens != nil {
			outputTokens = event.Message.Usage.OutputTokens
		}
		// message_delta carries final output tokens
		if event.Usage.OutputTokens != nil {
			outputTokens = event.Usage.OutputTokens
		}
		if event.Usage.InputTokens != nil {
			inputTokens = event.Usage.InputTokens
		}

		// First content-bearing event marks TTFT. Reasoning models emit
		// thinking deltas before (or instead of) text deltas; both count.
		if event.Type == "content_block_delta" && (event.Delta.Text != "" || event.Delta.Thinking != "") {
			if ttft == nil {
				t := latency()
				ttft = &t
			}
			sawContent = true
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
