package store

// capabilitySuites is the question-bank v3 seed (ticket 50, ADR 0010): one
// suite per capability dimension, each carrying 10 first-issue cases across
// three difficulty tiers. Cases are deterministic where possible — rule
// verdicts (exact/regex/contains) with unambiguous expected answers, no
// "reply pong" style toy prompts — and judge cases follow the 1/0.5/0 rubric
// convention with judgeRubricSuffix. Per ADR 0010 judge cases stay a minority
// (4 of 50 here, 8% <= the 40% cap) with sampleCount 3; rule cases pin
// sampleCount 1. cap_knowledge is four-option multiple choice throughout, so
// its nadir is the random-guess floor 0.25; the other suites floor at 0.
//
// Suite order matters: cap_language is last so it is the final suite a full
// sweep executes (the only suite containing judge cases), which keeps
// mid-sweep freeze scenarios deterministic in tests.
//
// PENDING HUMAN REVIEW: these are LLM-generated first-issue candidates meant
// to bootstrap each capability. Ticket 50's full bank (30-50 cases per
// capability) goes through manual review before replacing them.
//
// RETIRED AT THE BENCHMARK CUTOVER (ticket 99, spec 0014 decision C): the
// authoritative-benchmark bank replaced these self-written suites. Every
// suite below carries retireAtGen 4 — strictly greater than the bank's
// highest case generation (3), so the generation-tracked retirement fires on
// every database that ever received the v3 seed (a retireAtGen of 1 would
// never trigger: 1 > received(3) is false — and would collide with the
// purge's seeds-disabled-by-design exemption, double-silencing the
// retirement). The same Open's disabled-suite purge (ADR 0012) then deletes
// the suites with their cases, runs and results, and tombstones them against
// re-seeding; this bank stays listed only so existing databases learn the
// retirement.
//
// builtinSuites is composed in seedbank.go (capability suites only since
// ticket 93 — the pre-v3 legacy bank was removed when disabled suites became
// hard-deleted, ADR 0012).
var capabilitySuites = []seedSuite{
	{
		key:         "cap_instruction",
		name:        "指令遵循",
		capability:  CapabilityInstruction,
		nadir:       0,
		retireAtGen: 4, // retired at the benchmark cutover; see file comment
		cases: []seedCase{
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "从以下文本中抽取姓名和城市，用严格的 JSON 回复 {\"name\": \"...\", \"city\": \"...\"}，不要任何其他文字。文本：张伟去年从上海搬到了杭州，现在他在杭州工作。",
				verdictType:  "rule",
				ruleMode:     strptr("regex"),
				ruleExpected: strptr(`^\s*\{\s*"name"\s*:\s*"张伟"\s*,\s*"city"\s*:\s*"杭州"\s*\}\s*$`),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "下面这段话里提到了几种水果？只回复数字。文本：桌子上放着苹果、香蕉和橙子三种水果。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("3"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "把下面这句英文改写成全大写，只回复结果：the hub is healthy",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("THE HUB IS HEALTHY"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "从下面的订单描述中抽取商品和数量，按「商品x数量」的格式回复，多个商品之间用英文逗号分隔，不要任何其他内容：客户订购了 2 台笔记本电脑和 5 个无线鼠标。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("笔记本电脑x2,无线鼠标x5"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "用 Markdown 表格列出太阳系中离太阳最近的两颗行星，表格只含「排名」和「行星」两列，不要任何其他文字。",
				verdictType:  "rule",
				ruleMode:     strptr("regex"),
				ruleExpected: strptr(`\|\s*1\s*\|\s*水星\s*\|[\s\S]*\|\s*2\s*\|\s*金星\s*\|`),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "阅读下面的规则并作答：如果给出的年份是闰年，回复「闰年」，否则回复「平年」。年份：2100",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("平年"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "从下面的通知中抽取会议的月份、日期和会议室号，用严格的 JSON 回复 {\"month\": 数字, \"day\": 数字, \"room\": 数字}，不要任何其他文字。通知：季度预算评审会定于 3 月 15 日下午 3 点在 301 会议室召开。",
				verdictType:  "rule",
				ruleMode:     strptr("regex"),
				ruleExpected: strptr(`^\s*\{\s*"month"\s*:\s*3\s*,\s*"day"\s*:\s*15\s*,\s*"room"\s*:\s*301\s*\}\s*$`),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "把下面三个英文单词按字典序排列，用竖线分隔，只回复结果：banana apple cherry",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("apple|banana|cherry"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "把字符串「abc」中的每个字母替换为它在字母表中的后一个字母，只回复结果。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("bcd"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "下面这句话共有多少个汉字（不含标点）？只回复数字。句子：人工智能正在改变世界",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("10"),
			},
		},
	},
	{
		key:         "cap_reasoning",
		name:        "推理",
		capability:  CapabilityReasoning,
		nadir:       0,
		retireAtGen: 4, // retired at the benchmark cutover; see file comment
		cases: []seedCase{
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "小明有 3 盒铅笔，每盒 12 支，他送出 5 支后还剩多少支？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("31"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "一个长方形长 8 厘米、宽 5 厘米，它的周长是多少厘米？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("26"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "现在是 15 时，100 小时后是几点？只回复 24 小时制的小时数。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("19"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "一列长 200 米的火车以每秒 20 米的速度通过一座 400 米长的桥，从车头进桥到车尾离桥共需多少秒？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("30"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "某班男生比女生多 6 人，全班共 42 人，男生有多少人？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("24"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "三个连续偶数的和是 48，其中最大的数是多少？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("18"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "甲乙两地相距 60 千米，甲以 7 千米/时、乙以 8 千米/时从两地同时出发相向而行，几小时后相遇？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("4"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "一个水池单开进水管 6 小时注满，单开出水管 10 小时放空，两管同时打开，多少小时能注满空池？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("15"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "把 100 分成两部分，使一部分的 3 倍等于另一部分的 2 倍，较小的一部分是多少？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("40"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "所有猫都是动物，咪咪是猫。根据这两句话，咪咪一定是动物吗？只回复「是」或「否」。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("是"),
			},
		},
	},
	{
		key:         "cap_coding",
		name:        "代码",
		capability:  CapabilityCoding,
		nadir:       0,
		retireAtGen: 4, // retired at the benchmark cutover; see file comment
		cases: []seedCase{
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "Python 表达式 len(\"hubscope\") 的值是多少？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("8"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "下面 Python 代码的输出是什么？只回复结果：print(7 // 2)",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("3"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "JavaScript 表达式 typeof null 的值是什么？只回复结果。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("object"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "下面 Python 代码的输出是什么？只回复结果：print([x * 2 for x in range(3)])",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("[0, 2, 4]"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "下面 Python 代码的输出是什么？只回复结果：print(\"abc\".upper()[1:])",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("BC"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "下面 JavaScript 代码的输出是什么？只回复结果：console.log(\"5\" + 3)",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("53"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "下面 Python 代码的输出是什么？只回复结果：def f(a, b=2):\n    return a ** b\nprint(f(3, 3))",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("27"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "下面 Python 表达式会抛出什么类型的异常？只回复异常类名：int(\"abc\")",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("ValueError"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "下面 Go 代码的输出是什么？只回复数字：fmt.Println(len([]int{1, 2, 3, 4}[1:3]))",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("2"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "下面 Python 代码的输出是什么？只回复数字：d = {\"a\": 1}\nd[\"b\"] = 2\nprint(sum(d.values()))",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("3"),
			},
		},
	},
	{
		key:        "cap_knowledge",
		name:       "知识问答",
		capability: CapabilityKnowledge,
		// Four-option multiple choice throughout: the random-guess floor is
		// 0.25, so the suite's nadir is calibrated to it (ADR 0009).
		nadir:       0.25,
		retireAtGen: 4, // retired at the benchmark cutover; see file comment
		cases: []seedCase{
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "水的化学式是什么？只回复选项字母。A. CO2 B. H2O C. O2 D. NaCl",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("B"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "中华人民共和国的首都是哪座城市？只回复选项字母。A. 上海 B. 广州 C. 北京 D. 深圳",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("C"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "三角形内角和是多少度？只回复选项字母。A. 90 度 B. 180 度 C. 270 度 D. 360 度",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("B"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "光在真空中的传播速度约为每秒多少千米？只回复选项字母。A. 3 万千米 B. 30 万千米 C. 300 万千米 D. 3000 万千米",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("B"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "《红楼梦》的作者是谁？只回复选项字母。A. 罗贯中 B. 施耐庵 C. 曹雪芹 D. 吴承恩",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("C"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "人体表面积最大的器官是什么？只回复选项字母。A. 肝脏 B. 皮肤 C. 心脏 D. 肺",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("B"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "TCP 协议工作在 OSI 参考模型的哪一层？只回复选项字母。A. 物理层 B. 数据链路层 C. 传输层 D. 应用层",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("C"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "「床前明月光」的下一句是什么？只回复选项字母。A. 疑是地上霜 B. 举头望明月 C. 低头思故乡 D. 月是故乡明",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("A"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "世界上面积最大的海洋是哪个？只回复选项字母。A. 大西洋 B. 印度洋 C. 太平洋 D. 北冰洋",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("C"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "一年有多少个节气？只回复选项字母。A. 12 个 B. 24 个 C. 36 个 D. 48 个",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("B"),
			},
		},
	},
	{
		key:         "cap_language",
		name:        "语言理解与生成",
		capability:  CapabilityLanguage,
		nadir:       0,
		retireAtGen: 4, // retired at the benchmark cutover; see file comment
		cases: []seedCase{
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "「他差一点没摔倒」这句话的意思是他摔倒了还是没有摔倒？只回复「摔倒了」或「没有摔倒」。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("没有摔倒"),
			},
			{
				gen: 3, difficulty: "basic", sampleCount: intptr(1),
				prompt:       "「这家餐厅的菜真是难以下咽」表达了说话人怎样的态度？只回复「满意」或「不满」。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("不满"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "「中国队大败美国队」这句话中获胜的是哪一方？只回复「中国队」或「美国队」。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("中国队"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "把「我、昨天、公园、去了」这几个词排成一个通顺的句子，只回复排好的句子，不要加标点。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("我昨天去了公园"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(1),
				prompt:       "下面哪句话与「他不得不离开」意思相同？只回复选项字母。A. 他主动离开 B. 他必须离开 C. 他拒绝离开",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("B"),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(1),
				prompt:       "「咬死了猎人的狗」这句话有几种不同的理解？只回复数字。",
				verdictType:  "rule",
				ruleMode:     strptr("exact"),
				ruleExpected: strptr("2"),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(3),
				prompt:      "阅读下面的文字，用不超过 30 个字总结其主要内容：随着远程办公的普及，越来越多的公司开始重新思考办公室的意义。一些企业选择完全放弃实体办公室，转向全远程模式；另一些企业则采取混合办公，每周只要求员工到岗两三天。研究表明，灵活的办公安排能提升员工满意度，但也给团队协作和企业文化带来了新的挑战。",
				verdictType: "judge",
				rubric:      strptr("评估摘要是否满足三点：1) 不超过 30 个字；2) 涵盖「远程或混合办公日益普及」这一背景；3) 提到办公模式变化给团队协作或企业文化带来挑战。三点全满足 1 分，满足两点 0.5 分，其余 0 分。" + judgeRubricSuffix),
			},
			{
				gen: 3, difficulty: "intermediate", sampleCount: intptr(3),
				prompt:      "把下面这段口语通知改写成规范的书面通知，只回复改写后的文字：大伙儿注意了啊，明天下午三点开会，都别迟到，带上上个月的数据。",
				verdictType: "judge",
				rubric:      strptr("评估改写是否满足三点：1) 使用规范书面语、语气正式；2) 保留会议时间信息（明天下午三点）；3) 保留携带材料要求（上个月的数据）。三点全满足 1 分，满足两点 0.5 分，其余 0 分。" + judgeRubricSuffix),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(3),
				prompt:      "Read the passage and summarize it in one Chinese sentence of at most 40 characters: \"Electric vehicles have grown from a niche product into a mainstream choice. Falling battery costs, expanding charging networks, and government incentives have driven adoption, while concerns about range and resale value continue to hold back some buyers.\"",
				verdictType: "judge",
				rubric:      strptr("评估摘要是否满足三点：1) 一句中文且不超过 40 字；2) 提到电动车普及的驱动因素（电池成本下降、充电网络扩展、政策激励，至少其一）；3) 提到仍然存在的顾虑（续航或保值）。三点全满足 1 分，满足两点 0.5 分，其余 0 分。" + judgeRubricSuffix),
			},
			{
				gen: 3, difficulty: "hard", sampleCount: intptr(3),
				prompt:      "根据以下要点写一段 50 字左右的中文产品介绍：产品是一款保温杯，要点包括保温 12 小时、容量 500 毫升、杯盖可以当水杯用。",
				verdictType: "judge",
				rubric:      strptr("评估文案是否满足三点：1) 覆盖全部三个要点（保温 12 小时、容量 500 毫升、杯盖可作水杯）；2) 篇幅 40 到 70 字；3) 中文通顺、有产品宣传感。三点全满足 1 分，满足两点 0.5 分，其余 0 分。" + judgeRubricSuffix),
			},
		},
	},
}
