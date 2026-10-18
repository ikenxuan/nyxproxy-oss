#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT/web"
if ! command -v pnpm >/dev/null 2>&1; then
  corepack enable
  corepack prepare pnpm@12.14.0 --activate
fi
pnpm install --frozen-lockfile
pnpm build

rm -rf "$ROOT/internal/webui/dist"
mkdir -p "$ROOT/internal/webui/dist"
cp -R out/. "$ROOT/internal/webui/dist/"
touch "$ROOT/internal/webui/dist/.gitkeep"

echo "web assets copied to internal/webui/dist"
