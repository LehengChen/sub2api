# Patch Queue

This file records runtime differences carried by the Frenzy candidate relative
to the upstream release. Upstreamed fixes are recorded so they are not replayed
as local patches during the next sync.

## v0.2.1 baseline

- Upstream tag: `v0.2.1`
- Upstream peeled commit: `578785ee7fb35030b094b69624efe25670a36f5f`
- Baseline status: v0.2.1 is merged with the reviewed Frenzy patch line in the
  isolated upgrade tree. No AWS, production, or traffic action is implied by
  this source merge; build, image, migration, and standby gates remain separate.
- The upstream tag still declares `0.2.0` in `backend/cmd/server/VERSION`;
  this candidate normalizes it to `0.2.1` so the binary, UI, and release
  evidence use one version identity.
- v0.2.1 upstream adds the GPT-6 Astra model manifest updates, per-group Codex
  model manifests, upstream request IDs in usage logs, and migrations 232-234.
  None of those changes classifies OAuth `invalid_prompt` as request-scoped or
  proves the OAuth/account-scoped capacity contract below, so FZ-010/011/013
  and FZ-015 remain explicit local patches.

### Prior v0.1.177 baseline (historical)

- Upstream tag: `v0.1.177`
- Upstream peeled commit: `073e92d17178a1ccdb0a27017f572f10c9c7ab62`
- Upstream capacity recovery: `8f7b0a314de816daabb5b761db3025cb10c3eca9` (PR #5398)
- This section records the previous synchronization point only. Do not use its
  production-status sentence as the status of the v0.2.1 candidate.
- `backend/cmd/server/VERSION` is normalized from the tag's stale `0.1.176` to
  `0.1.177`; release builds should still inject the immutable candidate version.
- FZ-010 was carried as a local reimplementation because the upstream capacity
  recovery remained broader than the frozen contract: v0.1.177 staged generic
  retryable errors and permits same-account backoff, while production requires
  OpenAI OAuth capacity to exclude the selected account and preserve legacy
  API-key/Grok/non-capacity and structural-EOF behavior.

### v0.1.177 upstream delta

- Codex remote compaction v2, account-bound `x-codex-turn-state`, and the
  opt-in fingerprint default are included. The cross-account turn-state guard
  complements but does not replace the local capacity account exclusion.
- Group usage daily rollups and migrations 222/223 are included. The migrations
  are additive but must complete the independent migration rehearsal and
  production migration axis before application activation.
- The Go module directive is 1.26.6. Frenzy additionally pins the corresponding
  builder image digest and raises `golang.org/x/mod` to 0.40.0 to close the
  current production Inspector findings; final image scanning remains required.

## FZ-001: HTTPS-only exit probes

```yaml
id: FZ-001
order: 10
status: cherry-pick
original_commit: 22fbd18ef90607facfa2782c7b40982a820549a9
applied_commits:
  - 0b5f9e16cf4617442e0f0543598ecaec4c8a20f4
stable_patch_ids:
  - b676c2495ff399d2ae5b1fe2b4fdf0fa01a61204
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Exit and quality probes remain HTTPS-only. Upstream v0.2.1 still uses the
  HTTP targets without this patch; the code and focused tests applied cleanly.

## FZ-002: isolated offline pricing

```yaml
id: FZ-002
order: 20
status: cherry-pick
original_commit: 081d44d7f8647771df6ca91ffdf860131c39f81d
applied_commits:
  - bbbf371d856038ace4d1651f87d141c8df557406
stable_patch_ids:
  - c28d9bb835a86e352b1d04277ec7a0dc925fe2a0
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- `pricing.remote_updates_enabled=false` uses packaged/fallback pricing and
  does not start the remote update scheduler. The default remains enabled for
  deployments that rely on upstream behavior.

## FZ-003: dependency and migration safety baseline

```yaml
id: FZ-003
order: 30
status: recalculate
applied_commits:
  - 2dba5b5c2d9198e464bb719a0d3ab1a319bf352a
  - 33f9f2dbbb6c1bb77a3d16b855e04a6810a18aec
  - b8cf9665e1c4e4a2bc13c902e2d87cf3fb67ca33
  - 0207b71a7fc401367af8b15cbb4d99e35eef502f
  - 2df25467ecbfca8fadfbcbe5eebb03a1e77e1a33
  - 6a58ab3784ff86831955afd17919f61434c9e5d6
stable_patch_ids:
  - 1cf1c2a64c9339b791a9966746eceac7dad31c3d
  - 24b9e98f08c18e00aad63642c26325f379c8c148
  - 804bf6f6e54af1b35c04819f72e03175b81ed7ed
  - eb5f06a591046d521553b6015d1cc90caa1900e0
  - ff5201438b2be96eb4eb4e7d04c8fb61c386c0d6
  - 0594c168fb5533516922aaff800f318cc64d0fef
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Recalculated on the v0.2.1 module graph: `golang.org/x/image` is 0.45.0,
  OpenTelemetry core/metric/sdk/trace are 1.44.0, and the frontend uses
  `@e965/xlsx` 0.20.3 instead of `xlsx` 0.18.5. The obsolete explicit Wire
  tool requirement is dropped; upstream's readonly generator checksums remain.
- Non-transactional migrations 175a and 190 drop an invalid concurrent index
  before retrying, while the v0.2.1 migrations 232-234 remain preserved.
- Dependency audit, unit tests, and image scanning must be rerun for the final
  candidate; old v0.1.172 scan results are not evidence for this release.

## FZ-004: externally managed release control

```yaml
id: FZ-004
order: 40
status: reimplement
applied_commits:
  - 77e2adfe0f0dae05ae3b51a13e2a70c0e101c041
  - 1ee7275e7a0c467dc18ca756041fa88e438898a2
stable_patch_ids:
  - cea97db42140175ed659393db8b4c5b2f2d45dd9
  - 01a55392cd874a4e6f1a4688430ace1bd434ddbc
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Externally managed deployments expose read-only version health but disable
  in-application update, rollback, and restart controls. The v0.2.1 Wire
  graph was regenerated rather than copying the old generated file.

## FZ-005: readiness and bounded drain

```yaml
id: FZ-005
order: 50
status: reimplement
applied_commits:
  - 8e6591972560821f7519cd44148813b8a68ded5d
  - 205c9609985be734783e1a7cb9e67e34faf4b53d
  - b6709d8f37ed47ba73990222b2c59d08cd5dd441
  - 02d7748082fc4b6971c8f56701af2c4607b929bf
stable_patch_ids:
  - 793431dd4daf7d426f9a000d5e28a38126f91ba4
  - 90941563989f468d721fd5d4783058bcf205ab6c
  - ca3f91b3856874b2b896d669ed4fcc1f52c1162a
  - c3ee1aaa8a08bb0f58d591189974beef122efee0
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- `/livez` and dependency-backed `/readyz`, migration readiness, request and
  WebSocket drain registration, forced close grace, and ordered usage/billing/
  quota cleanup are carried. Deployment probes target lifecycle endpoints.

## FZ-006: shared OAuth sessions

```yaml
id: FZ-006
order: 60
status: cherry-pick
original_commit: 6b0bf99da9380cca4f3c45d9a36b29c425c72149
applied_commits:
  - 61fb096564ec55a82d582e8920f9eb927e8a5506
stable_patch_ids:
  - a0bedb66b091548f3d24a1573e0aff53997c2e1e
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Claude, OpenAI, Grok, Gemini, and Antigravity OAuth state is injected through
  the fail-closed Redis session store. The v0.2.1 Grok password/SSO and token
  validation behavior is retained while its session lifecycle uses the shared
  store; memory stores remain limited to direct/test constructors.

## FZ-007: fenced multi-center runtime roles

```yaml
id: FZ-007
order: 70
status: reimplement
applied_commits:
  - 0b0f923768878b4ee09c3c8705afcaf5b3dd9837
  - b6695543029b59f302234ba7b122581c9c60c75a
  - 70b1f4bfc5073f48076c3d9c4d318f0fcc2b3e9f
stable_patch_ids:
  - d029b0965dce360745b8a0b3f31fa495b654daf7
  - 77c6a2470cc7f1c4e56e3ccb581f8189dae7ff0b
  - 409888b8a2f81ad5406d6d9e468dd5b52a3d1f45
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Explicit active/api/worker/standby/migrator roles, Redis worker lease and
  readiness fencing, migration-only startup, and inert standby providers are
  carried. The v0.2.1 Channel Monitor runner and V2 aggregator are also
  fenced, and shutdown cleans workers before releasing the runtime lease.
- This preserves manual cold standby. It does not approve active-active or
  automatic failover; critical write fencing still requires separate proof.

## FZ-008: redirect-hop validation

```yaml
id: FZ-008
order: 80
status: cherry-pick
original_commit: f37abee8b1ccb2fcd5746690b7fbd68df3ed0ee4
applied_commits:
  - 453deff3a63d666863aa31d72972d3632ef94cee
stable_patch_ids:
  - 14c320693cd8eedd3267f3eca38731567e1b1eac
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Every redirect hop revalidates scheme, host allowlist, userinfo, port, and
  private DNS resolution. v0.2.1 transport behavior is preserved.

## FZ-009: pinned container build inputs

```yaml
id: FZ-009
order: 90
status: recalculate
applied_commits:
  - 700ab158229365e512849c1772f2cda7b1a2cbbd
stable_patch_ids:
  - 36a5dc267f33b12d6a25afb216313515a0e4f998
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Dockerfile frontend, Node 24, Go 1.27.0, Alpine 3.21, PostgreSQL 18, and
  pnpm 9.15.9 inputs are pinned. The Go 1.27.0 builder digest is fixed in the
  candidate Dockerfile; the final multi-arch manifest still requires a release
  build-time resolution check.
- Manifest resolution is not an image vulnerability scan or provenance
  approval. The final application image must still be built once, identified
  by digest, scanned, and rehearsed on the standby center before promotion.

## FZ-010: OpenAI OAuth streaming capacity failover

```yaml
id: FZ-010
order: 100
status: reimplement
original_commit: 9ecd2604dc8cebccdd28cf7014f396320d4037a6
applied_commits:
  - dfa39ebc341209e0a49e0a83717a71e0ea534a24
upstream_commit_reviewed: c33c3208e307c53c82daebc0ba303c3f09b51308
upstream_test_calibration_reviewed: 14a27f196
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
upstream_issue_or_pr: https://github.com/Wei-Shaw/sub2api/pull/5398
```

- Intent: recognize exact OpenAI OAuth capacity signals before semantic
  output, return `UpstreamFailoverError`, and let the existing handler exclude
  the account and select another account immediately.
- Scope: native Responses and streaming passthrough only; codes are exactly
  `server_is_overloaded` and `slow_down`. The SSE `event:` header is used when
  the JSON data has no `type` field.
- Scheduling invariant: OpenAI OAuth capacity sets
  `RetryableOnSameAccount=false`; API-key, Grok, and non-capacity errors retain
  their previous retry/flush behavior.
- Compatibility invariant: `response.created`/`response.in_progress`/
  `response.output_item.added` EOF, API-key/Grok/non-capacity errors, and the WS
  bridge retain their prior behavior. Post-semantic capacity is rewritten to a
  generic retryable error without replay. No config, migration, or infra change
  is part of FZ-010.
- Evidence boundary: service tests cover native and passthrough bare-header and
  typed capacity, account/platform negatives, structured failover logging,
  post-output no-replay, structural EOF, and WS raw relay. They do not claim a
  live upstream A-to-B replay.
- Drop condition: only remove this local patch after an upstream release proves
  the same OAuth/account guards and all listed compatibility tests.

## FZ-011: OpenAI OAuth non-streaming `response.failed`

```yaml
id: FZ-011
order: 110
status: reimplement
original_commit: 5e8acfebec71ac9ae6558787b3d4f563d1913fb2
applied_commits:
  - be42eaff992735407cdf486ec9e8725ef5944258
  - 7c9e545e9e62e5490228226cf933efe811c9209c
upstream_stable_patch_id: 382e95eb4d1abefe9a23814df687523a4e568c3f
scope_fix_stable_patch_id: 00b4dc259198fa7aedeacfc5c91b2a5e5627541a
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
upstream_issue_or_pr: https://github.com/Wei-Shaw/sub2api/pull/5326
```

- Intent: route HTTP 200 SSE `response.failed` capacity/transient events in
  `stream=false` native and passthrough conversion through the existing
  failover path before semantic output is committed.
- Scope: only `PlatformOpenAI + OAuth` and a positive 429/transient
  classification. OpenAI API-key, Grok, unknown failures, committed output,
  and structural-event EOF retain upstream behavior.
- Dependencies: existing transient/status classification,
  `UpstreamFailoverError`, account exclusion, and EWMA scheduling. No new
  runtime dependency or migration is introduced by FZ-011.
- Evidence boundary: service-level native/passthrough conversion and account
  guards are tested; this does not claim a real OAuth A-to-B upstream replay or
  complete coverage of every Issue #5281 envelope.
- Drop condition: when upstream provides the same OAuth non-streaming
  classification, failover, and compatibility tests, stop replaying both
  patch units and mark FZ-011 `drop-upstreamed`.

## FZ-012: restore lifecycle probe bypass for the embedded frontend

```yaml
id: FZ-012
order: 115
status: reimplement
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- The v0.2.1 tree carries the FZ-005 lifecycle handlers, but its embedded
  frontend middleware does not bypass `/livez` or `/readyz`; those paths then
  receive the SPA document instead of the JSON health routes.
- This two-line compatibility fix is required for the existing readiness and
  ALB contracts. It has no effect on API routing or capacity classification.

## FZ-013: OpenAI OAuth overload-message capacity failover

```yaml
id: FZ-013
order: 120
status: reimplement
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
upstream_issue_or_pr: https://github.com/Wei-Shaw/sub2api/issues/5281
```

- Intent: recognize the exact observed upstream message `Our servers are
  currently overloaded. Please try again later.` as an OpenAI OAuth capacity
  signal before semantic output, then exclude the current account and switch.
- Scope: HTTP Responses SSE in native, passthrough, and the existing
  `stream=false` SSE conversion path. Structured capacity codes and the known
  `Selected model is at capacity` message share the same OAuth-only decision.
- Compatibility invariant: API-key, Grok, non-capacity errors, ordinary HTTP
  JSON, WebSocket responses, post-output replay, and structural EOF behavior
  remain unchanged. The new production message uses a trimmed, case-insensitive
  full-sentence match rather than a generic `overloaded` keyword.
- Evidence boundary: decision logs distinguish pre-output failover from
  post-output passthrough without recording the raw upstream body. Unit, race,
  EOF, and full service tests do not claim a live upstream A-to-B replay.

## FZ-014: bounded batch statistics for the admin usage view

```yaml
id: FZ-014
order: 130
status: reimplement
original_commit: 6fbd4b4a85adf1e7df830a43f0417c3c25c082c7
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
```

- Intent: keep the admin account-usage page responsive when many OpenAI OAuth
  accounts are loaded at once. Quota probes are bounded per account and the
  two local rolling-window aggregates use one set-based query per window.
- Scope: `GetUsageBatch` only; the public `UsageLogRepository` interface,
  database schema, and API response shape are unchanged. Older repository
  implementations use a bounded compatibility fallback.
- Failure policy: quota snapshots are returned when local statistics time out
  or fail, with structured slow/error logs for diagnosis. This patch does not
  change gateway scheduling or account capacity behavior.

## FZ-015: OpenAI OAuth `invalid_prompt` is not an account failure

```yaml
id: FZ-015
order: 140
status: reimplement
applied_commits:
  - e13b393cce54994e5d1a7e9769ee32a1ad6ddabe
stable_patch_ids:
  - 5956afdd2d63f23c21587f70b187fbe83febfba5
last_reviewed_against: 578785ee7fb35030b094b69624efe25670a36f5f
upstream_issue_or_pr: https://github.com/Wei-Shaw/sub2api/issues/6712
```

- Intent: treat an OpenAI OAuth `invalid_prompt` response as a request-scoped
  client error, return it to the caller, and never use it to switch accounts.
  This prevents one rejected prompt from being replayed across the account pool.
- Detection is deliberately narrow: exact structured error type/code
  `invalid_prompt`, plus the exact observed usage-policy sentence prefix from
  Issue #6712. Generic text containing `invalid prompt` does not match.
- Scope: native and passthrough Responses streams, `stream=false` SSE-to-JSON,
  and the WebSocket-to-HTTP bridge. Existing direct HTTP 400 behavior remains
  unchanged. API-key, Grok, capacity, and ordinary transient handling retain
  their prior behavior.
- Diagnostics record account ID, path, event type, match source, upstream
  request ID, and `decision=return_client_error`; prompt and response bodies are
  not logged. No account cooldown, scheduler, schema, admin setting, migration,
  or infrastructure change is introduced by FZ-015.
