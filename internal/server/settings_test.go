package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestSettingsRoundTrip covers GET defaults and partial PUT updates.
func TestSettingsRoundTrip(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	getSettings := func() map[string]interface{} {
		resp := doGet(t, ts.URL+"/api/settings")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get settings: expected 200, got %d", resp.StatusCode)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		var settings map[string]interface{}
		if err := json.Unmarshal(env.Data, &settings); err != nil {
			t.Fatalf("unmarshal settings: %v", err)
		}
		return settings
	}

	defaults := getSettings()
	if defaults["lark_webhook_url"].(string) != "" {
		t.Errorf("default webhook should be empty, got %v", defaults["lark_webhook_url"])
	}
	if defaults["alert_enabled"].(bool) != true {
		t.Errorf("default alert_enabled should be true, got %v", defaults["alert_enabled"])
	}
	if defaults["score_drop_alert_enabled"].(bool) != true {
		t.Errorf("default score_drop_alert_enabled should be true, got %v", defaults["score_drop_alert_enabled"])
	}
	if defaults["judge_model"].(string) != "claude-opus-4-8" {
		t.Errorf("default judge_model should be claude-opus-4-8, got %v", defaults["judge_model"])
	}
	if defaults["default_sample_count"].(float64) != 1 {
		t.Errorf("default default_sample_count should be 1, got %v", defaults["default_sample_count"])
	}

	// Partial update: only the webhook URL changes.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": "https://open.feishu.cn/example-hook",
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}
	after := getSettings()
	if after["lark_webhook_url"].(string) != "https://open.feishu.cn/example-hook" {
		t.Errorf("webhook not updated: %v", after["lark_webhook_url"])
	}
	if after["alert_enabled"].(bool) != true {
		t.Errorf("alert_enabled should stay true, got %v", after["alert_enabled"])
	}

	// Partial update of the remaining keys. judge_model must name a model
	// registered on some hub (GH #155 save-time guard) — seed one first.
	kimiHub, err := db.CreateHub("kimi-hub", "http://kimi.test", "tok-kimi-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	if _, err := db.CreateModel(kimiHub.ID, "kimi-k3", []string{"openai"}); err != nil {
		t.Fatalf("create judge model: %v", err)
	}
	putResp = doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"alert_enabled":            false,
		"score_drop_alert_enabled": false,
		"judge_model":              "kimi-k3",
		"default_sample_count":     3,
	})
	putResp.Body.Close()
	after = getSettings()
	if after["alert_enabled"].(bool) != false {
		t.Errorf("alert_enabled not updated: %v", after["alert_enabled"])
	}
	if after["score_drop_alert_enabled"].(bool) != false {
		t.Errorf("score_drop_alert_enabled not updated: %v", after["score_drop_alert_enabled"])
	}
	if after["judge_model"].(string) != "kimi-k3" {
		t.Errorf("judge_model not updated: %v", after["judge_model"])
	}
	if after["default_sample_count"].(float64) != 3 {
		t.Errorf("default_sample_count not updated: %v", after["default_sample_count"])
	}

	// Out-of-range sample counts are rejected.
	for _, bad := range []int{0, 11} {
		badResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
			"default_sample_count": bad,
		})
		badResp.Body.Close()
		if badResp.StatusCode != http.StatusBadRequest {
			t.Errorf("default_sample_count=%d: expected 400, got %d", bad, badResp.StatusCode)
		}
	}

	// Writes require a session: an anonymous PUT is rejected.
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings",
		strings.NewReader(`{"alert_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	anonResp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("anonymous PUT: %v", err)
	}
	anonResp.Body.Close()
	if anonResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous PUT: expected 401, got %d", anonResp.StatusCode)
	}
}
