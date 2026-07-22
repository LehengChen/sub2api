#!/usr/bin/env python3

from __future__ import annotations

import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from typing import Any


SCRIPT = Path(__file__).with_name("verify_oci_platform.py")
OCI_INDEX = "application/vnd.oci.image.index.v1+json"
OCI_MANIFEST = "application/vnd.oci.image.manifest.v1+json"
OCI_CONFIG = "application/vnd.oci.image.config.v1+json"


class LayoutBuilder:
    def __init__(self) -> None:
        self.blobs: dict[str, bytes] = {}

    def add_json(
        self,
        document: dict[str, Any],
        media_type: str,
        *,
        platform: dict[str, str] | None = None,
        annotations: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        data = json.dumps(document, sort_keys=True, separators=(",", ":")).encode()
        digest = f"sha256:{hashlib.sha256(data).hexdigest()}"
        self.blobs[digest] = data
        descriptor: dict[str, Any] = {
            "mediaType": media_type,
            "digest": digest,
            "size": len(data),
        }
        if platform is not None:
            descriptor["platform"] = platform
        if annotations is not None:
            descriptor["annotations"] = annotations
        return descriptor

    def image_manifest(
        self,
        os_name: str,
        architecture: str,
        *,
        descriptor_platform: dict[str, str] | None = None,
        annotations: dict[str, str] | None = None,
        created: str | None = None,
    ) -> dict[str, Any]:
        config_document: dict[str, Any] = {
            "architecture": architecture,
            "os": os_name,
            "rootfs": {"type": "layers", "diff_ids": []},
        }
        if created is not None:
            config_document["created"] = created
        config = self.add_json(
            config_document,
            OCI_CONFIG,
        )
        manifest = {
            "schemaVersion": 2,
            "mediaType": OCI_MANIFEST,
            "config": config,
            "layers": [],
        }
        return self.add_json(
            manifest,
            OCI_MANIFEST,
            platform=descriptor_platform,
            annotations=annotations,
        )

    def index(
        self,
        manifests: list[dict[str, Any]],
        *,
        annotations: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self.add_json(
            {"schemaVersion": 2, "mediaType": OCI_INDEX, "manifests": manifests},
            OCI_INDEX,
            annotations=annotations,
        )

    def write_archive(
        self, root_manifests: list[dict[str, Any]], archive_path: Path
    ) -> None:
        root_index = json.dumps(
            {"schemaVersion": 2, "mediaType": OCI_INDEX, "manifests": root_manifests},
            sort_keys=True,
            separators=(",", ":"),
        ).encode()
        layout = b'{"imageLayoutVersion":"1.0.0"}'
        with tarfile.open(archive_path, mode="w") as archive:
            self._write_member(archive, "index.json", root_index)
            self._write_member(archive, "oci-layout", layout)
            for digest, data in self.blobs.items():
                self._write_member(
                    archive, f"blobs/sha256/{digest.removeprefix('sha256:')}", data
                )

    @staticmethod
    def _write_member(archive: tarfile.TarFile, name: str, data: bytes) -> None:
        member = tarfile.TarInfo(name=name)
        member.size = len(data)
        archive.addfile(member, io.BytesIO(data))


class VerifyOCIPlatformTest(unittest.TestCase):
    def run_verifier(self, builder: LayoutBuilder, manifests: list[dict[str, Any]]):
        with tempfile.TemporaryDirectory() as temp_dir:
            archive_path = Path(temp_dir) / "candidate.oci.tar"
            builder.write_archive(manifests, archive_path)
            return subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    str(archive_path),
                    "--platform",
                    "linux/amd64",
                ],
                text=True,
                capture_output=True,
                check=False,
            )

    def test_selects_runnable_manifest_through_nested_attestation_indexes(self) -> None:
        builder = LayoutBuilder()
        runnable = builder.image_manifest(
            "linux", "amd64", descriptor_platform={"os": "linux", "architecture": "amd64"}
        )
        provenance = builder.image_manifest(
            "unknown",
            "unknown",
            descriptor_platform={"os": "unknown", "architecture": "unknown"},
            annotations={"vnd.docker.reference.type": "attestation-manifest"},
        )
        sbom = builder.image_manifest(
            "linux",
            "amd64",
            annotations={"vnd.docker.reference.type": "attestation-manifest"},
        )
        nested_index = builder.index([runnable, provenance])
        attestation_index = builder.index(
            [sbom],
            annotations={"vnd.docker.reference.type": "attestation-manifest"},
        )

        result = self.run_verifier(builder, [nested_index, attestation_index])

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), runnable["digest"])

    def test_rejects_multiple_runnable_manifests_for_target_platform(self) -> None:
        builder = LayoutBuilder()
        first = builder.image_manifest("linux", "amd64")
        second = builder.image_manifest(
            "linux",
            "amd64",
            descriptor_platform={"os": "linux", "architecture": "amd64"},
            created="2026-07-22T00:00:00Z",
        )

        result = self.run_verifier(builder, [first, second])

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("found 2", result.stderr)

    def test_rejects_archive_without_target_platform(self) -> None:
        builder = LayoutBuilder()
        arm_image = builder.image_manifest("linux", "arm64")

        result = self.run_verifier(builder, [arm_image])

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("found 0", result.stderr)

    def test_rejects_descriptor_and_config_platform_disagreement(self) -> None:
        builder = LayoutBuilder()
        image = builder.image_manifest(
            "linux",
            "amd64",
            descriptor_platform={"os": "linux", "architecture": "arm64"},
        )

        result = self.run_verifier(builder, [image])

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("architecture disagrees", result.stderr)


if __name__ == "__main__":
    unittest.main()
