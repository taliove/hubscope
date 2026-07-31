#!/usr/bin/env python3
"""zhpunct — fullwidth-punctuation guard for Chinese materials (GH release
notes, issue bodies/comments, deployment announcements).

Modes:
  fix      Convert halfwidth punctuation to fullwidth in Chinese prose.
           Reads a file argument or stdin, writes to stdout (-i: in place).
  check    Scan only: report every halfwidth punctuation mark outside the
           protected zones as line:col, exit 1 if any is found.
  selftest Run the embedded regression cases for the protection rules.

Conversion map (applied only outside protected zones):
  : ; , ? ! ( )  ->  ： ； ， ？ ！ （ ）
  .              ->  。  (only when the previous char is CJK — version
                        numbers like v0.2.5 and decimals stay untouched)
  " / "          ->  、  (spaced-slash enumerations, spaces tightened)

Protected zones (never converted, never flagged):
  - fenced code blocks (``` ... ```)
  - inline code spans (`...`)
  - URLs (http:// or https:// up to whitespace)
  - Markdown link/image destinations: the (...) part of ](...)
"""
import re
import sys

MAP = {
    ":": "：",
    ";": "；",
    ",": "，",
    "?": "？",
    "!": "！",
    "(": "（",
    ")": "）",
}
HALFWIDTH_SET = set(":;,()?!")
FULLWIDTH_CHARS = "。「」、【】，：；？！（）"


def is_cjk(ch: str) -> bool:
    return "一" <= ch <= "鿿" or ch in FULLWIDTH_CHARS


def protected_spans(line: str):
    """Ordered (start, end) spans of the line that must stay untouched."""
    spans = []
    for m in re.finditer(r"`[^`]*`", line):
        spans.append((m.start(), m.end()))
    for m in re.finditer(r"https?://\S+", line):
        # Greedy \S+ swallows prose punctuation trailing the URL (e.g. the
        # colon after it). Parens stay: they legitimately appear in URLs.
        end = m.end()
        while end > m.start() and line[end - 1] in ":;,.!?。，；：？！":
            end -= 1
        spans.append((m.start(), end))
    for m in re.finditer(r"\]\([^)]*\)", line):
        spans.append((m.start() + 1, m.end()))
    spans.sort()
    return spans


def in_spans(i: int, spans) -> bool:
    return any(s <= i < e for s, e in spans)


def convert_line(line: str) -> str:
    spans = protected_spans(line)
    out = []
    for i, ch in enumerate(line):
        if in_spans(i, spans):
            out.append(ch)
        elif ch in MAP:
            out.append(MAP[ch])
        elif ch == "." and i > 0 and is_cjk(line[i - 1]):
            out.append("。")
        elif ch == "/" and 0 < i < len(line) - 1 and line[i - 1] == " " and line[i + 1] == " ":
            out.append("、")
        else:
            out.append(ch)
    return "".join(out).replace(" 、 ", "、")


def check_line(line: str, lineno: int, errors: list) -> None:
    spans = protected_spans(line)
    for i, ch in enumerate(line):
        if in_spans(i, spans):
            continue
        if ch in HALFWIDTH_SET:
            errors.append(f"{lineno}:{i + 1} halfwidth {ch!r}")
        elif ch == "." and i > 0 and is_cjk(line[i - 1]):
            errors.append(f"{lineno}:{i + 1} halfwidth '.' after CJK")
        elif ch == "/" and 0 < i < len(line) - 1 and line[i - 1] == " " and line[i + 1] == " ":
            errors.append(f"{lineno}:{i + 1} spaced-slash enumeration")


def process(text: str, mode: str):
    """Run one mode over the whole text, tracking fenced code blocks."""
    lines = text.splitlines(keepends=True)
    fenced = False
    out = []
    errors = []
    for n, line in enumerate(lines, 1):
        if line.lstrip().startswith("```"):
            fenced = not fenced
            out.append(line)
            continue
        if fenced:
            out.append(line)
            continue
        if mode == "fix":
            out.append(convert_line(line))
        else:
            check_line(line.rstrip("\n"), n, errors)
    return "".join(out), errors


def read_input(argv):
    # Accept the file in any position after the mode (both "fix notes.md -i"
    # and "fix -i notes.md"); flags are skipped, the first non-flag wins.
    for arg in argv[1:]:
        if arg.startswith("-"):
            continue
        with open(arg, encoding="utf-8") as f:
            return f.read(), arg
    return sys.stdin.read(), None


def selftest() -> int:
    cases_fix = [
        ("题库切换为 5 套权威基准套件:MMLU", "题库切换为 5 套权威基准套件：MMLU"),
        ("(各 100 题,全规则判分)。", "（各 100 题，全规则判分）。"),
        ("兜底(`retireV3SuitesAtOpen(x)`)收敛", "兜底（`retireV3SuitesAtOpen(x)`）收敛"),
        ("见 https://example.com/a(b): 说明", "见 https://example.com/a(b)： 说明"),
        ("自 v0.2.5 起 88 个提交", "自 v0.2.5 起 88 个提交"),
        ("MMLU / GSM8K / IFEval", "MMLU、GSM8K、IFEval"),
        ("**先备份数据库**;升级后触发", "**先备份数据库**；升级后触发"),
        ("成本是 1.5 倍", "成本是 1.5 倍"),
        ("[文档](https://x.com/a:b) 在此", "[文档](https://x.com/a:b) 在此"),
        ("监控:images_generation、images_edit", "监控：images_generation、images_edit"),
    ]
    fails = 0
    for src, want in cases_fix:
        got, _ = process(src, "fix")
        if got != want:
            fails += 1
            print(f"FAIL fix: {src!r}\n  want {want!r}\n  got  {got!r}")
    cases_check = [
        ("监控:images_generation\n备份;后触发", 2),
        ("监控：images_generation\n备份；后触发", 0),
        ("`code(x)` 与 https://a.b/c(d) 不查", 0),
        ("句号在中文后.出现", 1),
        ("版本 v0.2.5 与 1.5 不查", 0),
        ("MMLU / GSM8K", 1),
        ("```\ncode: here (x)\n```\n正文: 查", 1),
    ]
    for src, want_n in cases_check:
        _, errors = process(src, "check")
        if len(errors) != want_n:
            fails += 1
            print(f"FAIL check: {src!r}\n  want {want_n} errors, got {errors}")
    if fails:
        print(f"selftest: {fails} FAILURES")
        return 1
    print("selftest: all cases pass")
    return 0


def main() -> int:
    if len(sys.argv) < 2 or sys.argv[1] not in ("fix", "check", "selftest"):
        print(__doc__)
        return 2
    mode = sys.argv[1]
    if mode == "selftest":
        return selftest()
    text, path = read_input(sys.argv[1:])
    if mode == "fix":
        out, _ = process(text, "fix")
        if "-i" in sys.argv and path:
            with open(path, "w", encoding="utf-8") as f:
                f.write(out)
        else:
            sys.stdout.write(out)
        return 0
    _, errors = process(text, "check")
    for e in errors:
        print(e)
    if errors:
        print(f"zhpunct: {len(errors)} halfwidth punctuation issue(s)")
        return 1
    print("zhpunct: clean")
    return 0


if __name__ == "__main__":
    sys.exit(main())
