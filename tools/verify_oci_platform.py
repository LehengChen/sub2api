#!/usr/bin/env python3
"""Select one runnable image manifest for a platform from an OCI archive."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
import tarfile
from pathlib import PurePosixPath
from typing import Any


OCI_INDEX_MEDIA_TYPES = {
    "application/vnd.oci.image.index.v1+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
}
OCI_MANIFEST_MEDIA_TYPES = {
    "application/vnd.oci.image.manifest.v1+json",
    "application/vnd.docker.distribution.manifest.v2+json",
}
IMAGE_CONFIG_MEDIA_TYPES = {
    "application/vnd.oci.image.config.v1+json",
    "application/vnd.docker.container.image.v1+json",
}
ATTESTATION_REFERENCE_TYPE = "attestation-manifest"
MAX_METADATA_BLOB_SIZE = 32 * 1024 * 1024
SHA256_DIGEST_RE = re.compile(r"^sha256:([0-9a-f]{64})$")


class VerificationError(RuntimeError):
    """The OCI archive does not satisfy the platform verification contract."""


def _as_object(value: Any, description: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise VerificationError(f"{description} must be a JSON object")
    return value


class OCIArchive:
    """Read and integrity-check OCI metadata without extracting the archive."""

    def __init__(self, archive_path: str) -> None:
        self._archive_path = archive_path
        self._archive: tarfile.TarFile | None = None
        self._members: dict[str, tarfile.TarInfo] = {}

    def __enter__(self) -> "OCIArchive":
        try:
            self._archive = tarfile.open(self._archive_path, mode="r:*")
        except (OSError, tarfile.TarError) as exc:
            raise VerificationError(f"cannot open OCI archive: {exc}") from exc

        for member in self._archive.getmembers():
            name = member.name
            while name.startswith("./"):
                name = name[2:]
            path = PurePosixPath(name)
            if path.is_absolute() or ".." in path.parts:
                raise VerificationError(f"unsafe OCI archive member: {member.name}")
            canonical_name = str(path)
            if canonical_name in self._members:
                raise VerificationError(f"duplicate OCI archive member: {canonical_name}")
            self._members[canonical_name] = member
        return self

    def __exit__(self, *_args: object) -> None:
        if self._archive is not None:
            self._archive.close()

    def _read_member(self, name: str, *, max_size: int) -> bytes:
        if self._archive is None:
            raise VerificationError("OCI archive is not open")
        member = self._members.get(name)
        if member is None or not member.isfile():
            raise VerificationError(f"OCI archive member is missing or not a file: {name}")
        if member.size > max_size:
            raise VerificationError(f"OCI metadata member is too large: {name}")
        file_object = self._archive.extractfile(member)
        if file_object is None:
            raise VerificationError(f"cannot read OCI archive member: {name}")
        data = file_object.read(max_size + 1)
        if len(data) > max_size:
            raise VerificationError(f"OCI metadata member is too large: {name}")
        return data

    @staticmethod
    def _parse_json(data: bytes, description: str) -> dict[str, Any]:
        try:
            document = json.loads(data)
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise VerificationError(f"{description} is not valid JSON: {exc}") from exc
        return _as_object(document, description)

    def read_index(self) -> dict[str, Any]:
        data = self._read_member("index.json", max_size=MAX_METADATA_BLOB_SIZE)
        return self._parse_json(data, "OCI index.json")

    def read_descriptor(self, descriptor: dict[str, Any]) -> dict[str, Any]:
        digest = descriptor.get("digest")
        if not isinstance(digest, str):
            raise VerificationError("OCI descriptor digest is missing")
        match = SHA256_DIGEST_RE.fullmatch(digest)
        if match is None:
            raise VerificationError(f"unsupported or malformed OCI digest: {digest}")

        size = descriptor.get("size")
        if not isinstance(size, int) or isinstance(size, bool) or size < 0:
            raise VerificationError(f"invalid size for OCI descriptor {digest}")
        if size > MAX_METADATA_BLOB_SIZE:
            raise VerificationError(f"OCI metadata blob is too large: {digest}")

        data = self._read_member(
            f"blobs/sha256/{match.group(1)}", max_size=MAX_METADATA_BLOB_SIZE
        )
        if len(data) != size:
            raise VerificationError(f"size mismatch for OCI descriptor {digest}")
        if hashlib.sha256(data).hexdigest() != match.group(1):
            raise VerificationError(f"digest mismatch for OCI descriptor {digest}")
        return self._parse_json(data, f"OCI blob {digest}")


class PlatformManifestVerifier:
    def __init__(self, archive: OCIArchive, target_os: str, target_architecture: str) -> None:
        self._archive = archive
        self._target_os = target_os
        self._target_architecture = target_architecture
        self._visited: set[tuple[str, str, bool]] = set()
        self._visiting: set[tuple[str, str, bool]] = set()
        self._matches: set[str] = set()

    @staticmethod
    def _is_attestation(descriptor: dict[str, Any]) -> bool:
        annotations = descriptor.get("annotations", {})
        if not isinstance(annotations, dict):
            raise VerificationError("OCI descriptor annotations must be an object")
        return annotations.get("vnd.docker.reference.type") == ATTESTATION_REFERENCE_TYPE

    @staticmethod
    def _descriptors(document: dict[str, Any], description: str) -> list[dict[str, Any]]:
        manifests = document.get("manifests")
        if not isinstance(manifests, list):
            raise VerificationError(f"{description} manifests must be an array")
        return [
            _as_object(descriptor, f"{description} descriptor")
            for descriptor in manifests
        ]

    def verify(self, root_index: dict[str, Any]) -> str:
        media_type = root_index.get("mediaType")
        if media_type is not None and media_type not in OCI_INDEX_MEDIA_TYPES:
            raise VerificationError(f"unsupported root OCI index media type: {media_type}")
        for descriptor in self._descriptors(root_index, "root OCI index"):
            self._walk_descriptor(descriptor, inherited_attestation=False)

        if len(self._matches) != 1:
            raise VerificationError(
                "expected exactly one runnable "
                f"{self._target_os}/{self._target_architecture} image manifest, "
                f"found {len(self._matches)}"
            )
        return next(iter(self._matches))

    def _walk_descriptor(
        self, descriptor: dict[str, Any], *, inherited_attestation: bool
    ) -> None:
        media_type = descriptor.get("mediaType")
        digest = descriptor.get("digest")
        if not isinstance(media_type, str):
            raise VerificationError("OCI descriptor mediaType is missing")
        if not isinstance(digest, str):
            raise VerificationError("OCI descriptor digest is missing")

        is_attestation = inherited_attestation or self._is_attestation(descriptor)
        visit_key = (media_type, digest, is_attestation)
        if visit_key in self._visiting:
            raise VerificationError(f"cycle detected in OCI indexes at {digest}")
        if visit_key in self._visited:
            return

        self._visiting.add(visit_key)
        try:
            document = self._archive.read_descriptor(descriptor)
            document_media_type = document.get("mediaType")
            if document_media_type is not None and document_media_type != media_type:
                raise VerificationError(
                    f"media type mismatch for OCI descriptor {digest}: "
                    f"{media_type} != {document_media_type}"
                )

            if media_type in OCI_INDEX_MEDIA_TYPES:
                for child in self._descriptors(document, f"OCI index {digest}"):
                    self._walk_descriptor(
                        child, inherited_attestation=is_attestation
                    )
            elif media_type in OCI_MANIFEST_MEDIA_TYPES:
                self._inspect_manifest(
                    descriptor,
                    document,
                    is_attestation or self._is_attestation(document),
                )
            elif is_attestation:
                # Explicitly labelled attestations are metadata, not runnable
                # images. Their descriptor/blob integrity was still checked.
                pass
            else:
                raise VerificationError(
                    f"unsupported OCI descriptor media type {media_type} at {digest}"
                )
        finally:
            self._visiting.remove(visit_key)
        self._visited.add(visit_key)

    def _inspect_manifest(
        self,
        manifest_descriptor: dict[str, Any],
        manifest: dict[str, Any],
        is_attestation: bool,
    ) -> None:
        if is_attestation:
            return

        config = _as_object(manifest.get("config"), "OCI image manifest config")
        config_media_type = config.get("mediaType")
        if config_media_type not in IMAGE_CONFIG_MEDIA_TYPES:
            # OCI indexes may also carry non-runnable artifacts. They are not
            # candidate images and therefore cannot satisfy the platform gate.
            return
        image_config = self._archive.read_descriptor(config)
        image_os = image_config.get("os")
        image_architecture = image_config.get("architecture")
        if not isinstance(image_os, str) or not isinstance(image_architecture, str):
            return

        platform = manifest_descriptor.get("platform")
        if platform is not None:
            platform = _as_object(platform, "OCI manifest descriptor platform")
            descriptor_os = platform.get("os")
            descriptor_architecture = platform.get("architecture")
            if descriptor_os not in (None, "", "unknown", image_os):
                raise VerificationError(
                    f"platform OS disagrees with image config for {manifest_descriptor['digest']}"
                )
            if descriptor_architecture not in (
                None,
                "",
                "unknown",
                image_architecture,
            ):
                raise VerificationError(
                    "platform architecture disagrees with image config for "
                    f"{manifest_descriptor['digest']}"
                )

        if (
            image_os == self._target_os
            and image_architecture == self._target_architecture
        ):
            self._matches.add(str(manifest_descriptor["digest"]))


def select_manifest(archive_path: str, platform: str) -> str:
    parts = platform.split("/")
    if len(parts) != 2 or not all(parts):
        raise VerificationError("platform must use the os/architecture form")
    target_os, target_architecture = parts
    with OCIArchive(archive_path) as archive:
        verifier = PlatformManifestVerifier(archive, target_os, target_architecture)
        return verifier.verify(archive.read_index())


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Recursively verify an OCI archive and print the unique runnable "
            "manifest digest for a requested platform."
        )
    )
    parser.add_argument("archive", help="path to an OCI archive tar file")
    parser.add_argument("--platform", required=True, help="target in os/architecture form")
    args = parser.parse_args()

    try:
        print(select_manifest(args.archive, args.platform))
    except VerificationError as exc:
        print(f"OCI platform verification failed: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
