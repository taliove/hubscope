package store

// builtinSuites is the full seed bank: the capability suites of question-bank
// v3 (seedbank_v3.go). The pre-v3 legacy suites were removed from the bank by
// ticket 93 (spec 0014 decision B, ADR 0012): disabled suites are hard-deleted
// at Open, so keeping them in the bank would re-seed what the purge deletes on
// every boot. Existing deployments already carry their generation records, and
// the purge removes those suites and records together.
var builtinSuites = capabilitySuites
