package server_test

import "strings"

// answerFor decides the response text by model and prompt content.
func (h *evalStubHub) answerFor(model, prompt string) string {
	// Judge calls are recognized by the裁判 prompt marker (the judge model
	// name is configurable via settings), returning a valid JSON verdict —
	// except for the formal-rewrite case whose embedded prompt marker
	// triggers an unparseable reply. A scripted judge sequence wins over the
	// default verdict so sampling tests can mix scored and failed samples.
	if strings.Contains(prompt, "你是评估裁判") {
		if text, ok := h.nextSeq(h.judgeSeq, prompt); ok {
			return text
		}
		if strings.Contains(prompt, "改写成更正式") {
			return "I cannot produce a score for this."
		}
		return `{"score": 0.75, "reason": "meets the rubric"}`
	}
	if text, ok := h.nextSeq(h.answerSeq, prompt); ok {
		return text
	}
	h.mu.Lock()
	bad := h.bad[model]
	h.mu.Unlock()
	if bad || model == "dumb-model" {
		return "随便说点什么"
	}
	// Smart models answer every seed case correctly.
	switch {
	// Question-bank v3 (gen 3) seed answers. Markers are unique substrings of
	// the v3 prompts; none of the v3 prompts contain a legacy marker, so the
	// two banks never cross-match.
	// cap_instruction
	case strings.Contains(prompt, "张伟去年从上海"):
		return `{"name": "张伟", "city": "杭州"}`
	case strings.Contains(prompt, "桌子上放着苹果"):
		return "3"
	case strings.Contains(prompt, "the hub is healthy"):
		return "THE HUB IS HEALTHY"
	case strings.Contains(prompt, "客户订购了 2 台笔记本电脑"):
		return "笔记本电脑x2,无线鼠标x5"
	case strings.Contains(prompt, "离太阳最近的两颗行星"):
		return "| 排名 | 行星 |\n| --- | --- |\n| 1 | 水星 |\n| 2 | 金星 |"
	case strings.Contains(prompt, "年份：2100"):
		return "平年"
	case strings.Contains(prompt, "季度预算评审会定于"):
		return `{"month": 3, "day": 15, "room": 301}`
	case strings.Contains(prompt, "banana apple cherry"):
		return "apple|banana|cherry"
	case strings.Contains(prompt, "字母表中的后一个字母"):
		return "bcd"
	case strings.Contains(prompt, "人工智能正在改变世界"):
		return "10"
	// cap_reasoning
	case strings.Contains(prompt, "3 盒铅笔"):
		return "31"
	case strings.Contains(prompt, "长 8 厘米"):
		return "26"
	case strings.Contains(prompt, "100 小时后"):
		return "19"
	case strings.Contains(prompt, "火车以每秒 20 米"):
		return "30"
	case strings.Contains(prompt, "男生比女生多 6 人"):
		return "24"
	case strings.Contains(prompt, "三个连续偶数"):
		return "18"
	case strings.Contains(prompt, "相距 60 千米"):
		return "4"
	case strings.Contains(prompt, "单开进水管 6 小时"):
		return "15"
	case strings.Contains(prompt, "3 倍等于另一部分"):
		return "40"
	case strings.Contains(prompt, "咪咪是猫"):
		return "是"
	// cap_coding
	case strings.Contains(prompt, `len("hubscope")`):
		return "8"
	case strings.Contains(prompt, "7 // 2"):
		return "3"
	case strings.Contains(prompt, "typeof null"):
		return "object"
	case strings.Contains(prompt, "x * 2 for x in range(3)"):
		return "[0, 2, 4]"
	case strings.Contains(prompt, `"abc".upper()`):
		return "BC"
	case strings.Contains(prompt, `"5" + 3`):
		return "53"
	case strings.Contains(prompt, "print(f(3, 3))"):
		return "27"
	case strings.Contains(prompt, `int("abc")`):
		return "ValueError"
	case strings.Contains(prompt, "[]int{1, 2, 3, 4}"):
		return "2"
	case strings.Contains(prompt, "sum(d.values())"):
		return "3"
	// cap_knowledge (each returns the correct option letter)
	case strings.Contains(prompt, "水的化学式"):
		return "B"
	case strings.Contains(prompt, "首都是哪座城市"):
		return "C"
	case strings.Contains(prompt, "三角形内角和"):
		return "B"
	case strings.Contains(prompt, "光在真空中"):
		return "B"
	case strings.Contains(prompt, "红楼梦"):
		return "C"
	case strings.Contains(prompt, "表面积最大的器官"):
		return "B"
	case strings.Contains(prompt, "TCP 协议"):
		return "C"
	case strings.Contains(prompt, "床前明月光"):
		return "A"
	case strings.Contains(prompt, "面积最大的海洋"):
		return "C"
	case strings.Contains(prompt, "多少个节气"):
		return "B"
	// cap_language (rule cases; judge cases fall through to the default answer
	// and are scored by the stub judge)
	case strings.Contains(prompt, "差一点没摔倒"):
		return "没有摔倒"
	case strings.Contains(prompt, "难以下咽"):
		return "不满"
	case strings.Contains(prompt, "大败美国队"):
		return "中国队"
	case strings.Contains(prompt, "我、昨天、公园"):
		return "我昨天去了公园"
	case strings.Contains(prompt, "不得不离开"):
		return "B"
	case strings.Contains(prompt, "咬死了猎人的狗"):
		return "2"
	case strings.Contains(prompt, "pong"):
		return "pong"
	case strings.Contains(prompt, "严格的 JSON"):
		return `{"ok": true}`
	case strings.Contains(prompt, "数到 3"):
		return "1\n2\n3"
	case strings.Contains(prompt, "不要任何标点"):
		return "hello"
	case strings.Contains(prompt, "翻译成英文"):
		return "artificial intelligence"
	case strings.Contains(prompt, "什么是递归"):
		return "递归就是函数调用自己来解决问题的编程技巧"
	case strings.Contains(prompt, "中文大写"):
		return "四十二"
	case strings.Contains(prompt, "重复我说的话"):
		return "天气真好"
	case strings.Contains(prompt, "所有偶数"):
		return "2,4"
	case strings.Contains(prompt, "首字母大写"):
		return "Hello World"
	case strings.Contains(prompt, "3+4*2"):
		return "11"
	case strings.Contains(prompt, "「abcdef」"):
		return "fedcba"
	case strings.Contains(prompt, "17 + 25"):
		return "42"
	case strings.Contains(prompt, "游泳"):
		return "3"
	case strings.Contains(prompt, "下一个数字"):
		return "13"
	case strings.Contains(prompt, "最大的质数"):
		return "97"
	case strings.Contains(prompt, "7 的平方"):
		return "49"
	case strings.Contains(prompt, "乘以 3 等于 51"):
		return "17"
	case strings.Contains(prompt, "鸡兔同笼"):
		return "6"
	case strings.Contains(prompt, "一本书 120 页"):
		return "90"
	case strings.Contains(prompt, "等差数列"):
		return "29"
	case strings.Contains(prompt, "涨价 10%"):
		return "99"
	case strings.Contains(prompt, "log2(64)"):
		return "6"
	case strings.Contains(prompt, "有多少种选法"):
		return "10"
	case strings.Contains(prompt, "add(a, b)"):
		return "def add(a, b):\n    return a + b"
	case strings.Contains(prompt, "len([1,2,3])"):
		return "6"
	case strings.Contains(prompt, "'hello'[1]"):
		return "e"
	case strings.Contains(prompt, "2 ** 10"):
		return "1024"
	case strings.Contains(prompt, "'ab' * 3"):
		return "ababab"
	case strings.Contains(prompt, "is_even"):
		return "def is_even(n):\n    return n % 2 == 0"
	case strings.Contains(prompt, "map(x => x * 2)"):
		return "3"
	case strings.Contains(prompt, "sorted([3,1,2])"):
		return "1"
	case strings.Contains(prompt, "join(['a','b','c'])"):
		return "a,b,c"
	case strings.Contains(prompt, "reverse_string"):
		return "def reverse_string(s):\n    return s[::-1]"
	case strings.Contains(prompt, "list(range(5))"):
		return "10"
	case strings.Contains(prompt, "{'a':1,'b':2}"):
		return "2"
	default:
		return "好的"
	}
}
