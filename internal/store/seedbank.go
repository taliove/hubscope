package store

// builtinSuites is the full seed bank: the capability suites of question-bank
// v3 (seedbank_v3.go) plus the authoritative-benchmark suites
// (seedbank_benchmark.go, spec 0014, ADR 0013). The pre-v3 legacy suites were
// removed from the bank by ticket 93 (spec 0014 decision B, ADR 0012): disabled
// suites are hard-deleted at Open, so keeping them in the bank would re-seed
// what the purge deletes on every boot. Existing deployments already carry
// their generation records, and the purge removes those suites and records
// together. Benchmark suites use the same generation mechanism
// (retireAtGen) to seed disabled until the cutover (ticket 99).
var builtinSuites = append(capabilitySuites, benchmarkSuites...)
