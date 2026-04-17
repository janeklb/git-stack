#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -ne 4 ]; then
  printf 'usage: %s <tap-repo> <tag> <source-url> <sha256>\n' "$0" >&2
  exit 1
fi

if [ -z "${GH_TOKEN:-}" ]; then
  printf 'GH_TOKEN must be set so the release workflow can open a Homebrew tap issue.\n' >&2
  exit 1
fi

tap_repo="$1"
tag="$2"
source_url="$3"
sha256="$4"
title="[git-stack] Release ${tag}"
release_url="https://github.com/janeklb/git-stack/releases/tag/${tag}"

existing_url="$(gh issue list \
  --repo "$tap_repo" \
  --state all \
  --search "\"$title\" in:title" \
  --limit 1 \
  --json url \
  --jq '.[0].url // ""')"

if [ -n "$existing_url" ]; then
  printf 'reusing existing tap issue: %s\n' "$existing_url"
  exit 0
fi

body_file="$(mktemp)"

cleanup() {
  rm -f "$body_file"
}

trap cleanup EXIT

{
  printf '## Summary\n\n'
  printf 'Update the `janeklb/homebrew-tap` formula for `git-stack` %s.\n\n' "$tag"
  printf '## Release metadata\n\n'
  printf -- '- Tag: `%s`\n' "$tag"
  printf -- '- Release: %s\n' "$release_url"
  printf -- '- Source URL: `%s`\n' "$source_url"
  printf -- '- SHA256: `%s`\n\n' "$sha256"
  printf '## Expected tap work\n\n'
  printf -- '- Update `Formula/git-stack.rb` to %s\n' "$tag"
  printf -- '- Use the immutable tagged source archive above\n'
  printf -- '- Regenerate any completion install logic needed by the formula\n'
  printf -- '- Open a PR in `janeklb/homebrew-tap` for review\n'
} > "$body_file"

gh issue create \
  --repo "$tap_repo" \
  --title "$title" \
  --body-file "$body_file"
