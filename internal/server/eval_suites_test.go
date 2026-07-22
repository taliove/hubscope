package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestSuitesSeeded verifies the migration ships the 4 built-in suites with
// their seed cases, difficulty tiers and verdict configurations.
func TestSuitesSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	resp := doGet(t, ts.URL+"/api/suites")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	var suites []map[string]interface{}
	if err := json.Unmarshal(env.Data, &suites); err != nil {
		t.Fatalf("unmarshal suites: %v", err)
	}

	if len(suites) != 4 {
		t.Fatalf("expected 4 built-in suites, got %d", len(suites))
	}

	byKey := map[string]map[string]interface{}{}
	for _, s := range suites {
		byKey[s["key"].(string)] = s
	}
	for _, key := range []string{"basic", "reasoning", "coding", "chinese"} {
		s, ok := byKey[key]
		if !ok {
			t.Fatalf("missing built-in suite %q", key)
		}
		if s["version"] != 1.0 {
			t.Errorf("fresh suite %q version = %v, want 1", key, s["version"])
		}
		cases := s["cases"].([]interface{})
		if len(cases) < 10 || len(cases) > 20 {
			t.Errorf("suite %q has %d cases, want 10~20", key, len(cases))
		}
		tiers := map[string]int{}
		for _, c := range cases {
			cm := c.(map[string]interface{})
			if cm["prompt"] == "" {
				t.Errorf("suite %q has a case with empty prompt", key)
			}
			if cm["enabled"] != true {
				t.Errorf("seed case in suite %q should be enabled", key)
			}
			d, _ := cm["difficulty"].(string)
			switch d {
			case "basic", "intermediate", "hard":
				tiers[d]++
			default:
				t.Errorf("suite %q has a case with invalid difficulty %q", key, d)
			}
		}
		for _, tier := range []string{"basic", "intermediate", "hard"} {
			if tiers[tier] == 0 {
				t.Errorf("suite %q has no %s-tier seed case", key, tier)
			}
		}
	}

	// basic/reasoning/coding are rule-judged with a rule_config each.
	for _, key := range []string{"basic", "reasoning", "coding"} {
		for _, c := range byKey[key]["cases"].([]interface{}) {
			cm := c.(map[string]interface{})
			if cm["verdict_type"] != "rule" {
				t.Errorf("suite %q case verdict_type = %v, want rule", key, cm["verdict_type"])
			}
			rc, ok := cm["rule_config"].(map[string]interface{})
			if !ok || rc["mode"] == "" || rc["expected"] == "" {
				t.Errorf("suite %q rule case missing rule_config: %v", key, cm["rule_config"])
			}
		}
	}

	// chinese is judge-judged with a rubric each.
	for _, c := range byKey["chinese"]["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if cm["verdict_type"] != "judge" {
			t.Errorf("chinese case verdict_type = %v, want judge", cm["verdict_type"])
		}
		rubric, ok := cm["rubric"].(string)
		if !ok || !strings.Contains(rubric, "score") {
			t.Errorf("chinese judge case missing usable rubric: %v", cm["rubric"])
		}
	}
}
