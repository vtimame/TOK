#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
if [ -z "${tag}" ]; then
  echo "usage: $0 <tag>" >&2
  exit 2
fi

notes_file="docs/releases/${tag}.md"
if [ ! -f "${notes_file}" ]; then
  echo "release notes file is required: ${notes_file}" >&2
  echo "copy docs/releases/TEMPLATE.md and fill it with verified release details" >&2
  exit 1
fi

if ! grep -q "^## TOK ${tag}$" "${notes_file}"; then
  echo "${notes_file} must start with heading: ## TOK ${tag}" >&2
  exit 1
fi

for section in Added Changed Fixed Migration; do
  if ! grep -q "^## ${section}$" "${notes_file}"; then
    echo "${notes_file} must include a ## ${section} section" >&2
    exit 1
  fi
done

if ! grep -q "^## Contract Notes$" "${notes_file}"; then
  echo "${notes_file} must include a ## Contract Notes section for CLI/MCP/HTTP changes" >&2
  exit 1
fi

if grep -Eiq "TODO|TBD|placeholder|pending|not run" "${notes_file}"; then
  echo "${notes_file} contains placeholder or unverified release-note text" >&2
  exit 1
fi

if ! grep -Eq "^- .+" "${notes_file}"; then
  echo "${notes_file} must include at least one release-note bullet" >&2
  exit 1
fi

printf '%s\n' "${notes_file}"
