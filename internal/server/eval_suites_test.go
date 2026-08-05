package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// fetchSuites fetches GET /api/suites (optionally with a raw query string)
// and returns the decoded suite list.
func fetchSuites(t *testing.T, base, query string) []map[string]interface{} {
	t.Helper()
	resp := doGet(t, base+"/api/suites"+query)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/suites%s: expected 200, got %d", query, resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var suites []map[string]interface{}
	if err := json.Unmarshal(env.Data, &suites); err != nil {
		t.Fatalf("unmarshal suites: %v", err)
	}
	return suites
}

// TestSuitesSeeded verifies the post-cutover seed bank (ticket 99, spec 0014
// decision C, ADR 0013): exactly the five authoritative-benchmark suites in
// the rotation, all enabled, each carrying its 100-case frozen subset of
// single-sample rule cases. The v3 capability suites never appear (retired
// and purged at Open, ADR 0012), nor do pre-v3 legacy suites. Every verdict
// is a deterministic rule — zero judge cases by design (spec 0014: 全部规则
// 判分,零 LLM 裁判). Nadir floors are the structural random-guess rates:
// 0.25 for the four-option MCQ suites, 0 for the open-ended ones (ticket 99
// recalibration confirmed the floors, see the ticket notes).
func TestSuitesSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suites := fetchSuites(t, ts.URL, "")
	if len(suites) != 5 {
		t.Fatalf("expected exactly the 5 benchmark suites, got %d", len(suites))
	}

	byKey := map[string]map[string]interface{}{}
	for _, s := range suites {
		byKey[s["key"].(string)] = s
	}

	// No v3 capability suite and no pre-v3 legacy suite may be listed.
	for key := range byKey {
		if strings.HasPrefix(key, "cap_") {
			t.Errorf("v3 suite %q listed, want retired and purged at the cutover", key)
		}
	}
	for _, key := range []string{"basic", "reasoning", "coding", "chinese"} {
		if _, ok := byKey[key]; ok {
			t.Errorf("legacy suite %q listed, want purged/never seeded", key)
		}
	}

	want := map[string]struct {
		capability string
		nadir      float64
		ruleMode   string
	}{
		"mmlu":       {"knowledge", 0.25, "mcq"},
		"agieval_zh": {"language", 0.25, "mcq"},
		"gsm8k":      {"reasoning", 0, "numeric"},
		"cruxeval":   {"coding", 0, "output_match"},
		"ifeval":     {"instruction", 0, "ifeval"},
	}
	for key, w := range want {
		s, ok := byKey[key]
		if !ok {
			t.Fatalf("missing benchmark suite %q", key)
		}
		if s["enabled"] != true {
			t.Errorf("benchmark suite %q enabled = %v, want true (post-cutover rotation)", key, s["enabled"])
		}
		if s["capability"] != w.capability {
			t.Errorf("suite %q capability = %v, want %q", key, s["capability"], w.capability)
		}
		if s["version"] != 1.0 {
			t.Errorf("fresh suite %q version = %v, want 1", key, s["version"])
		}
		if s["nadir"] != w.nadir {
			t.Errorf("suite %q nadir = %v, want %v", key, s["nadir"], w.nadir)
		}
		cases := s["cases"].([]interface{})
		// ifeval keeps 23: one single-instruction case per ported type (22)
		// plus the multi-instruction combo — the checker coverage contract
		// outranks the round number.
		want := 20
		if key == "ifeval" {
			want = 23
		}
		if len(cases) != want {
			t.Errorf("suite %q has %d cases, want %d (frozen subset)", key, len(cases), want)
		}
		for _, c := range cases {
			cm := c.(map[string]interface{})
			if cm["prompt"] == "" {
				t.Errorf("suite %q has a case with empty prompt", key)
			}
			if cm["enabled"] != true {
				t.Errorf("seed case in suite %q should be enabled", key)
			}
			if cm["verdict_type"] != "rule" {
				t.Errorf("suite %q case verdict_type = %v, want rule (zero LLM judge)", key, cm["verdict_type"])
			}
			if cm["sample_count"] != 1.0 {
				t.Errorf("suite %q case sample_count = %v, want 1", key, cm["sample_count"])
			}
			if key == "ifeval" {
				if cm["check_params"] == nil {
					t.Errorf("ifeval case missing check_params: %v", cm)
				}
				continue
			}
			rc, ok := cm["rule_config"].(map[string]interface{})
			if !ok || rc["mode"] != w.ruleMode {
				t.Errorf("suite %q case rule mode = %v, want %q", key, rc["mode"], w.ruleMode)
			}
		}
	}
}

// TestSuitesCapabilityFilter covers the capability query parameter of
// GET /api/suites: it narrows the listing to the matching capability
// dimension, while an absent parameter lists everything.
func TestSuitesCapabilityFilter(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	filtered := fetchSuites(t, ts.URL, "?capability=reasoning")
	if len(filtered) != 1 || filtered[0]["key"] != "gsm8k" {
		t.Fatalf("capability=reasoning suites = %v, want exactly gsm8k", filtered)
	}
	if filtered[0]["capability"] != "reasoning" {
		t.Errorf("filtered suite capability = %v, want reasoning", filtered[0]["capability"])
	}

	// An unknown capability yields an empty list, not an error.
	if got := fetchSuites(t, ts.URL, "?capability=nosuch"); len(got) != 0 {
		t.Errorf("capability=nosuch suites = %v, want empty", got)
	}

	// No parameter: exactly the five benchmark suites of the rotation.
	if got := fetchSuites(t, ts.URL, ""); len(got) != 5 {
		t.Errorf("unfiltered suites = %d, want 5", len(got))
	}
}
