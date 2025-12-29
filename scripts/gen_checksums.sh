#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   scripts/gen_checksums.sh <artifact-dir> > checksums.txt
#
# Expects release artifacts (binaries) to be present in <artifact-dir>.

dir="${1:-}"
if [[ -z "${dir}" || ! -d "${dir}" ]]; then
  echo "usage: $0 <artifact-dir>" >&2
  exit 2
fi

hash_cmd=""
if command -v sha256sum >/dev/null 2>&1; then
  hash_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  hash_cmd="shasum -a 256"
else
  echo "error: need sha256sum or shasum" >&2
  exit 1
fi

cd "${dir}"

# Only hash regular files (ignore directories).
while IFS= read -r -d '' f; do
  ${hash_cmd} "${f#./}"
done < <(find . -maxdepth 1 -type f -print0 | sort -z)

