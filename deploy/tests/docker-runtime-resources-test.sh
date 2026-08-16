#!/bin/sh
# Assertions below intentionally use literal shell expressions from Dockerfiles.
# shellcheck disable=SC2016
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

fail() {
  printf 'docker runtime resources test failed: %s\n' "$1" >&2
  exit 1
}

assert_line() {
  file=$1
  line=$2
  grep -Fqx "$line" "$file" || fail "$file is missing: $line"
}

assert_count() {
  file=$1
  line=$2
  expected=$3
  actual=$(grep -Fxc "$line" "$file" || true)
  [ "$actual" -eq "$expected" ] || fail "$file has $actual occurrences of '$line', expected $expected"
}

test -s backend/resources/model-pricing/model_prices_and_context_window.json || \
  fail 'fallback pricing data is missing or empty'

assert_line Dockerfile '# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e'
assert_line Dockerfile 'ARG NODE_IMAGE=node:24-alpine@sha256:f70403e87646dc51b45295f4b8b70cdad0b63d2297c4c9899119b03f7af7a6b3'
assert_line Dockerfile 'ARG GOLANG_IMAGE=golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df'
assert_line Dockerfile 'ARG ALPINE_IMAGE=alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d'
assert_line Dockerfile 'ARG POSTGRES_IMAGE=postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
assert_line Dockerfile 'ARG PNPM_VERSION=9.15.9'
assert_line Dockerfile 'RUN corepack enable && corepack prepare "pnpm@${PNPM_VERSION}" --activate'
assert_line deploy/Dockerfile 'ARG NODE_IMAGE=node:24-alpine@sha256:f70403e87646dc51b45295f4b8b70cdad0b63d2297c4c9899119b03f7af7a6b3'
assert_line deploy/Dockerfile 'ARG GOLANG_IMAGE=golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df'
assert_line deploy/Dockerfile 'ARG ALPINE_IMAGE=alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc'
assert_line Dockerfile.goreleaser 'ARG ALPINE_IMAGE=alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d'
assert_line Dockerfile.goreleaser 'ARG POSTGRES_IMAGE=postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
assert_line backend/Dockerfile 'ARG GOLANG_IMAGE=golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df'
assert_line Dockerfile.goreleaser 'COPY --chown=sub2api:sub2api backend/resources /app/resources'
assert_line deploy/Dockerfile 'COPY --from=backend-builder --chown=sub2api:sub2api /app/backend/resources /app/resources'
assert_line Dockerfile '    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/livez || exit 1'
assert_line deploy/Dockerfile '    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/livez || exit 1'
for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.dev.yml \
  deploy/docker-compose.standalone.yml; do
  grep -F 'http://localhost:8080/readyz' "$compose_file" >/dev/null || \
    fail "$compose_file must use /readyz for service health"
done
assert_count .goreleaser.yaml '      - backend/resources' 4
assert_count .goreleaser.simple.yaml '      - backend/resources' 1

printf 'docker runtime resources test passed\n'
