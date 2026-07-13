## Change contract

- Change class: <!-- docs | application | dependency | schema | runtime-config | upstream-integration -->
- User-visible outcome:
- Scope / out of scope:
- Rollback point:

## Compatibility

- Schema change: <!-- none | expand | backfill | contract -->
- N/N-1 coexistence tested: <!-- yes | no | not applicable -->
- Image-only rollback safe: <!-- yes | no | not applicable -->
- Config/API/cache/payload compatibility:

## Upstream / Frenzy patches

- Upstream base and target tag object/peeled commit: <!-- integration PRs only -->
- Patch decisions (`FZ-xxx`: carry/reimplement/drop/contribute/retire):
- Generic change proposed upstream: <!-- link or reason not applicable -->

## Evidence

- [ ] Relevant backend tests
- [ ] Frontend lint/typecheck/tests/build, when applicable
- [ ] Generated code and lockfiles are reproducible
- [ ] Security/dependency/container checks, when applicable
- [ ] Migration rehearsal and production-chain synthetic, when applicable
- Candidate SHA / workflow run:

## Operations

- Deployment authorized by this PR: **no**
- Required private ops change/release manifest:
- Stop conditions and observation window:

<!--
Application PR approval never authorizes AWS, ECR, SSM, Terraform, database, account,
or rollout writes. Follow docs/operations/ and the private infra runbooks.
-->
