# 55 — 拆分超行文件(eval_test.go / store/eval.go)

**What to build:** code-review LOW-5 遗留:`internal/server/eval_test.go`(~740 行,stub 与 helpers 单文件)拆出 `eval_stub_test.go`;`internal/store/eval.go`(~575 行)按 result/run 职责拆分。纯重构,行为不变,测试全绿即可。

**Blocked by:** 49 / 50a / 51 / 52 全部合入(避免与在制改动冲突)

**Status:** done

- [x] eval_test.go 拆分后 `go test ./internal/server/` 全绿
- [x] store/eval.go 拆分后全量 `make test` 绿
- [x] 无行为变更(纯移动)
