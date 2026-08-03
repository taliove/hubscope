package evaluator

import (
	"encoding/json"
	"sort"
	"strconv"
)

// Jury policies (spec 0020): the weighting strategy the jury selector
// ranks candidates with. Stored in the jury_policy setting.
const (
	JuryPolicyBalanced = "balanced"
	JuryPolicySpeed    = "speed"
	JuryPolicyIQ       = "iq"
	JuryPolicyCost     = "cost"
)

// ValidJuryPolicy reports whether s names a known jury policy.
func ValidJuryPolicy(s string) bool {
	switch s {
	case JuryPolicyBalanced, JuryPolicySpeed, JuryPolicyIQ, JuryPolicyCost:
		return true
	}
	return false
}

// juryCandidate is one model eligible to serve on a subject's jury: a
// non-retired model on the subject's hub with an enabled chat endpoint,
// probed reachable by the gate. Registry fields are nil when the model is
// unregistered — unknown is never read as zero by the scorer below (an
// unknown IQ ranks last, an unknown price ranks least cheap).
type juryCandidate struct {
	ModelDBID int64
	ModelID   string
	IQ        *float64 // registry tier 1–10
	PriceIn   *float64 // USD per 1M input tokens
	PriceOut  *float64
	TPS       float64 // measured by the probe gate
}

// jurySelection is the selector's outcome: up to three judge model IDs,
// plus whether the subject had to serve on its own jury (self-preference
// bias risk — only allowed when fewer than three alternatives exist).
type jurySelection struct {
	Judges       []string
	SelfIncluded bool
}

// selectJury ranks the candidates by the active policy and takes the top
// three (spec 0020). The subject is excluded when at least three other
// candidates exist; otherwise it serves and the result flags the
// self-preference risk. Fewer than three judges is a valid (degraded)
// outcome — the median rule degrades with the vote count (ADR 0016).
func selectJury(cands []juryCandidate, policy, subjectModelID string) jurySelection {
	maxTPS := 0.0
	maxPrice := 0.0
	for _, c := range cands {
		if c.TPS > maxTPS {
			maxTPS = c.TPS
		}
		if c.PriceIn != nil && c.PriceOut != nil {
			if p := *c.PriceIn + *c.PriceOut; p > maxPrice {
				maxPrice = p
			}
		}
	}

	score := func(c juryCandidate) float64 {
		iq := 0.0
		if c.IQ != nil {
			iq = *c.IQ / 10
		}
		spd := 0.0
		if maxTPS > 0 {
			spd = c.TPS / maxTPS
		}
		chp := 0.0
		if maxPrice > 0 && c.PriceIn != nil && c.PriceOut != nil {
			chp = 1 - (*c.PriceIn+*c.PriceOut)/maxPrice
		}
		switch policy {
		case JuryPolicySpeed:
			return 0.1*iq + 0.7*spd + 0.2*chp
		case JuryPolicyIQ:
			return 0.8*iq + 0.1*spd + 0.1*chp
		case JuryPolicyCost:
			return 0.15*iq + 0.15*spd + 0.7*chp
		default: // balanced
			return 0.4*iq + 0.3*spd + 0.3*chp
		}
	}

	rank := func(pool []juryCandidate) []juryCandidate {
		out := make([]juryCandidate, len(pool))
		copy(out, pool)
		sort.SliceStable(out, func(a, b int) bool {
			sa, sb := score(out[a]), score(out[b])
			if sa != sb {
				return sa > sb
			}
			return out[a].ModelID < out[b].ModelID
		})
		return out
	}

	var others []juryCandidate
	for _, c := range cands {
		if c.ModelID != subjectModelID {
			others = append(others, c)
		}
	}
	sel := jurySelection{}
	pool := cands
	if len(others) >= 3 {
		pool = others
	} else {
		sel.SelfIncluded = containsModel(cands, subjectModelID)
	}
	for i, c := range rank(pool) {
		if i == 3 {
			break
		}
		sel.Judges = append(sel.Judges, c.ModelID)
	}
	return sel
}

func containsModel(cands []juryCandidate, modelID string) bool {
	for _, c := range cands {
		if c.ModelID == modelID {
			return true
		}
	}
	return false
}

// parseJurySnapshot decodes a stored jury snapshot into the policy and the
// per-model judge lists (keys are model DB IDs). An empty or corrupt
// snapshot yields nil juries — callers then ride the legacy single-judge
// path (pre-jury runs).
func parseJurySnapshot(raw string) (string, map[int64][]string) {
	if raw == "" {
		return "", nil
	}
	var payload struct {
		Policy string              `json:"policy"`
		Juries map[string][]string `json:"juries"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", nil
	}
	juries := make(map[int64][]string, len(payload.Juries))
	for k, v := range payload.Juries {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		juries[id] = v
	}
	return payload.Policy, juries
}

// jurySnapshotJSON renders the run's jury snapshot (ADR 0016): the policy
// plus, per evaluated model, the judges selected for it. The snapshot makes
// every verdict attributable to the jury that produced it. Map keys marshal
// in sorted order, so the snapshot is deterministic.
func jurySnapshotJSON(policy string, juries map[int64]jurySelection) string {
	jm := map[string][]string{}
	for modelDBID, sel := range juries {
		jm[strconv.FormatInt(modelDBID, 10)] = sel.Judges
	}
	raw, err := json.Marshal(map[string]any{"policy": policy, "juries": jm})
	if err != nil {
		return ""
	}
	return string(raw)
}
