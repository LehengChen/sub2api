# Patch Queue

This file records runtime differences carried by the Frenzy candidate relative
to the upstream release. Upstreamed fixes are recorded so they are not replayed
as local patches during the next sync.

## v0.1.172 baseline

- Upstream tag: `v0.1.172`
- Upstream peeled commit: `155c494964c3ea6ecc31f52679525c1034bf0f16`
- Upstream capacity recovery: `8f7b0a314de816daabb5b761db3025cb10c3eca9` (PR #5398)
- FZ-010 (`9ecd2604dc8cebccdd28cf7014f396320d4037a6`) is `drop-upstreamed`.
  The upstream capacity recovery includes the pre-output OAuth capacity
  switch, staged structural-event EOF behavior, and the bare `event:error`
  compatibility cases. It is not replayed locally.

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
