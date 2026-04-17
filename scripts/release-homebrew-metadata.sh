#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -ne 2 ]; then
  printf 'usage: %s <tag> <output-path>\n' "$0" >&2
  exit 1
fi

tag="$1"
output_path="$2"
repo="${GITHUB_REPOSITORY:-janeklb/git-stack}"

case "$tag" in
  v*) ;;
  *)
    printf 'tag must start with v: %s\n' "$tag" >&2
    exit 1
    ;;
esac

source_url="https://github.com/${repo}/archive/refs/tags/${tag}.tar.gz"
tmpdir="$(mktemp -d)"
archive_path="${tmpdir}/source.tar.gz"
build_commit="${GITHUB_SHA:-$(git rev-parse HEAD)}"
build_date="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

cleanup() {
  rm -rf "$tmpdir"
}

trap cleanup EXIT

curl -fsSL "$source_url" -o "$archive_path"

if command -v sha256sum >/dev/null 2>&1; then
  sha256="$(sha256sum "$archive_path" | cut -d' ' -f1)"
else
  sha256="$(shasum -a 256 "$archive_path" | cut -d' ' -f1)"
fi

mkdir -p "$(dirname "$output_path")"

cat > "$output_path" <<EOF
{
  "tag": "${tag}",
  "version": "${tag#v}",
  "build_commit": "${build_commit}",
  "build_date": "${build_date}",
  "source_url": "${source_url}",
  "sha256": "${sha256}"
}
EOF
