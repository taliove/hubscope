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

// TestSuitesSeeded verifies the migration ships the question-bank v3 seed
// (ADR 0010): five capability suites in the rotation with 8-12 first-issue
// cases each across three difficulty tiers, judge cases at sample_count 3
// and rule cases at 1, and the knowledge suite calibrated to the
// multiple-choice nadir floor. Pre-v3 legacy suites never appear (spec 0014
// decision B, ADR 0012): disabled suites are hard-deleted at Open and the
// legacy bank is no longer seeded. The mmlu and agieval_zh benchmark suites
// (ADR 0013) are the sixth and seventh seeds: listed but disabled until the
// ticket-99 cutover.
func TestSuitesSeeded(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	suites := fetchSuites(t, ts.URL, "")
	if len(suites) != 7 {
		t.Fatalf("expected 7 suites (5 capability + 2 benchmark disabled), got %d", len(suites))
	}

	byKey := map[string]map[string]interface{}{}
	for _, s := range suites {
		byKey[s["key"].(string)] = s
	}

	// No pre-v3 legacy suite (empty capability) may be listed.
	for _, key := range []string{"basic", "reasoning", "coding", "chinese"} {
		if _, ok := byKey[key]; ok {
			t.Errorf("legacy suite %q listed, want purged/never seeded", key)
		}
	}
	for _, s := range suites {
		if s["capability"] == "" {
			t.Errorf("suite %q has empty capability (legacy), want capability suites only", s["key"])
		}
	}

	// Capability suites: enabled, capability-tagged, 8-12 cases, three tiers.
	capByKey := map[string]string{
		"cap_instruction": "instruction",
		"cap_reasoning":   "reasoning",
		"cap_coding":      "coding",
		"cap_language":    "language",
		"cap_knowledge":   "knowledge",
	}
	for key, capability := range capByKey {
		s, ok := byKey[key]
		if !ok {
			t.Fatalf("missing capability suite %q", key)
		}
		if s["enabled"] != true {
			t.Errorf("capability suite %q enabled = %v, want true", key, s["enabled"])
		}
		if s["capability"] != capability {
			t.Errorf("suite %q capability = %v, want %q", key, s["capability"], capability)
		}
		if s["version"] != 1.0 {
			t.Errorf("fresh suite %q version = %v, want 1", key, s["version"])
		}
		cases := s["cases"].([]interface{})
		if len(cases) < 8 || len(cases) > 12 {
			t.Errorf("suite %q has %d first-issue cases, want 8~12", key, len(cases))
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
			// Rule cases pin one sample; judge cases pin three and carry a
			// rubric spelling out the 1/0.5/0 scale.
			if cm["verdict_type"] == "rule" {
				if cm["sample_count"] != 1.0 {
					t.Errorf("suite %q rule case sample_count = %v, want 1", key, cm["sample_count"])
				}
				rc, ok := cm["rule_config"].(map[string]interface{})
				if !ok || rc["mode"] == "" || rc["expected"] == "" {
					t.Errorf("suite %q rule case missing rule_config: %v", key, cm["rule_config"])
				}
			} else {
				if cm["sample_count"] != 3.0 {
					t.Errorf("suite %q judge case sample_count = %v, want 3", key, cm["sample_count"])
				}
				rubric, ok := cm["rubric"].(string)
				if !ok || !strings.Contains(rubric, "0.5") {
					t.Errorf("suite %q judge case rubric should spell out the 1/0.5/0 scale: %v", key, cm["rubric"])
				}
			}
		}
		for _, tier := range []string{"basic", "intermediate", "hard"} {
			if tiers[tier] == 0 {
				t.Errorf("suite %q has no %s-tier seed case", key, tier)
			}
		}
	}

	// Judge cases stay a minority of the bank (ADR 0010 caps them at 40%).
	total, judges := 0, 0
	for key := range capByKey {
		for _, c := range byKey[key]["cases"].([]interface{}) {
			total++
			if c.(map[string]interface{})["verdict_type"] == "judge" {
				judges++
			}
		}
	}
	if judges == 0 || judges*100/total > 40 {
		t.Errorf("judge share = %d/%d, want >0 and <= 40%%", judges, total)
	}

	// The knowledge suite is multiple choice throughout and floors its nadir
	// at the random-guess rate; the other capability suites floor at 0.
	if got := byKey["cap_knowledge"]["nadir"]; got != 0.25 {
		t.Errorf("cap_knowledge nadir = %v, want 0.25", got)
	}
	for _, key := range []string{"cap_instruction", "cap_reasoning", "cap_coding", "cap_language"} {
		if got := byKey[key]["nadir"]; got != 0.0 {
			t.Errorf("suite %q nadir = %v, want 0", key, got)
		}
	}
}

// TestSuitesCapabilityFilter covers the capability query parameter of
// GET /api/suites: it narrows the listing to the matching capability
// dimension, while an absent parameter lists everything.
func TestSuitesCapabilityFilter(t *testing.T) {
	ts, _, _ := setupEvalEnv(t)

	filtered := fetchSuites(t, ts.URL, "?capability=reasoning")
	if len(filtered) != 1 || filtered[0]["key"] != "cap_reasoning" {
		t.Fatalf("capability=reasoning suites = %v, want only cap_reasoning", filtered)
	}
	if filtered[0]["capability"] != "reasoning" {
		t.Errorf("filtered suite capability = %v, want reasoning", filtered[0]["capability"])
	}

	// An unknown capability yields an empty list, not an error.
	if got := fetchSuites(t, ts.URL, "?capability=nosuch"); len(got) != 0 {
		t.Errorf("capability=nosuch suites = %v, want empty", got)
	}

	// No parameter: every suite, the full capability bank plus the disabled
	// benchmark suites.
	if got := fetchSuites(t, ts.URL, ""); len(got) != 7 {
		t.Errorf("unfiltered suites = %d, want 7", len(got))
	}
}
