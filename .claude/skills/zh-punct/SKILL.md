---
name: zh-punct
description: 中文物料全角标点保障。写或改 GitHub Release Notes、issue 正文/评论、部署通告、ADR 中文正文等面向读者的中文说明时使用;流程 = 起草 → fix 转换 → check 校验 → 发布 → 线上回读再 check,任一 check 不过不发布。
---

# zh-punct —— 中文物料全角标点保障

## 为什么存在(事故教训,2026-07-29)

v0.3.0 Release Notes 事件:模型自以为在 shell 参数里打了全角标点,实际落盘是半角,用户指出后第一次修复仍犯同样错误,**改了两次才对**。教训:人肉打全角不可靠,必须机制化——脚本转换 + 校验闸门 + 线上回读复验。

## 强制流程(写任何中文物料时)

```
① 起草(可用半角写,不纠结)
② scripts/zhpunct.py fix  → 转换为全角
③ scripts/zhpunct.py check → 本地校验,exit≠0 回到 ②
④ 发布(gh release edit / gh issue comment / ...)
⑤ 线上回读再 check ← 关键一步,本地对了不算,发出去拉回来再扫一遍
```

### 命令示例

```bash
ZP=.claude/skills/zh-punct/scripts/zhpunct.py

# ② 转换(文件原地)
python3 $ZP fix -i notes.md

# ③ 本地校验(发现半角标点报 行:列,exit 1)
python3 $ZP check notes.md

# ⑤ 发布后的线上复验(以 Release 为例)
gh release view vX.Y.Z --json body --jq .body | python3 $ZP check

# 改动转换规则后必跑
python3 $ZP selftest
```

## 转换与保护规则(改规则 = 改脚本 + selftest 加用例)

- **转换:** `:;,!?()` → 全角;CJK 字符后的 `.` → `。`(版本号 `v0.2.5`、小数 `1.5` 不动);空格斜杠枚举 `" / "` → `、`(如 "MMLU / GSM8K" → "MMLU、GSM8K")。
- **保护区(不动不查):** 围栏代码块、行内代码(`` ` ``)、URL、Markdown 链接目标 `](...)`。
- **技术标识符保留英文原样:** suite key、函数名、issue 号(GH #40)、spec/ADR 编号——靠保护区与上下文规则自然实现,不靠人肉豁免清单。
- **范围边界:** git commit message 与 tag annotation 按仓库纪律用英文(Conventional Commits),不经过本 skill;代码与代码注释不经过本 skill。

## 适用物料清单

GitHub Release Notes、issue 正文与评论、部署通告、ADR 与 docs 中文正文、面向用户的一切说明文字。发布后第 ⑤ 步的「线上回读」对每一种都适用——凡经外部系统渲染的内容,以回读校验为准。
