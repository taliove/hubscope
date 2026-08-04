package evaluator

import (
	"reflect"
	"testing"
)

func c(model string, iq, price float64, tps float64) juryCandidate {
	c := juryCandidate{ModelID: model, TPS: tps}
	if iq > 0 {
		c.IQ = &iq
	}
	if price >= 0 {
		c.PriceIn = &price
		c.PriceOut = &price
	}
	return c
}

func TestSelectJuryExcludesSubjectWithEnoughAlternatives(t *testing.T) {
	cands := []juryCandidate{
		c("subject", 10, 1, 100), // highest IQ but must not judge itself
		c("cand-1", 9, 1, 90),
		c("cand-2", 8, 1, 80),
		c("cand-3", 7, 1, 70),
		c("cand-4", 1, 1, 60),
	}
	sel := selectJury(cands, JuryPolicyIQ, "subject")
	want := []string{"cand-1", "cand-2", "cand-3"}
	if !reflect.DeepEqual(sel.Judges, want) {
		t.Errorf("judges = %v, want %v (subject excluded with 4 alternatives)", sel.Judges, want)
	}
	if sel.SelfIncluded {
		t.Error("self-inclusion flag must stay false when alternatives suffice")
	}
}

func TestSelectJuryAllowsSelfWhenAlternativesShort(t *testing.T) {
	cands := []juryCandidate{
		c("subject", 9, 1, 100),
		c("cand-1", 8, 1, 90),
	}
	sel := selectJury(cands, JuryPolicyBalanced, "subject")
	if !sel.SelfIncluded {
		t.Error("subject must serve (flagged) when fewer than 3 alternatives exist")
	}
	if len(sel.Judges) != 2 {
		t.Errorf("short jury = %v, want both candidates", sel.Judges)
	}
}

func TestSelectJuryPolicyWeights(t *testing.T) {
	cands := []juryCandidate{
		c("smart-slow-pricey", 10, 100, 10),
		c("dumb-fast-cheap", 1, 0, 100),
		c("mid", 5, 1, 50),
		// A fourth candidate so "subject" (absent) exclusion does not apply.
		c("filler", 2, 1, 40),
	}
	if sel := selectJury(cands, JuryPolicyIQ, "subject"); sel.Judges[0] != "smart-slow-pricey" {
		t.Errorf("iq policy winner = %v, want smart-slow-pricey", sel.Judges[0])
	}
	if sel := selectJury(cands, JuryPolicySpeed, "subject"); sel.Judges[0] != "dumb-fast-cheap" {
		t.Errorf("speed policy winner = %v, want dumb-fast-cheap", sel.Judges[0])
	}
	if sel := selectJury(cands, JuryPolicyCost, "subject"); sel.Judges[0] != "dumb-fast-cheap" {
		t.Errorf("cost policy winner = %v, want dumb-fast-cheap", sel.Judges[0])
	}
	if sel := selectJury(cands, JuryPolicyBalanced, "subject"); sel.Judges[0] != "mid" {
		t.Errorf("balanced policy winner = %v, want mid", sel.Judges[0])
	}
}

func TestSelectJuryUnknownFieldsRankLast(t *testing.T) {
	registered := c("registered", 5, 1, 50)
	unknown := juryCandidate{ModelID: "unknown"} // no IQ, no price, no TPS
	filler1 := c("filler-1", 1, 1, 10)
	filler2 := c("filler-2", 1, 1, 10)
	sel := selectJury([]juryCandidate{unknown, registered, filler1, filler2}, JuryPolicyBalanced, "subject")
	for _, j := range sel.Judges {
		if j == "unknown" {
			t.Errorf("unregistered model must not make the jury over registered ones: %v", sel.Judges)
		}
	}
}

func TestSelectJuryEmptyPool(t *testing.T) {
	sel := selectJury(nil, JuryPolicyBalanced, "subject")
	if len(sel.Judges) != 0 {
		t.Errorf("empty pool must yield an empty jury, got %v", sel.Judges)
	}
}

func TestJurySnapshotDeterministic(t *testing.T) {
	juries := map[int64]jurySelection{
		7:  {Judges: []string{"a", "b"}},
		12: {Judges: []string{"c"}},
	}
	probes := map[string]probeSummaryDTO{
		"a": {OK: true, Succ: 3, Rounds: 3, TPS: 88},
	}
	first := jurySnapshotJSON(JuryPolicyBalanced, juries, probes)
	for range 10 {
		if got := jurySnapshotJSON(JuryPolicyBalanced, juries, probes); got != first {
			t.Fatalf("snapshot not deterministic: %q vs %q", got, first)
		}
	}
	want := `{"juries":{"12":["c"],"7":["a","b"]},"policy":"balanced","probe":{"a":{"ok":true,"succ":3,"rounds":3,"tps":88}}}`
	if first != want {
		t.Errorf("snapshot = %s, want %s", first, want)
	}
}
