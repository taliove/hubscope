package hubclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"time"
)

// probePrompt is the fixed user message sent on every probe
const probePrompt = "Reply with the single word: pong"

// maxTokens is the fixed max_tokens for probe requests
const maxTokens = 16

// requestTimeout is the per-request timeout for chat-protocol calls.
const requestTimeout = 60 * time.Second

// ImageRequestTimeout is the per-request timeout for image-protocol calls:
// generation queues and cold starts make 10-60s responses normal, so the
// chat budget would misread a slow-but-healthy path as down (spec 0014).
const ImageRequestTimeout = 180 * time.Second

// ProtocolImagesGeneration is the OpenAI-compatible image generation
// protocol (POST /v1/images/generations, spec 0014 / ADR 0012).
const ProtocolImagesGeneration = "images_generation"

// IsImageProtocol reports whether the protocol belongs to the image family:
// one call per probe round, no streaming or TTFT, and the image request
// budget. images_edit joins the family in a later ticket.
func IsImageProtocol(protocol string) bool {
	return protocol == ProtocolImagesGeneration
}

// errorSummaryLimit truncates error summaries to this many characters
const errorSummaryLimit = 500

// Result is the unified result returned regardless of protocol or mode.
// Text carries the assistant's answer for non-streaming calls; it is left
// empty for streaming probes, which only measure availability.
type Result struct {
	OK           bool
	HTTPStatus   int
	ErrorSummary *string
	LatencyMs    int
	TTFTMs       *int // only populated for streaming probes
	InputTokens  *int
	OutputTokens *int
	Text         string
}

// Client executes probe requests against a Hub.
type Client struct {
	httpClient *http.Client
	// timeout is the per-call budget for chat protocols; image protocols
	// always use ImageRequestTimeout instead (see callTimeout).
	timeout time.Duration
}

// New creates a Client with the standard chat probe timeout. Image-protocol
// calls get ImageRequestTimeout regardless of construction.
func New() *Client {
	return NewWithTimeout(requestTimeout)
}

// NewWithTimeout creates a Client with a custom per-request timeout for chat
// protocols.
func NewWithTimeout(d time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: probeTransport(),
		},
		timeout: d,
	}
}

// callTimeout returns the per-call budget: image protocols always get
// ImageRequestTimeout (the client-level timeout would cut a healthy
// generation off at 60s), chat protocols use the client's configured value.
// Every public method wraps its context with this budget — "no unbounded
// exit" is a construction property of the Client, so a hung hub can never
// park a caller (e.g. the discovery in-flight guard) forever.
func (c *Client) callTimeout(protocol string) time.Duration {
	if IsImageProtocol(protocol) {
		return ImageRequestTimeout
	}
	return c.timeout
}

// probeTransport mirrors http.DefaultTransport with explicit proxy handling:
// HTTP_PROXY/HTTPS_PROXY/NO_PROXY are honored, which matters on machines
// running fake-ip local proxies (direct DNS answers are unroutable there).
func probeTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// Probe executes a single probe request against the hub using the given
// protocol and model ID. When streaming is true it opens an SSE stream and
// measures TTFT. Probe semantics (fixed prompt, tiny token budget) are
// unchanged; it simply delegates to call with the probe constants.
func (c *Client) Probe(ctx context.Context, baseURL, token, protocol, modelID string, streaming bool) Result {
	ctx, cancel := context.WithTimeout(ctx, c.callTimeout(protocol))
	defer cancel()
	return c.call(ctx, baseURL, token, protocol, modelID, probePrompt, maxTokens, streaming)
}

// Complete executes a single non-streaming completion with a custom prompt
// and token budget. Used by the evaluator, where answers need room to be
// complete (unlike the 16-token probe). The per-request timeout comes from
// the client construction (see NewWithTimeout).
func (c *Client) Complete(ctx context.Context, baseURL, token, protocol, modelID, prompt string, maxTok int) Result {
	ctx, cancel := context.WithTimeout(ctx, c.callTimeout(protocol))
	defer cancel()
	return c.call(ctx, baseURL, token, protocol, modelID, prompt, maxTok, false)
}

// call dispatches to the protocol-specific implementation. Callers (Probe,
// Complete) apply the protocol-family timeout budget before reaching it.
func (c *Client) call(ctx context.Context, baseURL, token, protocol, modelID, prompt string, maxTok int, streaming bool) Result {
	switch protocol {
	case "anthropic":
		return c.callAnthropic(ctx, baseURL, token, modelID, prompt, maxTok, streaming)
	case "openai":
		return c.callOpenAI(ctx, baseURL, token, modelID, prompt, maxTok, streaming)
	case ProtocolImagesGeneration:
		// Image calls have no streaming mode and use their own fixed prompt;
		// the chat prompt/token arguments do not apply.
		return c.callImagesGeneration(ctx, baseURL, token, modelID)
	default:
		msg := "unknown protocol: " + protocol
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
}

// netErrorSummary builds an error summary for transport-level failures.
// Timeouts are prefixed with "timeout:" so they stay recognizable in stored
// error summaries; other failures keep the given prefix.
func netErrorSummary(prefix string, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return truncate("timeout: " + err.Error())
	}
	return truncate(prefix + err.Error())
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
		msg := netErrorSummary("stream read error: ", o.scanErr)
		result.ErrorSummary = &msg
	case !o.sawContent:
		msg := "HTTP 200: stream produced no content"
		result.ErrorSummary = &msg
	default:
		result.OK = true
	}
	return result
}
