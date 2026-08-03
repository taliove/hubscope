package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/taliove/hubscope/internal/store"
)

// TestModelRegistryOverridesSettings covers the spec-0020 ticket-1 settings
// contract at the W1 seam: default empty list, validated round-trip,
// rejection of invalid entries, clearing via [], and fail-open reads of a
// corrupt stored value.
func TestModelRegistryOverridesSettings(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	getOverrides := func() []interface{} {
		resp := doGet(t, ts.URL+"/api/settings")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get settings: expected 200, got %d", resp.StatusCode)
		}
		var body struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		ovs, ok := body.Data["model_registry_overrides"].([]interface{})
		if !ok {
			t.Fatalf("model_registry_overrides should be an array, got %T", body.Data["model_registry_overrides"])
		}
		return ovs
	}

	// Default: empty list (never null).
	if got := getOverrides(); len(got) != 0 {
		t.Fatalf("default overrides should be empty, got %v", got)
	}

	// Valid entries round-trip.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"model_registry_overrides": []map[string]interface{}{
			{"match": "deepseek-v3*", "price_in": 0.5},
			{"match": "my-local-model", "iq_tier": 6, "price_in": 0, "price_out": 0},
		},
	})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put overrides: expected 200, got %d", putResp.StatusCode)
	}
	if got := getOverrides(); len(got) != 2 {
		t.Fatalf("overrides not persisted: %v", got)
	}

	// Invalid entries are rejected and leave the stored value unchanged.
	for name, bad := range map[string]interface{}{
		"iq out of range": []map[string]interface{}{{"match": "a", "iq_tier": 11}},
		"negative price":  []map[string]interface{}{{"match": "a", "price_in": -1}},
		"empty match":     []map[string]interface{}{{"match": " ", "iq_tier": 5}},
		"no fields":       []map[string]interface{}{{"match": "a"}},
	} {
		resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
			"model_registry_overrides": bad,
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", name, resp.StatusCode)
		}
	}
	if got := getOverrides(); len(got) != 2 {
		t.Fatalf("rejected writes must leave the stored list unchanged, got %v", got)
	}

	// An empty array clears the list.
	clearResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"model_registry_overrides": []interface{}{},
	})
	clearResp.Body.Close()
	if clearResp.StatusCode != http.StatusOK {
		t.Fatalf("clear overrides: expected 200, got %d", clearResp.StatusCode)
	}
	if got := getOverrides(); len(got) != 0 {
		t.Fatalf("overrides should be cleared, got %v", got)
	}

	// A corrupt stored value (hand-edited row) fails open: reads return an
	// empty list instead of a 500.
	if err := db.SetSetting(store.SettingModelRegistryOverrides, "{not json"); err != nil {
		t.Fatalf("seed corrupt setting: %v", err)
	}
	if got := getOverrides(); len(got) != 0 {
		t.Fatalf("corrupt stored value should read as empty overrides, got %v", got)
	}
}
