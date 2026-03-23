# Unified Routing Reset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `/clipal` as unified ingress while preserving separated Claude, Codex, and Gemini backend provider pools, configs, and UI grouping.

**Architecture:** Keep the existing three backend config domains as routing truth and user-facing management units. Add a unified request-context layer for `/clipal`, map that context into the correct backend pool, and keep compatibility aliases broad and stable. Runtime state may still be capability-aware, but only within the correct backend pool.

**Tech Stack:** Go, `net/http`, YAML config loading, existing `internal/proxy`, `internal/config`, `internal/web`, Go tests

---

### Task 1: Restore separated config model as routing truth

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Add tests covering:

```go
func TestLoad_UsesSeparatedClientConfigsAsRoutingTruth(t *testing.T) {}
func TestLoad_DoesNotMergeProviderPoolsAcrossClients(t *testing.T) {}
func TestLoad_PreservesManualModeAndPinnedProvider(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run 'TestLoad_(UsesSeparatedClientConfigsAsRoutingTruth|DoesNotMergeProviderPoolsAcrossClients|PreservesManualModeAndPinnedProvider)' -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

Restore `ClientConfig`-based loading in `internal/config/config.go` and make:

- `claude-code.yaml`
- `codex.yaml`
- `gemini.yaml`

the persisted routing truth again.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run 'TestLoad_(UsesSeparatedClientConfigsAsRoutingTruth|DoesNotMergeProviderPoolsAcrossClients|PreservesManualModeAndPinnedProvider)' -v`
Expected: PASS

- [ ] **Step 5: Run package tests**

Run: `go test ./internal/config -v`
Expected: PASS

### Task 2: Implement unified ingress over separated backend pools

**Files:**
- Modify: `internal/proxy/proxy.go`
- Modify: `internal/proxy/protocols.go`
- Modify: `internal/proxy/failover_forward.go`
- Modify: `internal/proxy/failover_protocol.go`
- Modify: `internal/proxy/status.go`
- Test: `internal/proxy/proxy_test.go`
- Test: `internal/proxy/protocols_test.go`
- Test: `internal/proxy/reload_state_test.go`

- [ ] **Step 1: Write the failing tests**

Add tests covering:

```go
func TestClipalClaudeRequestUsesClaudePool(t *testing.T) {}
func TestClipalResponsesRequestUsesCodexPool(t *testing.T) {}
func TestClipalGeminiRequestUsesGeminiPool(t *testing.T) {}
func TestCompatibilityAliasUnknownSubpathStillForwards(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/proxy -run 'Test(ClipalClaudeRequestUsesClaudePool|ClipalResponsesRequestUsesCodexPool|ClipalGeminiRequestUsesGeminiPool|CompatibilityAliasUnknownSubpathStillForwards)' -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

Implement:

- strict protocol detection for `/clipal`
- backend-pool mapping from protocol to existing client groups
- broad compatibility alias handling for `/claudecode`, `/codex`, `/gemini`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/proxy -run 'Test(ClipalClaudeRequestUsesClaudePool|ClipalResponsesRequestUsesCodexPool|ClipalGeminiRequestUsesGeminiPool|CompatibilityAliasUnknownSubpathStillForwards)' -v`
Expected: PASS

- [ ] **Step 5: Run package tests**

Run: `go test ./internal/proxy -v`
Expected: PASS

### Task 3: Restore separated web persistence and grouped UI semantics

**Files:**
- Modify: `internal/web/api.go`
- Modify: `internal/web/config_store.go`
- Modify: `internal/web/types.go`
- Modify: `internal/web/handler.go`
- Modify: `internal/web/static/app.js`
- Modify: `internal/web/static/index.html`
- Modify: `internal/web/static/styles.css`
- Test: `internal/web/api_test.go`
- Test: `internal/web/config_store_test.go`
- Test: `internal/web/handler_test.go`

- [ ] **Step 1: Write the failing tests**

Add tests covering:

```go
func TestHandleAddProvider_WritesClientSpecificConfig(t *testing.T) {}
func TestHandleGetProviders_RequiresClientGroup(t *testing.T) {}
func TestHandleUpdateProvider_OnlyTouchesRequestedClientGroup(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web -run 'TestHandle(AddProvider_WritesClientSpecificConfig|GetProviders_RequiresClientGroup|UpdateProvider_OnlyTouchesRequestedClientGroup)' -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

Restore:

- grouped CRUD under `/api/providers/claude-code`
- grouped CRUD under `/api/providers/codex`
- grouped CRUD under `/api/providers/gemini`

and remove unified provider-list semantics from the UI.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/web -run 'TestHandle(AddProvider_WritesClientSpecificConfig|GetProviders_RequiresClientGroup|UpdateProvider_OnlyTouchesRequestedClientGroup)' -v`
Expected: PASS

- [ ] **Step 5: Run package tests**

Run: `go test ./internal/web -v`
Expected: PASS

### Task 4: Align CLI status and docs-facing runtime outputs with grouped backend pools

**Files:**
- Modify: `cmd/clipal/status.go`
- Test: `cmd/clipal/status_test.go`

- [ ] **Step 1: Write the failing tests**

Add tests covering:

```go
func TestStatus_ReportsGroupedBackendPools(t *testing.T) {}
func TestStatus_RuntimeProjectionDoesNotAssumeUnifiedProviders(t *testing.T) {}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/clipal -run 'TestStatus_(ReportsGroupedBackendPools|RuntimeProjectionDoesNotAssumeUnifiedProviders)' -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

Make status output reflect grouped backend pools only.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/clipal -run 'TestStatus_(ReportsGroupedBackendPools|RuntimeProjectionDoesNotAssumeUnifiedProviders)' -v`
Expected: PASS

- [ ] **Step 5: Run package tests**

Run: `go test ./cmd/clipal -v`
Expected: PASS

### Task 5: Full verification

**Files:**
- Verify only

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 2: Run smoke script**

Run: `./scripts/smoke_test.sh`
Expected: PASS

- [ ] **Step 3: Manual verification**

Verify:

- `/clipal` handles Claude / OpenAI / Gemini paths
- compatibility aliases still work
- Web UI shows separated groups only

