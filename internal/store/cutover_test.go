package store

import (
	"fmt"
	"testing"
)

// TestTrimBenchmarkSuitesToTwenty pins the 100→20 convergence migration: a
// database that cast the 100-row bank keeps exactly the every-5th cases
// enabled, the suite version bumps once (trend breakpoint caliber), custom
// cases past the seed window stay untouched, and the migration is
// idempotent and one-shot.
func TestTrimBenchmarkSuitesToTwenty(t *testing.T) {
	db := openTestDB(t)

	// Fresh banks seed 20 cases: the trim is a no-op there.
	suites, err := db.ListSuites()
	if err != nil {
		t.Fatalf("list suites: %v", err)
	}
	byKey := map[string]Suite{}
	for _, s := range suites {
		byKey[s.Key] = s
	}
	mmlu := byKey["mmlu"]
	cases, err := db.ListCases(mmlu.ID)
	if err != nil {
		t.Fatalf("list cases: %v", err)
	}
	if len(cases) != 20 {
		t.Fatalf("fresh bank mmlu cases = %d, want 20", len(cases))
	}

	// Fake the 100-row bank: disable the trim flag, re-add 80 enabled
	// cases behind the seeded 20 (seed order = id order), plus one custom
	// case that must survive. CreateCase inserts disabled (the review-flow
	// default), so each synthetic case is enabled explicitly.
	if err := db.SetSetting(benchmarkTrimKey, ""); err != nil {
		t.Fatalf("clear trim flag: %v", err)
	}
	for i := 20; i < 100; i++ {
		c, err := db.CreateCase(Case{
			SuiteID: mmlu.ID, Prompt: fmt.Sprintf("legacy-row-%d", i),
			VerdictType: "rule",
		})
		if err != nil {
			t.Fatalf("insert legacy case %d: %v", i, err)
		}
		if _, err := db.SetCaseEnabled(c.ID, true); err != nil {
			t.Fatalf("enable legacy case %d: %v", i, err)
		}
	}
	custom, err := db.CreateCase(Case{SuiteID: mmlu.ID, Prompt: "custom-admin-case", VerdictType: "rule"})
	if err != nil {
		t.Fatalf("insert custom case: %v", err)
	}
	if _, err := db.SetCaseEnabled(custom.ID, true); err != nil {
		t.Fatalf("enable custom case: %v", err)
	}

	// Version bumped once (the bank changed) and the migration is a no-op
	// on the second run. The baseline is read right before the trim: case
	// creation bumps the version too, and only the trim's bump matters here.
	preTrim, err := db.GetSuite(mmlu.ID)
	if err != nil {
		t.Fatalf("reload suite pre-trim: %v", err)
	}
	if err := db.trimBenchmarkSuitesToTwenty(); err != nil {
		t.Fatalf("trim: %v", err)
	}

	after, err := db.ListCases(mmlu.ID)
	if err != nil {
		t.Fatalf("list cases after trim: %v", err)
	}
	var enabled, customAlive int
	for _, c := range after {
		if c.Enabled {
			enabled++
			if c.ID == custom.ID {
				customAlive++
			}
		}
	}
	if enabled != 21 {
		t.Errorf("enabled cases after trim = %d, want 21 (20 kept + 1 custom)", enabled)
	}
	if customAlive != 1 {
		t.Error("custom case past the seed window must stay enabled")
	}

	// The kept cases are exactly the 20 seeded prompts (seed order = id
	// order): every legacy row retired, the custom case alive past the
	// seed window.
	var enabledIDs []int64
	for _, c := range after {
		if c.Enabled && c.ID != custom.ID {
			enabledIDs = append(enabledIDs, c.ID)
		}
	}
	if len(enabledIDs) != 20 {
		t.Fatalf("kept seeded cases = %d, want 20", len(enabledIDs))
	}
	seededIDs := map[int64]bool{}
	for _, c := range cases {
		seededIDs[c.ID] = true
	}
	for _, id := range enabledIDs {
		if !seededIDs[id] {
			t.Errorf("kept case %d is not one of the seeded 20", id)
		}
	}

	// Version bumped once (the bank changed) and the migration is a no-op
	// on the second run.
	reloaded, err := db.GetSuite(mmlu.ID)
	if err != nil {
		t.Fatalf("reload suite: %v", err)
	}
	if reloaded.Version <= preTrim.Version {
		t.Errorf("suite version = %d, want > %d (breakpoint for the bank change)", reloaded.Version, preTrim.Version)
	}
	if err := db.trimBenchmarkSuitesToTwenty(); err != nil {
		t.Fatalf("second trim must be a no-op: %v", err)
	}
	again, _ := db.ListCases(mmlu.ID)
	enabled = 0
	for _, c := range again {
		if c.Enabled {
			enabled++
		}
	}
	if enabled != 21 {
		t.Errorf("enabled after second trim = %d, want 21 (idempotent)", enabled)
	}
}
