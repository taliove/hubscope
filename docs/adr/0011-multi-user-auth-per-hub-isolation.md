# ADR 0011 — Multi-user auth and per-Hub isolation

**Date:** 2026-07-23
**Status:** Accepted (in-flight — auth/session landed in ticket 61; isolation sweep, audit `hub_id`, user management UI, and CLI bootstrap tracked by tickets 62–69)
**Supersedes:** the single-admin-password model documented in ADR-0001's auth section and the original W6 wording.
**Spec:** [docs/specs/0005-multi-user-auth-per-hub-isolation.md](../specs/0005-multi-user-auth-per-hub-isolation.md)
**Revises load-bearing wall:** W6 (凭证边界).

## Context

The original auth model (ADR-0001) was a single admin password: `ADMIN_PASSWORD` env var → plaintext in memory → stateless session cookie whose signing key was derived from the password (`auth.go` legacy). Audit `actor` was hardcoded `"admin"`, there was no users table, no roles, and no data isolation across hubs. This was sufficient for a single operator but blocks:

- team multi-user concurrent use with per-actor audit traceability;
- per-Hub data isolation in multi-Hub deployments;
- rotation of the session signing key without changing the login password;
- role separation (read-only viewers, hub-scoped operators).

Ticket 61 landed the first half of the new model: a `users` table (bcrypt password hashes), an independent `SESSION_SECRET` (env var or auto-generated in the `settings` table), a session token of the form `<userId>.<issuedUnix>.<hmac>`, `server.New(db, opts...)` with no `adminPassword` parameter, `/api/auth/login` accepting `{username, password}`, and `/api/auth/me` returning the user identity. The `ADMIN_PASSWORD` env var is now read only to emit a deprecation warning (transitional window); it no longer gates startup.

## Decision

The full target model, per spec 0005:

1. **Four roles.** `super_admin` (global, CLI-created, unbound to a Hub) / `admin` (full power within a Hub, including user management for that Hub) / `operator` (Hub-scoped operations, no user management) / `viewer` (Hub-scoped read-only console). `super_admin` creates hubs and assigns each hub's first `admin`, closing the cross-Hub role seam that per-Hub isolation would otherwise open.
2. **Bootstrap.** A CLI subcommand `hubscope admin create` creates the first `super_admin` (bcrypt hash, `hub_id = NULL`). `ADMIN_PASSWORD` is deprecated; it is hard-deleted once the CLI lands (tracked by ticket 69). The env var is **not** used to auto-create a user — a password is not a username, and auto-creating from env is a security hole.
3. **Per-Hub isolation.** Every user except `super_admin` is bound to a `hub_id`; their role takes effect only within that Hub; after login they see and manage only their own Hub's data.
4. **Session signing key.** Independent `SESSION_SECRET` env var; if absent on first start, a 32-byte hex secret is generated into the `settings` table and reused across restarts. Rotation invalidates every session. The token carries the user id; HMAC verifies identity.
5. **Public route dual semantics** (handler branches on session):
   - `/api/overview`: anonymous = global aggregate (public status board unchanged); logged-in non-`super_admin` = own-Hub aggregate; `super_admin` = global.
   - `/api/endpoints/{id}` (and `/series`, `/probes`, `/eval-summary`): public detail stays globally accessible (status is public; cross-Hub readable).
6. **Global-resource scope** (JOIN-through-model does not hold for these; scoped separately):
   - `settings` / `classification_rules` / `suites` / `cases`: global; writes are `super_admin`-only, Hub-level users read-only.
   - `audit_logs`: gains a `hub_id` column; `s.audit` writes the user's `hub_id` from ctx; actions with no hub affiliation (`auth.login` / `auth.logout` / `settings.update` / `discovery.*`) write NULL, visible only to `super_admin`. Historical rows backfill NULL (equivalent to `super_admin`-visible; old data stays readable).
   - `tasks`: filtered by `entity_type` JOIN (`eval_run`→`campaign`→`campaign_models`→`models.hub_id`; `hub`→`entity_id` directly); rollup/retention tasks with no Hub affiliation are `super_admin`-visible.
   - `hubs`: `super_admin` sees all; Hub-level users see only their own Hub (needed by the frontend Hub switcher).
7. **Question bank is global.** `suites` / `cases` are consistent across hubs (an extension of W7 immutability — only a shared question bank makes cross-Hub comparison meaningful). `campaigns` / `eval_runs` are naturally per-Hub-isolated via `campaign_models`→`models.hub_id`; the JOIN holds.
8. **Background jobs do not change isolation logic.** `discovery` / `prober` / `evaluator` / `alerter` / `scheduler` write by `entity_id` precisely, do not pass through the session, and do no cross-Hub aggregate reads. Background jobs do **not** write `audit_logs` (grep verified zero hits); they use the `tasks.source` field. The `s.audit` call sites are all inside HTTP handlers, all holding `r`, so the actor is read directly from ctx — no "system fallback" path is needed.
9. **Structural isolation enforcement** (replaces a pure test guardrail): store-layer `List*` functions lose their no-arg form; only `ListXByHub(hubID int64)` + `ListXAll()` (reachable only on the `super_admin` path) remain. Forgetting to pass the hub filter is a compile error, structurally eliminating "forgot to filter". The isolation sweep test is a runtime second line of defense.

## Consequences

**Positive:**
- Multiple users can use HubScope concurrently with per-actor audit.
- Per-Hub data isolation supports multi-Hub deployments.
- `SESSION_SECRET` is independently rotatable without changing login passwords.
- Role separation (viewer/operator/admin/super_admin) matches team workflows.

**Negative:**
- No SSO/OIDC this iteration (local accounts only).
- No inter-Hub data sharing/replication (each Hub is an independent dataset).
- Bootstrap depends on the CLI `hubscope admin create` — until ticket 69 lands, the `ADMIN_PASSWORD` deprecation warning must remain (it is the only fallback signal pointing operators at the not-yet-built CLI; the warning itself is cosmetic and does not gate startup).
- `audit_logs.hub_id` is a schema change absorbed late (spec 0005 corrected the original "Phase 4 has no schema change" claim).

**Neutral:**
- The transitional `ADMIN_PASSWORD` deprecation window closes once the CLI lands in ticket 69; the hard-delete of the env-var read path moves to 69.

## Migration

Per spec 0005:

1. Stop service → upgrade binary.
2. `hubscope admin create --username admin --password <new password>` (no `--hub` → `super_admin`). **CLI delivered by ticket 69.**
3. Set `SESSION_SECRET` (or omit to auto-generate).
4. Remove `ADMIN_PASSWORD` (hard-ignored once ticket 69 lands; during the transition window, if still set, a deprecation warning is logged without error).
5. Start (`ADDR=:8080` default; production customizes).
6. `super_admin` logs in → creates/audits Hubs → assigns each Hub's first `admin`.

## W6 four questions (this ADR revises W6)

1. **Why must W6 change?** The old wording ("admin password only via `ADMIN_PASSWORD` env") no longer matches the implementation reality since ticket 61: auth is users-table + bcrypt + independent `SESSION_SECRET`. Leaving W6 stale misleads every subsequent agent and reviewer into assuming the single-password model still holds; W6 is the anchor that ADR 0011, the deployment docs, and the release-check skill cross-reference.
2. **Which callers are affected?** Direct: `.claude/rules/load-bearing-walls.md` W6 entry. Indirect: `CLAUDE.md` (references the W6 list), `docs/specs/0005` (W6 revision section, already written to the target state), `.claude/skills/release-check/SKILL.md`, `docs/deployment.md`, `README.md`. W6 is a documentation wall, not a code symbol — no code call sites.
3. **Alternatives?** None. W6 revision is the documentation-implementation reconciliation step; an adapter layer is meaningless for a documentation wall.
4. **Regression testing?** L3 full `make test` (backend + frontend typecheck + build). The W6 text change has no unit test of its own; the auth/CLI behavior it depends on is covered by tickets 61 and 69. Ticket 68 is documentation-only — `make test` must stay green with zero code changes.

## Known gaps (honest record)

- **CLI bootstrap not yet implemented.** `hubscope admin create` is referenced by the main.go deprecation warning but the subcommand does not exist in `cmd/hubscope/` (no `admin.go`, no `os.Args`/`flag.` subcommand branch, no CLI caller of `store.CreateUser`). This is the reason ticket 68 is documentation-only and the `ADMIN_PASSWORD` hard-delete is deferred to ticket 69. Until 69 lands, the deprecation warning is cosmetic-only (it does not gate startup, and the CLI it points at does not yet exist).
- **Isolation sweep not yet implemented.** spec 0005 specifies `internal/server/isolation_test.go` (seed two hubs, log in as a Hub-A user, assert no list endpoint returns Hub-B data; `super_admin` sees both; new list endpoints must add a corresponding assertion line). The structural `List*ByHub` / `List*All` split (decision 9) and the runtime sweep test are both pending — tracked by tickets 64–65.
- **65b cross-Hub campaign write path.** The campaign/eval write path is not yet isolation-hardened: a handler could currently accept a cross-Hub `campaign_id` / `eval_run_id` without verifying it belongs to the session user's hub on the write side (the read side is filtered by 64b/65). This is a known gap recorded here for honesty; the isolation sweep (tickets 64–65) and any 65b follow-up must cover write-path ownership checks, not just read filtering.

## Out of scope

Per spec 0005: multi-tenant billing/quota; inter-Hub data sharing/replication; SSO/OIDC; `endpoints` table denormalized `hub_id` (only if JOIN-through proves slow); API tokens for service-to-service auth (session cookie reused).
