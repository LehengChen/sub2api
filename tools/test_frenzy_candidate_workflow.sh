#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
WORKFLOW="$ROOT_DIR/.github/workflows/frenzy-candidate.yml"

fail() {
  printf 'frenzy candidate workflow test failed: %s\n' "$1" >&2
  exit 1
}

test -f "$WORKFLOW" || fail "workflow is missing"

python3 - "$WORKFLOW" <<'PY'
from pathlib import Path
import re
import sys

workflow = Path(sys.argv[1]).read_text(encoding="utf-8")

def require(pattern: str, description: str) -> None:
    if not re.search(pattern, workflow, re.MULTILINE):
        raise SystemExit(f"missing {description}: {pattern}")

def forbid(pattern: str, description: str) -> None:
    if re.search(pattern, workflow, re.MULTILINE | re.IGNORECASE):
        raise SystemExit(f"forbidden {description}: {pattern}")

# The trigger set is deliberately narrow. Pull-request head filtering is
# enforced by the gate job because GitHub has no native head-ref filter.
require(r"(?m)^on:\s*$", "workflow trigger root")
require(r"(?m)^\s+workflow_dispatch:\s*$", "workflow_dispatch trigger")
require(r"(?ms)^\s+workflow_dispatch:\s*\n\s+inputs:\s*\n\s+version:\s*\n\s+description:.*\n\s+required:\s+true", "required manual version input")
require(r"(?m)^\s+pull_request:\s*$", "pull_request trigger")
require(r"(?m)^\s+push:\s*$", "push trigger")
require(r"frenzy/candidate/\*\*", "candidate tag filter")
require(r"(?ms)^\s+push:\s*\n\s+tags:\s*\n\s+- [\"']frenzy/candidate/\*\*[\"']", "tag-only push filter")
require(r"startsWith\(github\.event\.pull_request\.head\.ref, 'integration/'\)", "integration PR guard")
require(r"startsWith\(github\.event\.pull_request\.head\.ref, 'release/'\)", "release PR guard")

# Read-only GitHub authority is a hard contract for this build-only workflow.
require(r"(?ms)^permissions:\s*\n\s+contents:\s+read\s*$", "contents: read-only permissions")
forbid(r"(?m)^\s+contents:\s+(?!read\s*$)", "writable contents permission")
forbid(r"(?m)^\s+id-token:\s+", "OIDC signing permission")
forbid(r"(?m)^\s+packages:\s+(?!none\s*$)", "package registry permission")

# No source/tag/image push or production control plane may be hidden in a
# candidate build. The build-push action itself is allowed with push: false.
forbid(r"\bgit\s+push\b", "git push")
forbid(r"\bdocker\s+push\b", "docker push")
forbid(r"docker/login-action", "registry login")
forbid(r"(?m)^\s+push:\s*true\s*$", "image push=true")
forbid(r"(?m)^\s+terraform\b|\bterraform\s+-", "Terraform control plane")
forbid(r"\baws\s+(?:[a-z]|--)", "AWS control plane")
forbid(r"\bkubectl\b|\bhelm\b|\bssh\s", "deployment control plane")
forbid(r"(?::|@)latest\b|@latest\b", "latest image/tool reference")

# Every evidence-producing gate must consume the immutable gate output.
for job in ("backend", "lint", "frontend", "security", "image"):
    require(rf"(?ms)^  {job}:\s*\n\s+needs:\s+gate\s*\n\s+if:\s+\$\{{\{{\s*needs\.gate\.result == 'success'", f"{job} gate dependency")

for field in ("VERSION", "COMMIT", "DATE", "full_commit", "utc_date"):
    require(re.escape(field), f"explicit {field} identity")

for command, description in (
    (r"go test -tags=unit", "backend unit tests"),
    (r"go test -tags=integration", "integration harness"),
    (r"go test -race -tags=unit", "targeted race gate"),
    (r"go generate ./ent", "Ent generation"),
    (r"go generate ./cmd/server", "Wire generation"),
    (r"pnpm run lint:check", "frontend lint"),
    (r"pnpm run typecheck", "frontend typecheck"),
    (r"pnpm run test:run", "frontend Vitest"),
    (r"pnpm run build", "frontend build"),
    (r"govulncheck", "govulncheck"),
    (r"pnpm audit --prod --audit-level=high --json", "pnpm audit"),
    (r"check_pnpm_audit_exceptions\.py", "audit exception gate"),
    (r"PROMPT_AUDIT_TEST_POSTGRES_DSN", "Prompt Audit PostgreSQL integration dependency"),
    (r"PROMPT_AUDIT_TEST_REDIS_ADDR", "Prompt Audit Redis integration dependency"),
    (r"Require shared-state integration coverage", "required integration skip rejection"),
):
    require(command, description)

for pattern, description in (
    (r"platforms:\s+linux/amd64", "linux/amd64 build"),
    (r"outputs:\s+type=oci,dest=", "OCI output"),
    (r"provenance:\s+mode=max", "Buildx provenance"),
    (r"sbom:\s+true", "Buildx SBOM"),
    (r"aquasecurity/trivy-action@v0\.36\.0", "pinned Trivy image scan"),
    (r"input:\s+\$\{\{ github\.workspace \}\}/evidence/image/scan-image\.tar", "Trivy local image input"),
    (r"exit-code:\s+1", "blocking image vulnerability scan"),
    (r"registry_image_digest:\s+\{status: \"missing\"", "missing registry digest report"),
    (r"signature:\s+\{status: \"missing\"", "missing signature report"),
    (r"actions/upload-artifact@", "non-production artifact upload"),
):
    require(pattern, description)

print("frenzy candidate workflow policy checks passed")
PY
