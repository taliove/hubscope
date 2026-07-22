package store

import (
	"strconv"
	"time"
)

// seedCase is one built-in evaluation question. Nullable rule/rubric fields
// use pointers so only the relevant one is set per verdict type. gen is the
// seed generation that introduced the case: generation 1 is the original
// 3-per-suite bank, generation 2 the tiered expansion (ticket 21), generation
// 3 the capability-bank relaunch (ticket 50, ADR 0010). difficulty is one of
// basic/intermediate/hard. sampleCount pins the per-case sample count (nil
// inherits the global default); v3 seeds set it explicitly — 3 for judge
// cases, 1 for rule cases.
type seedCase struct {
	gen          int
	difficulty   string
	prompt       string
	verdictType  string
	ruleMode     *string
	ruleExpected *string
	rubric       *string
	sampleCount  *int
}

// seedSuite is a built-in evaluation suite with its cases. capability is the
// ADR 0010 capability dimension (empty for legacy suites). nadir is the
// suite's normalization constant (ADR 0009), locked at insert time. retireAtGen
// is the seed generation at which the suite leaves the evaluation rotation
// (0 = stays enabled): a one-time, generation-tracked retirement that never
// deletes the suite or its cases and never re-applies after an admin
// re-enables it.
type seedSuite struct {
	key         string
	name        string
	capability  string
	nadir       float64
	retireAtGen int
	cases       []seedCase
}

// judgeRubricSuffix reminds the judge of the required output format. Seed
// rubrics spell out the scoring scale; this suffix pins the wire format.
const judgeRubricSuffix = "只输出 JSON：{\"score\": 0到1之间的数字, \"reason\": \"简短中文理由\"}，不要输出任何其他内容。"

// seedSuites inserts the built-in suites and their cases. It is additive-only
// and idempotent: suites are keyed by UNIQUE key, and each suite records the
// seed generation it has received in settings (seed_gen_<key>). Cases with a
// higher generation than the recorded one are inserted; nothing is ever
// updated or deleted, so admin edits are never reverted or duplicated.
//
// A suite that already has cases but no recorded generation came from the
// pre-tiering bank (generation 1), so only later-generation cases are added.
// A suite with no cases at all receives the full bank.
//
// Retirement (retireAtGen) is also generation-tracked: when the recorded
// generation predates it, the suite's enabled flag is cleared once and the
// generation is recorded, so re-opening the database never re-retires a
// suite an admin deliberately re-enabled.
func (db *DB) seedSuites() error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, suite := range builtinSuites {
		if _, err := db.conn.Exec(
			"INSERT OR IGNORE INTO suites (key, name, capability, nadir) VALUES (?, ?, ?, ?)",
			suite.key, suite.name, suite.capability, suite.nadir,
		); err != nil {
			return err
		}

		var suiteID int64
		if err := db.conn.QueryRow(
			"SELECT id FROM suites WHERE key = ?", suite.key,
		).Scan(&suiteID); err != nil {
			return err
		}

		received, err := db.seedGeneration(suite.key, suiteID)
		if err != nil {
			return err
		}

		maxGen := received
		for _, c := range suite.cases {
			if c.gen > maxGen {
				maxGen = c.gen
			}
		}
		if suite.retireAtGen > maxGen {
			maxGen = suite.retireAtGen
		}

		if suite.retireAtGen > received {
			if _, err := db.conn.Exec(
				"UPDATE suites SET enabled = 0 WHERE id = ?", suiteID,
			); err != nil {
				return err
			}
		}

		for _, c := range suite.cases {
			if c.gen <= received {
				continue
			}
			if _, err := db.conn.Exec(`
				INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, difficulty, sample_count, enabled, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
			`, suiteID, c.prompt, c.verdictType, c.ruleMode, c.ruleExpected, c.rubric, c.difficulty, c.sampleCount, now); err != nil {
				return err
			}
		}
		if maxGen > received {
			if err := db.SetSetting(seedGenerationKey(suite.key), strconv.Itoa(maxGen)); err != nil {
				return err
			}
		}
	}
	return nil
}

// seedGenerationKey is the settings key recording a suite's seed generation.
func seedGenerationKey(suiteKey string) string {
	return "seed_gen_" + suiteKey
}

// seedGeneration returns the highest seed generation a suite has received. A
// missing record means first contact: a suite that already holds cases came
// from the original generation-1 bank; an empty suite starts at generation 0.
func (db *DB) seedGeneration(suiteKey string, suiteID int64) (int, error) {
	recorded, err := db.GetSetting(seedGenerationKey(suiteKey), "")
	if err != nil {
		return 0, err
	}
	if recorded != "" {
		gen, err := strconv.Atoi(recorded)
		if err != nil {
			return 0, err
		}
		return gen, nil
	}

	var caseCount int
	if err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM cases WHERE suite_id = ?", suiteID,
	).Scan(&caseCount); err != nil {
		return 0, err
	}
	if caseCount > 0 {
		return 1, nil
	}
	return 0, nil
}

// strptr returns a pointer to s, for building seed data literals.
func strptr(s string) *string { return &s }

// intptr returns a pointer to n, for building seed data literals.
func intptr(n int) *int { return &n }
