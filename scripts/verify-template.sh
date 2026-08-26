#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
template_dir="$repository_root/internal/templates/self-hosted-v0.8.0"
expected="af4f6b1589fbf5efe57a4679f33f7f3b8d8c46b33a36c733ce77c4581ce25a81"

actual=$(
  cd "$template_dir"
  find . -type f -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256 | awk '{print $1}'
)

if [ "$actual" != "$expected" ]; then
  echo "official template checksum mismatch: expected $expected, got $actual" >&2
  exit 1
fi

echo "official Supabase self-hosted/v0.8.0 template verified"
