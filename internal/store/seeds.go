package store

import "time"

// seedCase is one built-in evaluation question. Nullable rule/rubric fields
// use pointers so only the relevant one is set per verdict type.
type seedCase struct {
	prompt       string
	verdictType  string
	ruleMode     *string
	ruleExpected *string
	rubric       *string
}

// seedSuite is a built-in evaluation suite with its cases.
type seedSuite struct {
	key   string
	name  string
	cases []seedCase
}

// judgeRubricSuffix reminds the judge of the required output format. Seed
// rubrics spell out the scoring scale; this suffix pins the wire format.
const judgeRubricSuffix = "只输出 JSON：{\"score\": 0到1之间的数字, \"reason\": \"简短中文理由\"}，不要输出任何其他内容。"

// seedSuites inserts the built-in suites and their cases on first run. It is
// idempotent: suites are keyed by UNIQUE key, and cases are only inserted
// while a suite has none, so admin edits are never duplicated or reverted.
func (db *DB) seedSuites() error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, suite := range builtinSuites {
		if _, err := db.conn.Exec(
			"INSERT OR IGNORE INTO suites (key, name) VALUES (?, ?)",
			suite.key, suite.name,
		); err != nil {
			return err
		}

		var suiteID int64
		if err := db.conn.QueryRow(
			"SELECT id FROM suites WHERE key = ?", suite.key,
		).Scan(&suiteID); err != nil {
			return err
		}

		var caseCount int
		if err := db.conn.QueryRow(
			"SELECT COUNT(*) FROM cases WHERE suite_id = ?", suiteID,
		).Scan(&caseCount); err != nil {
			return err
		}
		if caseCount > 0 {
			continue
		}

		for _, c := range suite.cases {
			if _, err := db.conn.Exec(`
				INSERT INTO cases (suite_id, prompt, verdict_type, rule_mode, rule_expected, rubric, enabled, created_at)
				VALUES (?, ?, ?, ?, ?, ?, 1, ?)
			`, suiteID, c.prompt, c.verdictType, c.ruleMode, c.ruleExpected, c.rubric, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// strptr returns a pointer to s, for building seed data literals.
func strptr(s string) *string { return &s }

// builtinSuites is the seed question bank shipped with the migration.
var builtinSuites = []seedSuite{
	{
		key:  "basic",
		name: "基础指令遵循",
		cases: []seedCase{
			{
				prompt:       "只回复 pong 这个单词",
				verdictType:  "rule",
				ruleMode:     strptr("contains"),
				ruleExpected: strptr("pong"),
			},
			{
				prompt:       "用严格的 JSON 回复 {\"ok\": true}，不要任何其他文字",
				verdictType:  "rule",
				ruleMode:     strptr("regex"),
				ruleExpected: strptr(`^\s*\{\s*"ok"\s*:\s*true\s*\}\s*$`),
			},
			{
				prompt:       "数到 3，每行一个数字",
				verdictType:  "rule",
				ruleMode:     strptr("regex"),
				ruleExpected: strptr(`^1\s*\n2\s*\n3\s*$`),
			},
		},
	},
	{
		key:  "reasoning",
		name: "推理数学",
		cases: []seedCase{
			{
				prompt:       "17 + 25 = ? 只回复数字",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("42"),
			},
			{
				prompt:       "一个班里 30 人，18 人会游泳，15 人会骑车，至少几人两样都会？只回复数字",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("3"),
			},
			{
				prompt:       "数列 1, 1, 2, 3, 5, 8 的下一个数字是什么？只回复数字",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("13"),
			},
		},
	},
	{
		key:  "coding",
		name: "代码能力",
		cases: []seedCase{
			{
				prompt:       "用 Python 写一个函数 add(a, b)，只回复代码",
				verdictType:  "rule",
				ruleMode:     strptr("regex"),
				ruleExpected: strptr(`def\s+add\s*\(`),
			},
			{
				prompt:       "下面代码的输出是什么，只回复数字： print(len([1,2,3])*2)",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("6"),
			},
			{
				prompt:       "Python 表达式 'hello'[1] 的结果是什么？只回复这个字符",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("e"),
			},
		},
	},
	{
		key:  "chinese",
		name: "中文能力",
		cases: []seedCase{
			{
				prompt:      "用一句中文总结'亡羊补牢'的寓意",
				verdictType: "judge",
				rubric: strptr("评估作答是否用一句中文准确总结了'亡羊补牢'的寓意（出了问题之后及时补救，仍然可以避免更大的损失）。完全准确 1 分，部分准确 0.5 分，错误或偏离 0 分。" + judgeRubricSuffix),
			},
			{
				prompt:      "把「他今天没来上班，因为生病了」改写成更正式的表达，只回复改写后的句子",
				verdictType: "judge",
				rubric: strptr("评估改写是否满足两点：1) 表达更正式（书面语，如「因身体不适今日未能到岗」）；2) 保留原意。两点都满足 1 分，只满足一点 0.5 分，都不满足 0 分。" + judgeRubricSuffix),
			},
			{
				prompt:      "「画蛇添足」这个成语是什么意思？用一句中文回答",
				verdictType: "judge",
				rubric: strptr("评估作答是否用一句中文准确解释了'画蛇添足'（做了多余的事，反而坏了事）。完全准确 1 分，部分准确 0.5 分，错误或偏离 0 分。" + judgeRubricSuffix),
			},
		},
	},
}
