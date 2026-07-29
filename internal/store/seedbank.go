package store

// builtinSuites is the full seed bank: the capability suites of question-bank
// v3 (seedbank_v3.go) plus the authoritative-benchmark suites
// (seedbank_benchmark.go, spec 0014, ADR 0013). The pre-v3 legacy suites were
// removed from the bank by ticket 93 (spec 0014 decision B, ADR 0012): disabled
// suites are hard-deleted at Open, so keeping them in the bank would re-seed
// what the purge deletes on every boot. Existing deployments already carry
// their generation records, and the purge removes those suites and records
// together. The v3 capability suites stay in the bank with retireAtGen 4 so
// existing databases learn their retirement at the ticket-99 cutover and the
// purge deletes them; the benchmark suites seed enabled (retireAtGen 0) and
// databases that pre-seeded them disabled are flipped by the one-time cutover
// migration (cutover.go).
var builtinSuites = append(capabilitySuites, benchmarkSuites...)
