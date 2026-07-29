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

// imageProbePrompt is the fixed generation instruction sent on every
// images_generation probe. Each call generates a real image and costs money
// (spec 0014), so the prompt stays trivial; model-specific cost-saving
// parameters (e.g. quality:"low" for gpt-image models) arrive via the
// rule-merged imageParams argument (GH #33).
const imageProbePrompt = "A single solid teal square on a white background"

// imageDataItem is one entry of the images API data array.
type imageDataItem struct {
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
}

// imageDataComplete reports the success payload shape: the data array is
// non-empty and every item carries b64_json or url. A 200 with an empty or
// malformed payload means the generation path is effectively unavailable.
func imageDataComplete(data []imageDataItem) bool {
	if len(data) == 0 {
		return false
	}
	for _, item := range data {
		if item.B64JSON == "" && item.URL == "" {
			return false
		}
	}
	return true
}

// callImagesGeneration executes one POST /v1/images/generations call with the
// minimal request body (model + prompt + n=1, the most portable shape across
// upstreams) plus any rule-merged extra parameters (spec 0014 / GH #33, e.g.
// quality:"low" for gpt-image models). Success requires HTTP 2xx AND a
// complete data payload; upstream usage, when present, maps into the existing
// token fields. There is no streaming mode and no TTFT — LatencyMs carries
// the whole call.
func (c *Client) callImagesGeneration(ctx context.Context, baseURL, token, modelID string, imageParams map[string]string) Result {
	url := strings.TrimRight(baseURL, "/") + "/v1/images/generations"

	payload := map[string]interface{}{
		"model":  modelID,
		"prompt": imageProbePrompt,
		"n":      1,
	}
	for k, v := range imageParams {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		msg := truncate("build request: " + err.Error())
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	return c.doImageCall(req)
}

// doImageCall executes one prepared image-API request and maps the response
// into a Result. Shared by generations and edits: both return the same
// data/usage payload shape and share the success determination.
func (c *Client) doImageCall(req *http.Request) Result {
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		msg := netErrorSummary("network error: ", err)
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg, LatencyMs: int(time.Since(start).Milliseconds())}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	latency := int(time.Since(start).Milliseconds())

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency}
	}

	var parsed struct {
		Data  []imageDataItem `json:"data"`
		Usage *struct {
			InputTokens  *int `json:"input_tokens"`
			OutputTokens *int `json:"output_tokens"`
		} `json:"usage"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency}
	}

	var inputTokens, outputTokens *int
	if parsed.Usage != nil {
		inputTokens = parsed.Usage.InputTokens
		outputTokens = parsed.Usage.OutputTokens
	}

	if len(parsed.Error) > 0 || !imageDataComplete(parsed.Data) {
		msg := buildErrorSummary(resp.StatusCode, respBody)
		return Result{OK: false, HTTPStatus: resp.StatusCode, ErrorSummary: &msg, LatencyMs: latency,
			InputTokens: inputTokens, OutputTokens: outputTokens}
	}

	return Result{
		OK:           true,
		HTTPStatus:   resp.StatusCode,
		LatencyMs:    latency,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
}
