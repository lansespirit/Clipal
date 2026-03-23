# Session-Sticky Busy Overflow Routing

Date: 2026-03-23

## Summary

Upgrade Clipal routing from failure-driven provider switching to session-sticky routing with black-box congestion handling.

The new behavior treats concurrency-limit `429` responses as a soft-capacity signal instead of a provider health failure:

- keep a session bound to one provider when possible
- pause and retry briefly when that provider reports concurrency saturation
- avoid sending a stampede of new requests to the same saturated provider
- overflow the affected session to the next candidate provider only after controlled retry
- rebind the session to the overflow target so later requests stay sticky there

This change must not degrade existing auth/quota/circuit-breaker semantics.

## Goals

- Preserve same-session affinity to maximize upstream cache reuse.
- Stop interpreting concurrency saturation as provider breakage.
- Support black-box upstreams where actual concurrency limits are unknown and may differ per provider.
- Reduce stampedes when one provider begins returning concurrency-limit `429`.
- Keep current provider failover behavior for hard failures and general transient errors.

## Non-Goals

- Do not infer a fake universal session identifier when the protocol does not provide one.
- Do not implement cluster-wide shared affinity state across multiple Clipal processes.
- Do not add queue persistence across process restarts.
- Do not replace the existing circuit breaker with a generic load balancer.

## Current Problem

Today a `429` classified as `rate_limit` causes provider-level cooldown in the normal failover path. That behavior is reasonable for quota or long reset windows, but it is wrong for concurrency saturation:

- the provider is still healthy
- only some requests should wait or spill
- current logic moves the global routing cursor away from the provider
- later requests lose affinity and may scatter across providers

The runtime has health-oriented state only:

- provider deactivation
- key deactivation
- circuit breaker
- scope-local current provider cursors

It does not model:

- session affinity ownership
- provider busy/backpressure state
- controlled retry before spillover

## External API Reality

There is no universal cross-provider session identifier. Sticky extraction must be capability-specific.

Strong reusable request-side identifiers:

- OpenAI `Responses`: `previous_response_id`
- Anthropic tool containers: `container`
- Gemini cached context: `cached_content`

Useful but weaker affinity hints:

- OpenAI `prompt_cache_key`

No strong reusable session key in common stateless flows:

- OpenAI `chat/completions`
- Anthropic standard `messages`
- Gemini plain `generateContent`

Response IDs should not be treated as a universal session key. They may still be useful as short-lived lookup material for future request-side chaining such as `previous_response_id`.

## High-Level Design

Add a new routing layer between scope selection and upstream attempt execution:

1. derive an optional sticky key from the incoming request
2. resolve the preferred provider for that sticky key
3. consult provider/key health and provider busy state
4. if preferred provider is busy, wait and retry in a controlled way
5. if retry still encounters concurrency saturation, overflow to the next candidate
6. rebind the sticky key to the overflow target
7. continue normal sticky routing on later requests

This introduces a new state category:

- `busy`: provider has recently reported concurrency saturation and should receive only limited probe traffic until the busy window expires

`busy` is not `deactivated`.

## Runtime State Model

### 1. Sticky Session Bindings

Maintain per-client, per-scope sticky bindings:

- key: `routingScope + stickyKey`
- value:
  - provider index
  - key index if selection is key-specific
  - bound at
  - last seen at
  - source type
  - overflow generation count

Bindings are in-memory and expire after idle TTL.

Recommended defaults:

- strong session key idle TTL: `30m`
- weak affinity hint idle TTL: `10m`

Only strong keys are allowed to trigger session rebinding. Weak hints can influence preferred provider choice but should not create long-lived ownership.

### 2. Response Lookup Cache

Maintain a short-lived map from response-like IDs to provider ownership so chained APIs can recover affinity.

- key: provider-specific response object ID
- value:
  - provider index
  - scope
  - observed at

Recommended TTL:

- `15m`

Usage:

- if a new request references a known object such as `previous_response_id`, resolve affinity directly
- otherwise ignore

### 3. Busy State

Maintain provider-level busy state, separate from cooldown and circuit breaker:

- `busyUntil`
- `busyBackoffStep`
- `lastBusyReason`
- `probeInFlight`
- optional per-key busy state if a provider has multiple keys

Recommended default behavior:

- first concurrency-limit `429` sets busy window to `5s`
- second consecutive busy signal extends to `10s`
- cap busy backoff at `10s`
- allow only `1` probe request per provider while leaving busy state

Busy state decays on success:

- a successful response from the provider clears busy window and resets backoff step

## Sticky Key Extraction

Implement capability-specific extractors. Extraction returns:

- key string
- strength: `strong` or `weak`
- source label for observability

### Strong Extractors

- OpenAI `Responses`
  - request `previous_response_id`
- Anthropic messages with tool container reuse
  - request `container`
- Gemini cached context
  - request `cached_content`

### Weak Extractors

- OpenAI `Responses`
  - request `prompt_cache_key`

### No Extractor

For capabilities without a strong request-side identifier, return no sticky key.

Fallback behavior in that case:

- use existing scope-local current-provider behavior
- apply busy-aware spillover to the current request
- do not create long-lived session ownership

## Concurrency-Limit Classification

Split current `429` handling into:

- hard unavailable
  - auth masquerading as `429`
  - quota exhaustion
  - long reset windows that should still deactivate key/provider
- soft busy
  - concurrency saturation
  - short-lived overload where retry is expected to work soon

Add a new classification result:

- `failureBusyRetry`

Classification rules:

1. preserve current auth/quota detection first
2. treat explicit concurrency phrases as `busy`
3. treat short `Retry-After` windows as `busy`
4. keep existing `rate_limit` cooldown behavior for long reset windows

Examples of `busy` indicators:

- error message contains `concurrent`
- error message contains `too many concurrent`
- error message contains `capacity`
- provider-specific overload types when semantics imply near-term retry
- `Retry-After <= 10s`

Examples of normal cooldown indicators:

- `Retry-After` much larger than the busy cap
- explicit rate/token/request budget exhaustion without concurrency wording

This split lets Clipal avoid poisoning provider health when the upstream only wants brief load shedding.

## Request Handling State Machine

### Path A: No Sticky Key

1. use current scope cursor to choose a preferred provider
2. if provider is not busy, attempt normally
3. if provider is busy, wait for the busy window only once for this request
4. retry on the same provider through the busy probe gate
5. if busy again, spill to next candidate for this request only
6. do not create long-lived session binding

### Path B: Strong Sticky Key Exists

1. resolve bound provider from sticky map if present
2. otherwise choose provider using current scope cursor and create binding
3. if bound provider is busy, queue the request behind its busy window
4. when busy window elapses, retry through the provider busy probe gate
5. if request succeeds, keep binding unchanged
6. if request returns busy again after controlled retry, overflow to next candidate
7. on successful overflow, rebind the sticky key to the new provider

### Path C: Weak Sticky Hint Exists

1. prefer historical provider if known
2. never rely on weak key as durable ownership
3. allow request-local overflow without persistent rebinding unless a later strong key appears

## Controlled Queueing

Queueing should be local and bounded by the request context.

Rules:

- do not send immediate retries while `busyUntil` is in the future
- waiting must respect client cancellation and request deadline
- only one probe request per provider should be allowed to test recovery when the busy window expires
- other requests should continue waiting briefly or spill, depending on remaining context budget

Recommended default request behavior:

- wait until `busyUntil`
- attempt one probe on the preferred provider
- if probe returns busy again, update busy window to next step and overflow immediately

This achieves the desired user experience:

- `429 busy -> wait 5s -> retry -> if still busy, overflow`
- later requests in the same session stick to the overflow target

## Spillover Selection

Overflow selection should reuse the existing scope-aware candidate order, excluding:

- disabled providers
- deactivated providers
- providers with no active keys
- providers blocked by open circuit
- providers currently in an active busy window, unless every remaining candidate is also busy

Selection order:

1. continue using current provider priority ordering
2. prefer non-busy candidates
3. if all remaining candidates are busy, choose the one with the earliest `busyUntil`

This preserves existing admin expectations around provider order.

## Interaction With Multi-Key Providers

Busy handling should be key-aware before escalating to provider-wide busy.

Recommended behavior:

1. if a request on one key returns busy, mark that key busy for the same busy window
2. try another active key in the same provider if available
3. only mark the provider busy when all currently usable keys are busy or the provider-level signal is clearly shared

This preserves Clipal's current "retry next key before next provider" behavior.

## Interaction With Existing Health Logic

Keep current semantics unchanged for:

- `401` / `403` auth failures
- `402` billing failures
- quota-style `429`
- transport failures
- `5xx`
- idle timeout
- incomplete protocol stream

Mapping:

- auth/billing/quota -> deactivation as today
- network/server/idle/protocol -> circuit breaker as today
- concurrency busy -> busy state only

Busy events must not:

- advance the global scope cursor permanently
- open the circuit breaker
- mark provider deactivated

The cursor should only move permanently on successful overflow rebinding or on existing hard-failure logic.

## Observability

Extend runtime status and logs with:

- sticky key source labels
- session binding counts
- provider busy state
- key busy counts
- last busy event
- whether last provider switch was `hard_failover` or `busy_overflow`

Suggested status additions:

- provider snapshot:
  - `busy_until`
  - `busy_backoff`
  - `busy_probe_inflight`
- client snapshot:
  - `sticky_binding_count`
  - `response_lookup_count`

Suggested log lines:

- `sticky bind scope=openai_responses key_source=previous_response_id provider=p1`
- `provider busy provider=p1 retry_in=5s reason=concurrency_limit`
- `session overflow key_source=previous_response_id from=p1 to=p2 after_retry=1`

## Configuration

Add a small, explicit config block rather than hard-coding behavior:

```yaml
routing:
  sticky_sessions:
    enabled: true
    strong_ttl: 30m
    weak_ttl: 10m
    response_lookup_ttl: 15m
  busy_backpressure:
    enabled: true
    retry_delays:
      - 5s
      - 10s
    probe_max_inflight: 1
    short_retry_after_max: 10s
```

Defaults should preserve current behavior when feature flags are disabled.

## Implementation Outline

### Config

- add routing config structs and defaults in [internal/config/config.go](/Users/sean/Programs/Clipal/internal/config/config.go)
- validate durations and array shape

### Protocol Extraction

- add sticky extraction helpers in [internal/proxy/protocols.go](/Users/sean/Programs/Clipal/internal/proxy/protocols.go) or a new focused file
- parse request body only once and expose a lightweight inspection path for JSON APIs

### Runtime State

- extend `ClientProxy` in [internal/proxy/proxy.go](/Users/sean/Programs/Clipal/internal/proxy/proxy.go) with:
  - sticky bindings
  - response lookup cache
  - busy state

### Classification

- extend [internal/proxy/failover_classify.go](/Users/sean/Programs/Clipal/internal/proxy/failover_classify.go) with `failureBusyRetry`
- split short concurrency busy from quota/rate-limit cooldown

### Forwarding Logic

- refactor [internal/proxy/failover_forward.go](/Users/sean/Programs/Clipal/internal/proxy/failover_forward.go) to:
  - resolve sticky provider before iteration
  - consult busy state before sending
  - perform bounded wait and probe
  - overflow and rebind on repeated busy

### Status and Presentation

- extend [internal/proxy/status.go](/Users/sean/Programs/Clipal/internal/proxy/status.go)
- extend [internal/proxy/presentation.go](/Users/sean/Programs/Clipal/internal/proxy/presentation.go)
- update Web API/view types if surfaced in UI

## Testing Strategy

Add regression coverage for:

- strong sticky key extraction per capability
- no sticky key for stateless requests
- response lookup assists `previous_response_id` chaining
- busy `429` does not deactivate provider
- busy `429` does not increment circuit failure
- request waits for `5s` busy window through fake clock or injected timing hooks
- retry succeeds on same provider and keeps binding
- retry fails again and overflows
- overflow rebinds later requests in same session
- busy provider does not get a stampede of immediate retries
- multi-key provider uses next key before cross-provider overflow
- all candidates busy returns nearest retry-after semantics
- hot reload preserves or safely resets sticky/busy state according to config compatibility

## Risks

### 1. JSON Body Inspection Cost

Sticky extraction for request-side identifiers may require parsing buffered JSON bodies. Keep extractors narrow and avoid generic deep parsing.

### 2. Weak-Key Misbinding

Treat weak hints conservatively. Only strong keys get durable rebinding.

### 3. Busy Classification Ambiguity

Some providers blur rate limit and concurrency limit. Default to current cooldown behavior unless the signal is clearly short-lived busy.

### 4. Wait Amplification

Bound queueing to request context and use provider probe gating to avoid synchronized wakeups.

## Rollout Plan

Phase 1:

- add config and runtime structures
- add sticky extraction
- add busy state and classification
- implement request-local wait and overflow
- keep Web UI additions minimal

Phase 2:

- improve provider-specific busy heuristics
- expose richer Web observability
- tune defaults with real traffic feedback

## References

- OpenAI Responses API: https://platform.openai.com/docs/api-reference/responses/retrieve
- OpenAI tools/computer use guide: https://platform.openai.com/docs/guides/tools-computer-use
- Anthropic API getting started: https://docs.anthropic.com/en/api/getting-started
- Anthropic Messages examples: https://docs.anthropic.com/en/api/messages-examples
- Anthropic code execution tool and container reuse: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/code-execution-tool
- Anthropic prompt caching: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
- Gemini API docs: https://ai.google.dev/api
- Gemini context caching: https://ai.google.dev/gemini-api/docs/caching/
