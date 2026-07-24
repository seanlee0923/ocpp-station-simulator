#!/usr/bin/env bash
# Builds the frontend, copies its output into the Go binary's embed
# directory, then builds a single self-contained backend executable.
# Usage: ./build.sh [output-path]  (default: ./ocpp-station-simulator)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT="${1:-$ROOT_DIR/ocpp-station-simulator}"

echo "==> building frontend"
(cd "$ROOT_DIR/frontend" && npm install && npm run build)

echo "==> copying frontend build into backend/internal/webui/dist"
rm -rf "$ROOT_DIR/backend/internal/webui/dist"
cp -r "$ROOT_DIR/frontend/dist" "$ROOT_DIR/backend/internal/webui/dist"

echo "==> building backend"
(cd "$ROOT_DIR/backend" && go build -o "$OUTPUT" ./cmd/server)

echo "==> done: $OUTPUT"
echo "    run it with: $OUTPUT"
echo "    (defaults to SQLite at ./data/ocpp-simulator.db; set DB_DRIVER=mysql and DB_DSN=... for MySQL)"
