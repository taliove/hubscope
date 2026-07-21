package hubclient

import (
	"context"
	"net/http"
	"time"
)

// probePrompt is the fixed user message sent on every probe
const probePrompt = "Reply with the single word: pong"

// maxTokens is the fixed max_tokens for probe requests
const maxTokens = 16

// requestTimeout is the per-request timeout
const requestTimeout = 60 * time.Second

// errorSummaryLimit truncates error summaries to this many characters
const errorSummaryLimit = 500

// Result is the unified probe result returned regardless of protocol or mode.
type Result struct {
	OK           bool
	HTTPStatus   int
	ErrorSummary *string
	LatencyMs    int
	TTFTMs       *int // only populated for streaming probes
	InputTokens  *int
	OutputTokens *int
}

// Client executes probe requests against a Hub.
type Client struct {
	httpClient *http.Client
}

// New creates a Client with the standard probe timeout.
func New() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

// Probe executes a single probe request against the hub using the given
// protocol and model ID. When streaming is true it opens an SSE stream and
// measures TTFT.
func (c *Client) Probe(ctx context.Context, baseURL, token, protocol, modelID string, streaming bool) Result {
	switch protocol {
	case "anthropic":
		return c.probeAnthropic(ctx, baseURL, token, modelID, streaming)
	case "openai":
		return c.probeOpenAI(ctx, baseURL, token, modelID, streaming)
	default:
		msg := "unknown protocol: " + protocol
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
}

// stringPtr returns a pointer to s.
func stringPtr(s string) *string {
	return &s
}

// intPtr returns a pointer to i.
func intPtr(i int) *int {
	return &i
}

// truncate limits s to errorSummaryLimit characters (runes), never splitting
// a multi-byte UTF-8 sequence — upstream error messages may be Chinese.
func truncate(s string) string {
	runes := []rune(s)
	if len(runes) > errorSummaryLimit {
		return string(runes[:errorSummaryLimit])
	}
	return s
}

// streamOutcome carries the parsed state of an SSE stream into finalizeStream.
type streamOutcome struct {
	statusCode   int
	ttft         *int
	inputTokens  *int
	outputTokens *int
	sawContent   bool
	streamErr    string
	scanErr      error
	totalMs      int
}

// finalizeStream converts parsed SSE stream state into a Result. Shared by
// both protocols; only the event parsing differs between them.
func finalizeStream(o streamOutcome) Result {
	result := Result{
		HTTPStatus:   o.statusCode,
		LatencyMs:    o.totalMs,
		TTFTMs:       o.ttft,
		InputTokens:  o.inputTokens,
		OutputTokens: o.outputTokens,
	}
	switch {
	case o.streamErr != "":
		msg := truncate("HTTP 200: " + o.streamErr)
		result.ErrorSummary = &msg
	case o.scanErr != nil:
		msg := truncate("stream read error: " + o.scanErr.Error())
		result.ErrorSummary = &msg
	case !o.sawContent:
		msg := "HTTP 200: stream produced no content"
		result.ErrorSummary = &msg
	default:
		result.OK = true
	}
	return result
}
