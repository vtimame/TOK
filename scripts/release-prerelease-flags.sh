#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
if [ -z "${tag}" ]; then
  echo "usage: $0 <tag>" >&2
  exit 2
fi

semver_identifier='[0-9A-Za-z-]+'
prerelease_regex="^v[0-9]+\\.[0-9]+\\.[0-9]+-${semver_identifier}(\\.${semver_identifier})*(\\+${semver_identifier}(\\.${semver_identifier})*)?$"

if [[ "${tag}" =~ ${prerelease_regex} ]]; then
  printf '%s\n' "--prerelease"
fi
