#!/usr/bin/env python3
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("check_pnpm_audit_exceptions.py")


def exception_yaml(*, expires_on: str = "2999-01-01", owner: str = "@owner") -> str:
    return f'''version: 1
exceptions:
  - package: xlsx
    advisory: "GHSA-example"
    severity: high
    reason: "Documented test exception"
    mitigation: "Test mitigation"
    expires_on: "{expires_on}"
    owner: "{owner}"
'''


class AuditExceptionValidatorTest(unittest.TestCase):
    def run_validator(self, audit: dict, exceptions: str) -> subprocess.CompletedProcess:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            audit_path = root / "audit.json"
            exception_path = root / "exceptions.yml"
            audit_path.write_text(json.dumps(audit), encoding="utf-8")
            exception_path.write_text(exceptions, encoding="utf-8")
            return subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--audit",
                    str(audit_path),
                    "--exceptions",
                    str(exception_path),
                ],
                check=False,
                capture_output=True,
                text=True,
            )

    @staticmethod
    def audit_with_high_finding() -> dict:
        return {
            "vulnerabilities": {
                "xlsx": {
                    "severity": "high",
                    "via": [
                        {
                            "github_advisory_id": "GHSA-example",
                            "title": "Example finding",
                        }
                    ],
                }
            }
        }

    def test_valid_matching_exception_passes(self):
        result = self.run_validator(
            self.audit_with_high_finding(), exception_yaml()
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_unused_expired_exception_fails_globally(self):
        result = self.run_validator({}, exception_yaml(expires_on="2000-01-01"))
        self.assertEqual(result.returncode, 1)
        self.assertIn("Exception expired", result.stderr)

    def test_placeholder_owner_fails(self):
        result = self.run_validator(
            {}, exception_yaml(owner="security@your-domain")
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("placeholder owner", result.stderr)

    def test_unexcepted_high_finding_fails(self):
        result = self.run_validator(self.audit_with_high_finding(), "version: 1\nexceptions:\n")
        self.assertEqual(result.returncode, 1)
        self.assertIn("missing exceptions", result.stderr)


if __name__ == "__main__":
    unittest.main()
