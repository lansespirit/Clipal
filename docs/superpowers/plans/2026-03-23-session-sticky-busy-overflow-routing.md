# Session-Sticky Busy Overflow Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add session-sticky, busy-aware overflow routing so concurrency-limit `429` responses trigger bounded inline wait and controlled spillover instead of provider cooldown, while preserving cache-friendly affinity.

**Architecture:** Extend `internal/proxy` with three new runtime concepts: sticky affinity state, response/dynamic feature caches, and provider busy state. Keep existing failover/circuit/deactivation behavior for hard failures, and insert a narrow busy-aware decision layer into `forwardWithFailover` and successful response learning paths.

**Tech Stack:** Go, stdlib HTTP/JSON/time/sync, existing `internal/proxy` runtime and Go test suite.

---

### Task 1: Add Config Surface For Sticky Sessions And Busy Backpressure

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `docs/en/config-reference.md`
- Modify: `docs/zh/config-reference.md`
- Modify: `examples/config.yaml`

- [ ] **Step 1: Write the failing config tests**

Add tests in `internal/config/config_test.go` covering:

- valid defaults for:
  - `routing.sticky_sessions.enabled`
  - `routing.sticky_sessions.explicit_ttl`
  - `routing.sticky_sessions.cache_hint_ttl`
  - `routing.sticky_sessions.dynamic_feature_ttl`
  - `routing.sticky_sessions.dynamic_feature_capacity`
  - `routing.sticky_sessions.response_lookup_ttl`
  - `routing.busy_backpressure.enabled`
  - `routing.busy_backpressure.retry_delays`
  - `routing.busy_backpressure.probe_max_inflight`
  - `routing.busy_backpressure.short_retry_after_max`
  - `routing.busy_backpressure.max_inline_wait`
- validation rejects:
  - empty `retry_delays`
  - invalid durations
  - non-positive `dynamic_feature_capacity`
  - negative `probe_max_inflight`

- [ ] **Step 2: Run config tests to verify they fail**

Run: `go test ./internal/config -run 'Routing|Sticky|Busy'`
Expected: FAIL because routing config structs/defaults/validation do not exist yet.

- [ ] **Step 3: Add the routing config structs and defaults**

Implement in `internal/config/config.go`:

- `RoutingConfig`
- `StickySessionsConfig`
- `BusyBackpressureConfig`
- default values:
  - `enabled: true`
  - `explicit_ttl: 30m`
  - `cache_hint_ttl: 10m`
  - `dynamic_feature_ttl: 10m`
  - `dynamic_feature_capacity: 1024`
  - `response_lookup_ttl: 15m`
  - `retry_delays: [5s, 10s]`
  - `probe_max_inflight: 1`
  - `short_retry_after_max: 3s`
  - `max_inline_wait: 8s`

- [ ] **Step 4: Add validation and duration parsing helpers**

Extend config validation so the routing block is validated only when enabled, using the same style as the existing circuit-breaker validation.

- [ ] **Step 5: Run config tests to verify they pass**

Run: `go test ./internal/config -run 'Routing|Sticky|Busy'`
Expected: PASS.

- [ ] **Step 6: Update docs and example config**

Document the new routing block in:

- `docs/en/config-reference.md`
- `docs/zh/config-reference.md`
- `examples/config.yaml`

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/config/config.go internal/config/config_test.go docs/en/config-reference.md docs/zh/config-reference.md examples/config.yaml
git commit -m "feat: add routing config for sticky busy overflow"
```

### Task 2: Add Sticky Key Extraction And Cache Data Structures

**Files:**
- Create: `internal/proxy/sticky.go`
- Create: `internal/proxy/sticky_test.go`
- Modify: `internal/proxy/proxy.go`
- Modify: `internal/proxy/protocols.go`

- [ ] **Step 1: Write the failing extraction tests**

Add tests in `internal/proxy/sticky_test.go` covering:

- OpenAI `Responses` request extracts `L1` from `previous_response_id`
- OpenAI `Responses` request extracts `L2` from `prompt_cache_key`
- OpenAI/Anthropic/Gemini stateless payloads derive `L3` request-side feature from the second-to-last human message
- single-human-message request produces no request-side `L3`
- response-side learning feature uses the last human message
- `L3` stores hash as key material and a 24-char preview for observability

- [ ] **Step 2: Run proxy extraction tests to verify they fail**

Run: `go test ./internal/proxy -run 'Sticky|Feature|PreviousResponse|PromptCache'`
Expected: FAIL because sticky extraction/runtime helpers do not exist.

- [ ] **Step 3: Implement sticky key types and parsers**

Create `internal/proxy/sticky.go` with:

- sticky key levels (`L1`, `L2`, `L3`)
- request-side extraction result
- response-side learning result
- normalization helpers
- human-message extraction helpers for:
  - OpenAI `messages`
  - Anthropic `messages`
  - Gemini `contents`

- [ ] **Step 4: Extend `ClientProxy` runtime state**

Modify `internal/proxy/proxy.go` to add bounded runtime state:

- explicit sticky bindings
- response lookup cache
- dynamic feature cache
- supporting TTL/capacity config

Do not wire behavior yet; just initialize state and focused helpers.

- [ ] **Step 5: Run proxy extraction tests to verify they pass**

Run: `go test ./internal/proxy -run 'Sticky|Feature|PreviousResponse|PromptCache'`
Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/proxy/sticky.go internal/proxy/sticky_test.go internal/proxy/proxy.go internal/proxy/protocols.go
git commit -m "feat: add sticky key extraction primitives"
```

### Task 3: Add Busy Classification And Busy State Merging

**Files:**
- Modify: `internal/proxy/failover_classify.go`
- Modify: `internal/proxy/failover_state.go`
- Modify: `internal/proxy/proxy_test.go`

- [ ] **Step 1: Write the failing busy classification tests**

Add tests in `internal/proxy/proxy_test.go` or a new focused test file covering:

- `429` with concurrency wording and `Retry-After: 1` becomes `failureBusyRetry`
- short `Retry-After` without concurrency wording stays on cooldown path
- auth/quota masquerading as `429` still deactivate
- busy state updates merge by max and never shrink `busyUntil`

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./internal/proxy -run 'Busy|Concurrency|RetryAfter'`
Expected: FAIL because busy classification/state does not exist.

- [ ] **Step 3: Implement busy classification**

Update `internal/proxy/failover_classify.go` to:

- add `failureBusyRetry`
- require concurrency semantics for busy
- only treat short retry hints (`<= short_retry_after_max`) as busy
- preserve existing auth/quota/rate-limit classification otherwise

- [ ] **Step 4: Implement busy state and merge semantics**

Update `internal/proxy/failover_state.go` to add:

- provider/key busy state
- busy state read helpers
- busy update helpers protected by proxy mutex
- merge-by-max semantics
- busy success clear/reset helpers

- [ ] **Step 5: Run the focused tests to verify they pass**

Run: `go test ./internal/proxy -run 'Busy|Concurrency|RetryAfter'`
Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/proxy/failover_classify.go internal/proxy/failover_state.go internal/proxy/proxy_test.go
git commit -m "feat: classify and track provider busy state"
```

### Task 4: Teach Forwarding To Use Sticky Affinity, Inline Wait, And Overflow

**Files:**
- Modify: `internal/proxy/failover_forward.go`
- Modify: `internal/proxy/failover_stream.go`
- Modify: `internal/proxy/failover_manual.go`
- Modify: `internal/proxy/proxy_test.go`
- Modify: `internal/proxy/clipal_routing_test.go`

- [ ] **Step 1: Write the failing forwarding tests**

Add tests covering:

- request with `previous_response_id` stays on bound provider
- busy provider performs bounded inline wait using buffered `bodyBytes`
- probe succeeds after wait and binding stays on provider
- probe returns busy again and request overflows to next provider
- overflow rebinds `L1` session for the next request
- `L3` first-turn learning enables second-turn lookup
- single request never rereads `req.Body` after waiting
- max inline wait exceeded causes immediate overflow

- [ ] **Step 2: Run the focused forwarding tests to verify they fail**

Run: `go test ./internal/proxy -run 'Overflow|InlineWait|PreviousResponse|L3|Rebind'`
Expected: FAIL because forwarding still uses only current cursor + cooldown logic.

- [ ] **Step 3: Implement request-side resolution**

Update `internal/proxy/failover_forward.go` to:

- parse sticky extraction input from buffered `bodyBytes`
- resolve preferred provider from:
  - `L1` explicit binding / response lookup
  - `L2` cache affinity
  - `L3` dynamic feature cache
  - fallback scope cursor

- [ ] **Step 4: Implement bounded inline wait and probe**

Within the existing forwarding call path:

- wait inline with timer/select
- respect request context cancellation
- reuse `bodyBytes` for probe retry
- gate probes by busy probe inflight limit
- overflow immediately when `max_inline_wait` would be exceeded

- [ ] **Step 5: Implement overflow rebinding**

On successful overflow:

- persist `L1` rebinding durably
- update `L2`/`L3` bounded affinity caches
- do not mutate sticky state on failed or partial attempts

- [ ] **Step 6: Run the focused forwarding tests to verify they pass**

Run: `go test ./internal/proxy -run 'Overflow|InlineWait|PreviousResponse|L3|Rebind'`
Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/proxy/failover_forward.go internal/proxy/failover_stream.go internal/proxy/failover_manual.go internal/proxy/proxy_test.go internal/proxy/clipal_routing_test.go
git commit -m "feat: add sticky busy-aware overflow routing"
```

### Task 5: Learn From Successful Responses And Preserve State Across Reload

**Files:**
- Modify: `internal/proxy/failover_stream.go`
- Modify: `internal/proxy/proxy.go`
- Modify: `internal/proxy/reload_state_test.go`
- Modify: `internal/proxy/sticky_test.go`

- [ ] **Step 1: Write the failing learning/reload tests**

Add tests covering:

- successful response writes response object `id` into response lookup cache
- successful response writes learned `L3` feature into dynamic feature cache
- partial/interrupted response does not learn
- reload inherits sticky bindings, response lookup entries, dynamic feature cache, and busy state when provider identity matches
- reload drops those entries when provider identity changes

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./internal/proxy -run 'Learn|Reload|Lookup|DynamicFeature'`
Expected: FAIL because response learning/reload inheritance is incomplete.

- [ ] **Step 3: Implement response learning hooks**

Update `internal/proxy/failover_stream.go` to invoke a success-only learning hook after Clipal confirms completion.

- [ ] **Step 4: Implement runtime inheritance**

Update `internal/proxy/proxy.go` inheritance helpers so reload preserves:

- sticky bindings
- response lookup cache
- dynamic feature cache
- busy state

only when provider runtime identity still matches.

- [ ] **Step 5: Run the focused tests to verify they pass**

Run: `go test ./internal/proxy -run 'Learn|Reload|Lookup|DynamicFeature'`
Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/proxy/failover_stream.go internal/proxy/proxy.go internal/proxy/reload_state_test.go internal/proxy/sticky_test.go
git commit -m "feat: persist sticky affinity across responses and reloads"
```

### Task 6: Expose Observability And Run Full Regression Suite

**Files:**
- Modify: `internal/proxy/status.go`
- Modify: `internal/proxy/presentation.go`
- Modify: `internal/proxy/presentation_test.go`
- Modify: `internal/web/types.go`
- Modify: `internal/web/api_test.go`
- Modify: `release-notes/v0.7.0.md`

- [ ] **Step 1: Write the failing observability tests**

Add tests covering:

- runtime snapshot exposes busy fields and sticky/cache counts
- provider presentation renders busy state distinctly from cooldown
- web API serializes new fields safely

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./internal/proxy ./internal/web -run 'Busy|Sticky|Snapshot|Presentation'`
Expected: FAIL because runtime/web output lacks the new fields.

- [ ] **Step 3: Implement runtime and presentation updates**

Update status and presentation layers to expose:

- provider busy metadata
- sticky binding count
- response lookup count
- dynamic feature cache count
- busy overflow wording

- [ ] **Step 4: Update release notes**

Add a short user-facing summary to `release-notes/v0.7.0.md`.

- [ ] **Step 5: Run full regression suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/proxy/status.go internal/proxy/presentation.go internal/proxy/presentation_test.go internal/web/types.go internal/web/api_test.go release-notes/v0.7.0.md
git commit -m "feat: expose sticky busy routing runtime status"
```
