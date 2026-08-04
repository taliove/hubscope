package server_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// suiteByKey fetches GET /api/suites and returns the suite with the given key.
func suiteByKey(t *testing.T, base, key string) map[string]interface{} {
	t.Helper()
	resp := doGet(t, base+"/api/suites")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/suites: expected 200, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var suites []map[string]interface{}
	_ = json.Unmarshal(env.Data, &suites)
	for _, s := range suites {
		if s["key"] == key {
			return s
		}
	}
	t.Fatalf("suite %q not found", key)
	return nil
}

// caseByID locates a case inside a suite listing.
func caseByID(t *testing.T, suite map[string]interface{}, id int64) map[string]interface{} {
	t.Helper()
	for _, c := range suite["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if int64(cm["id"].(float64)) == id {
			return cm
		}
	}
	t.Fatalf("case %d not found in suite %v", id, suite["key"])
	return nil
}

// patchCase issues PATCH /api/cases/{id} and expects 200.
func patchCase(t *testing.T, base string, id int64, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := doPatch(t, fmt.Sprintf("%s/api/cases/%d", base, id), body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/cases/%d: expected 200, got %d", id, resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var out map[string]interface{}
	_ = json.Unmarshal(env.Data, &out)
	return out
}

// TestSuiteVersioning covers the Suite Version contract: every case mutation
// bumps the version, a no-op patch does not, and each eval run records the
// version it ran against.
func TestSuiteVersioning(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	if v := suiteByKey(t, ts.URL, "gsm8k")["version"]; v != 1.0 {
		t.Fatalf("initial version = %v, want 1", v)
	}

	// A run records the version in effect at creation.
	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)
	run := getEvalRun(t, ts.URL, run1)
	if run["suite_version"] != 1.0 {
		t.Errorf("run1 suite_version = %v, want 1", run["suite_version"])
	}

	// Content patch on a seed case: new case id, old case disabled, version 2.
	seedID := firstCaseID(t, suiteByKey(t, ts.URL, "gsm8k"))
	seed := caseByID(t, suiteByKey(t, ts.URL, "gsm8k"), seedID)
	patched := patchCase(t, ts.URL, seedID, map[string]interface{}{"prompt": "只回复 pong，别的什么都不要说"})
	newID := int64(patched["id"].(float64))
	if newID == seedID {
		t.Fatalf("content patch should mint a new case id, still got %d", newID)
	}
	if v := suiteByKey(t, ts.URL, "gsm8k")["version"]; v != 2.0 {
		t.Errorf("after content patch version = %v, want 2", v)
	}
	listed := suiteByKey(t, ts.URL, "gsm8k")
	old := caseByID(t, listed, seedID)
	if old["enabled"] != false || old["prompt"] != seed["prompt"] {
		t.Errorf("old case should be disabled with its original prompt: %v", old)
	}

	// The next run records version 2 and executes the new case, not the old.
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	run = getEvalRun(t, ts.URL, run2)
	if run["suite_version"] != 2.0 {
		t.Errorf("run2 suite_version = %v, want 2", run["suite_version"])
	}
	var sawOld, sawNew bool
	for _, r := range resultsByModel(run, "smart-model") {
		switch int64(r["case_id"].(float64)) {
		case seedID:
			sawOld = true
		case newID:
			sawNew = true
		}
	}
	if sawOld {
		t.Error("run2 should not execute the retired case")
	}
	if !sawNew {
		t.Error("run2 should execute the replacement case")
	}

	// Case creation bumps to 3.
	createResp := doPost(t, ts.URL+"/api/cases", map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "只回复 hi",
		"verdict_type": "rule",
		"rule_config":  map[string]string{"mode": "contains", "expected": "hi"},
	})
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create case: expected 201, got %d", createResp.StatusCode)
	}
	if v := suiteByKey(t, ts.URL, "gsm8k")["version"]; v != 3.0 {
		t.Errorf("after create version = %v, want 3", v)
	}

	// Enabled-only patch bumps to 4.
	patchCase(t, ts.URL, newID, map[string]interface{}{"enabled": false})
	if v := suiteByKey(t, ts.URL, "gsm8k")["version"]; v != 4.0 {
		t.Errorf("after disable version = %v, want 4", v)
	}

	// A patch that changes nothing must not bump the version.
	noop := patchCase(t, ts.URL, seedID, map[string]interface{}{"prompt": seed["prompt"].(string)})
	if int64(noop["id"].(float64)) != seedID {
		t.Errorf("no-op patch should return the same case, got %v", noop["id"])
	}
	if v := suiteByKey(t, ts.URL, "gsm8k")["version"]; v != 4.0 {
		t.Errorf("after no-op patch version = %v, want still 4", v)
	}
}

// getEvalRun fetches GET /api/evals/{id}.
func getEvalRun(t *testing.T, base string, id int64) map[string]interface{} {
	t.Helper()
	resp := doGet(t, fmt.Sprintf("%s/api/evals/%d", base, id))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/evals/%d: expected 200, got %d", id, resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var run map[string]interface{}
	_ = json.Unmarshal(env.Data, &run)
	return run
}

// TestCaseImmutabilityKeepsHistory runs a suite, edits one case, and asserts
// the old run still references the old case (which keeps its original prompt)
// while new runs reference the replacement.
func TestCaseImmutabilityKeepsHistory(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "smart-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	target := suiteByKey(t, ts.URL, "gsm8k")["cases"].([]interface{})[1].(map[string]interface{})
	targetID := int64(target["id"].(float64))
	oldPrompt := target["prompt"].(string)

	run1 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run1)

	replacement := patchCase(t, ts.URL, targetID, map[string]interface{}{"prompt": "用严格的 JSON 回复 {\"ok\": false}"})
	newID := int64(replacement["id"].(float64))

	// The old run still points at the old case id, and the old case row keeps
	// its original prompt, so the historical result renders the old question.
	run1Detail := getEvalRun(t, ts.URL, run1)
	var found bool
	for _, r := range resultsByModel(run1Detail, "smart-model") {
		if int64(r["case_id"].(float64)) == targetID {
			found = true
		}
	}
	if !found {
		t.Error("run1 should still reference the retired case id")
	}
	oldRow := caseByID(t, suiteByKey(t, ts.URL, "gsm8k"), targetID)
	if oldRow["prompt"] != oldPrompt || oldRow["enabled"] != false {
		t.Errorf("retired case lost its original prompt: %v", oldRow)
	}

	// The new run scores the replacement case instead.
	run2 := triggerEval(t, ts.URL, suiteID, modelID)
	waitEvalDone(t, ts.URL, run2)
	run2Detail := getEvalRun(t, ts.URL, run2)
	var scoredNew bool
	for _, r := range resultsByModel(run2Detail, "smart-model") {
		if int64(r["case_id"].(float64)) == newID && r["score"] != nil {
			scoredNew = true
		}
	}
	if !scoredNew {
		t.Error("run2 should score the replacement case")
	}
}

// TestEvalSampling covers per-case and global-default sample counts: a case
// is answered N times and its score is the average of the judged samples;
// samples whose judge fails stay unjudged and are excluded from the average.
func TestEvalSampling(t *testing.T) {
	ts, stub, _ := setupEvalEnv(t)
	modelID := createEvalModel(t, ts.URL, stub.URL, "sample-model")
	suiteID := suiteIDByKey(t, ts.URL, "gsm8k")

	// Global default of 2 samples for cases without an explicit count.
	putResp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{"default_sample_count": 2})
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", putResp.StatusCode)
	}

	// Rule case with 3 explicit samples: answers yes/no/yes -> scores 1,0,1.
	stub.setAnswerSeq("SEQ-RULE", "yes", "no", "yes")
	ruleCase := createSamplingCase(t, ts.URL, map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "SEQ-RULE 请只回复 yes",
		"verdict_type": "rule",
		"rule_config":  map[string]string{"mode": "contains", "expected": "yes"},
		"sample_count": 3,
	})

	// Judge case with 3 samples: judge scores 1, garbage (unjudged), 0.
	stub.setJudgeSeq("SEQ-JUDGE",
		`{"score": 1, "reason": "好"}`, "not a score at all", `{"score": 0, "reason": "差"}`)
	judgeCase := createSamplingCase(t, ts.URL, map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "SEQ-JUDGE 随便写一句话",
		"verdict_type": "judge",
		"rubric":       "评估作答是否是一句通顺的中文。是 1 分,否 0 分。",
		"sample_count": 3,
	})

	// Rule case without a sample count: inherits the global default of 2.
	stub.setAnswerSeq("SEQ-DEFAULT", "ok", "no")
	defaultCase := createSamplingCase(t, ts.URL, map[string]interface{}{
		"suite_id":     suiteID,
		"prompt":       "SEQ-DEFAULT 请只回复 ok",
		"verdict_type": "rule",
		"rule_config":  map[string]string{"mode": "contains", "expected": "ok"},
	})

	runID := triggerEval(t, ts.URL, suiteID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	if run["status"] != "done" {
		t.Fatalf("run status = %v, want done", run["status"])
	}

	byCase := map[int64]map[string]interface{}{}
	for _, r := range resultsByModel(run, "sample-model") {
		byCase[int64(r["case_id"].(float64))] = r
	}

	// 3 samples scoring 1, 0, 1 average to 2/3.
	rule := byCase[int64(ruleCase["id"].(float64))]
	if rule == nil {
		t.Fatal("rule sampling case missing from results")
	}
	if got := rule["score"].(float64); math.Abs(got-2.0/3.0) > 1e-9 {
		t.Errorf("rule case score = %v, want 0.667 (avg of 1,0,1)", got)
	}
	if !strings.Contains(rule["verdict_detail"].(string), "sample 1/3") {
		t.Errorf("verdict_detail should enumerate samples: %v", rule["verdict_detail"])
	}
	if got := stub.callCount("sample-model", "SEQ-RULE 请只回复 yes"); got != 3 {
		t.Errorf("rule case answered %d times, want 3", got)
	}

	// 3 samples scoring 1, unjudged, 0 average to 0.5 over the judged two.
	judge := byCase[int64(judgeCase["id"].(float64))]
	if judge == nil {
		t.Fatal("judge sampling case missing from results")
	}
	if got := judge["score"].(float64); got != 0.5 {
		t.Errorf("judge case score = %v, want 0.5 (avg of judged 1 and 0)", got)
	}
	if !strings.Contains(judge["verdict_detail"].(string), "judge parse failed") {
		t.Errorf("failed sample should stay visibly unjudged: %v", judge["verdict_detail"])
	}
	if got := stub.callCount("sample-model", "SEQ-JUDGE 随便写一句话"); got != 3 {
		t.Errorf("judge case answered %d times, want 3", got)
	}

	// No explicit count: the global default of 2 applies (scores 1, 0).
	def := byCase[int64(defaultCase["id"].(float64))]
	if def == nil {
		t.Fatal("default sampling case missing from results")
	}
	if got := def["score"].(float64); got != 0.5 {
		t.Errorf("default case score = %v, want 0.5 (avg of 1,0)", got)
	}
	if got := stub.callCount("sample-model", "SEQ-DEFAULT 请只回复 ok"); got != 2 {
		t.Errorf("default case answered %d times, want 2 (global default)", got)
	}
}

// createSamplingCase posts a case and returns the decoded response.
func createSamplingCase(t *testing.T, base string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := doPost(t, base+"/api/cases", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create case: expected 201, got %d", resp.StatusCode)
	}
	var env envelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	var out map[string]interface{}
	_ = json.Unmarshal(env.Data, &out)
	return out
}

// TestPreV3UpgradePurgesLegacySuites stages a pre-ticket-21 database (old
// column set, the original 3 cases per legacy suite, everything enabled) and
// asserts the migration hard-deletes the pre-v3 legacy suites with their
// cases (spec 0014 decision B, ADR 0012) while seeding the capability bank
// exactly once — the upgrade path of a database that never saw the
// generation-tracked retirement.
func TestPreV3UpgradePurgesLegacySuites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.db")
	stagePreTieringDatabase(t, dbPath)

	serveSuites := func() (*httptest.Server, *store.DB) {
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("open migrated db: %v", err)
		}
		seedTestUser(t, db)
		ts := httptest.NewServer(server.New(db, server.WithRateLimits(server.RateLimits{})))
		t.Cleanup(ts.Close)
		return ts, db
	}

	legacyKeys := []string{"basic", "reasoning", "coding", "chinese"}

	ts, db := serveSuites()
	suites := fetchSuites(t, ts.URL, "")
	if len(suites) != 5 {
		t.Fatalf("suites after upgrade = %d, want exactly the 5 benchmark suites: %v", len(suites), suites)
	}
	for _, s := range suites {
		for _, key := range legacyKeys {
			if s["key"] == key {
				t.Errorf("legacy suite %q survived the upgrade purge", key)
			}
		}
		if strings.HasPrefix(s["key"].(string), "cap_") {
			t.Errorf("v3 suite %q survived the upgrade purge", s["key"])
		}
		if s["capability"] == "" {
			t.Errorf("suite %q has no capability after upgrade", s["key"])
		}
		if s["enabled"] != true {
			t.Errorf("benchmark suite %q enabled = %v, want true (post-cutover rotation)", s["key"], s["enabled"])
		}
		// The benchmark suites seed their 100-case frozen subsets on a
		// database that predates them (ADR 0013).
		want := 20
		if s["key"] == "ifeval" {
			want = 23
		}
		if got := len(s["cases"].([]interface{})); got != want {
			t.Errorf("benchmark suite %q has %d cases, want %d", s["key"], got, want)
		}
	}
	db.Close()

	// Re-opening must be a no-op: tombstoned suites never resurrect.
	ts2, db2 := serveSuites()
	defer db2.Close()
	again := fetchSuites(t, ts2.URL, "")
	if len(again) != 5 {
		t.Fatalf("suites after reopen = %d, want still 5 (benchmark rotation)", len(again))
	}
	for _, s := range again {
		for _, key := range legacyKeys {
			if s["key"] == key {
				t.Errorf("legacy suite %q resurrected on reopen", key)
			}
		}
	}
}

// stagePreTieringDatabase writes a minimal pre-ticket-21 database: the four
// built-in suites with their original three seed cases each, under the old
// column set (no version/difficulty/sample_count/suite_version columns).
func stagePreTieringDatabase(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer conn.Close()

	ddl := `
		CREATE TABLE suites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL
		);
		CREATE TABLE cases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			suite_id INTEGER NOT NULL,
			prompt TEXT NOT NULL,
			verdict_type TEXT NOT NULL,
			rule_mode TEXT,
			rule_expected TEXT,
			rubric TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		);
		CREATE TABLE eval_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			suite_id INTEGER NOT NULL,
			"trigger" TEXT NOT NULL,
			judge_model TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT
		);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`
	if _, err := conn.Exec(ddl); err != nil {
		t.Fatalf("create old schema: %v", err)
	}

	type oldCase struct {
		prompt, verdictType, ruleMode, ruleExpected, rubric string
	}
	suites := map[string][]oldCase{
		"basic": {
			{"只回复 pong 这个单词", "rule", "contains", "pong", ""},
			{"用严格的 JSON 回复 {\"ok\": true}，不要任何其他文字", "rule", "regex", `^\s*\{\s*"ok"\s*:\s*true\s*\}\s*$`, ""},
			{"数到 3，每行一个数字", "rule", "regex", `^1\s*\n2\s*\n3\s*$`, ""},
		},
		"reasoning": {
			{"17 + 25 = ? 只回复数字", "rule", "exact", "42", ""},
			{"一个班里 30 人，18 人会游泳，15 人会骑车，至少几人两样都会？只回复数字", "rule", "exact", "3", ""},
			{"数列 1, 1, 2, 3, 5, 8 的下一个数字是什么？只回复数字", "rule", "exact", "13", ""},
		},
		"coding": {
			{"用 Python 写一个函数 add(a, b)，只回复代码", "rule", "regex", `def\s+add\s*\(`, ""},
			{"下面代码的输出是什么，只回复数字： print(len([1,2,3])*2)", "rule", "exact", "6", ""},
			{"Python 表达式 'hello'[1] 的结果是什么？只回复这个字符", "rule", "exact", "e", ""},
		},
		"chinese": {
			{"用一句中文总结'亡羊补牢'的寓意", "judge", "", "", "rubric 1"},
			{"把「他今天没来上班，因为生病了」改写成更正式的表达，只回复改写后的句子", "judge", "", "", "rubric 2"},
			{"「画蛇添足」这个成语是什么意思？用一句中文回答", "judge", "", "", "rubric 3"},
		},
	}
	names := map[string]string{
		"basic": "基础指令遵循", "reasoning": "推理数学", "coding": "代码能力", "chinese": "中文能力",
	}

	for _, key := range []string{"basic", "reasoning", "coding", "chinese"} {
		res, err := conn.Exec("INSERT INTO suites (key, name) VALUES (?, ?)", key, names[key])
		if err != nil {
			t.Fatalf("insert old suite %s: %v", key, err)
		}
		suiteID, _ := res.LastInsertId()
		for _, c := range suites[key] {
			var ruleMode, ruleExpected, rubric interface{}
			if c.ruleMode != "" {
				ruleMode = c.ruleMode
			}
			if c.ruleExpected != "" {
				ruleExpected = c.ruleExpected
			}
			if c.rubric != "" {
				rubric = c.rubric
			}
			if _, err := conn.Exec(`
				INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, enabled, created_at)
				VALUES (?, ?, ?, ?, ?, ?, 1, '2026-01-01T00:00:00Z')
			`, suiteID, c.prompt, c.verdictType, ruleMode, ruleExpected, rubric); err != nil {
				t.Fatalf("insert old case: %v", err)
			}
		}
	}
}
