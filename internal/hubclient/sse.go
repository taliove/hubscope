package hubclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// extractUpstreamError tries to pull a human-readable message out of an upstream
// error body shaped like {"error": {"message": "..."}} or {"error": "..."}.
// Falls back to the raw body when no structured message is present.
func extractUpstreamError(body []byte) string {
	var structured struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &structured); err == nil && len(structured.Error) > 0 {
		// error may be an object with a message field
		var obj struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(structured.Error, &obj); err == nil && obj.Message != "" {
			return obj.Message
		}
		// error may be a plain string
		var str string
		if err := json.Unmarshal(structured.Error, &str); err == nil && str != "" {
			return str
		}
	}
	return string(body)
}

// buildErrorSummary combines HTTP status with the upstream message.
func buildErrorSummary(status int, body []byte) string {
	msg := extractUpstreamError(body)
	return truncate(fmt.Sprintf("HTTP %d: %s", status, strings.TrimSpace(msg)))
}

// sseEvent is a single parsed SSE data payload.
type sseEvent struct {
	data string
}

// scanSSE reads an SSE stream and invokes onData for each data: line.
// It stops when the reader is exhausted or onData returns false.
func scanSSE(r io.Reader, onData func(data string) bool) error {
	scanner := bufio.NewScanner(r)
	// Allow large SSE lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if !onData(data) {
			return nil
		}
	}
	return scanner.Err()
}
