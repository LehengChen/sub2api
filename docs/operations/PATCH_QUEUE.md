# Patch Queue

This file records runtime differences carried by the Frenzy candidate relative
to the upstream release. Upstreamed fixes are recorded so they are not replayed
as local patches during the next sync.

## v0.1.172 baseline

- Upstream tag: `v0.1.172`
- Upstream peeled commit: `155c494964c3ea6ecc31f52679525c1034bf0f16`
- Upstream capacity recovery: `8f7b0a314de816daabb5b761db3025cb10c3eca9` (PR #5398)
- FZ-010 is carried as a local reimplementation because the upstream capacity
  recovery is broader than the frozen contract: it stages every retryable
  `event:error`, retries capacity on the same account, and rewrites capacity
  codes for every platform/account type. Those changes would turn legacy
  API-key/Grok/non-capacity and structural-EOF paths into new failover paths.

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
last_reviewed_against: 155c494964c3ea6ecc31f52679525c1034bf0f16
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
  `response.output_item.added` EOF, post-output capacity, and the WS bridge
  retain the pre-v172 raw/missing-terminal behavior. No config, migration,
  scheduler, or infra change is part of FZ-010.
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
last_reviewed_against: 155c494964c3ea6ecc31f52679525c1034bf0f16
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
