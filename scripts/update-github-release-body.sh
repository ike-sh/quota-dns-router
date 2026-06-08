#!/usr/bin/env bash
# 用法：GITHUB_TOKEN=ghp_xxx ./scripts/update-github-release-body.sh v0.2.0 docs/releases/v0.2.0.md
set -euo pipefail

tag="${1:-}"
body_file="${2:-}"
repo="${GITHUB_REPOSITORY:-ike-sh/quota-dns-router}"

if [[ -z "${tag}" || -z "${body_file}" ]]; then
  echo "usage: GITHUB_TOKEN=... $0 <tag> <markdown-file>" >&2
  exit 1
fi
if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN is required" >&2
  exit 1
fi
if [[ ! -f "${body_file}" ]]; then
  echo "body file not found: ${body_file}" >&2
  exit 1
fi

body="$(python3 -c 'import json,sys; print(json.dumps(open(sys.argv[1], encoding="utf-8").read()))' "${body_file}")"
payload="$(printf '{"body":%s}' "${body}")"

curl -fsSL -X PATCH \
  -H "Authorization: Bearer ${GITHUB_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  "https://api.github.com/repos/${repo}/releases/tags/${tag}" \
  -d "${payload}"

echo "updated release body for ${tag}"
