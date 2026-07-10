#!/usr/bin/env bash
# Build the WebAssembly front-end into the server's embed directory and copy
# the matching wasm_exec.js glue. Run before `go build` so the embedded bundle
# is up to date.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
assets="$root/internal/server/assets"

echo "building reader.wasm (GOOS=js GOARCH=wasm, CGO=0)…"
GOWORK=off CGO_ENABLED=0 GOOS=js GOARCH=wasm \
	go build -trimpath -ldflags="-s -w" -o "$assets/reader.wasm" "$root/cmd/front"

goroot="$(go env GOROOT)"
if [ -f "$goroot/lib/wasm/wasm_exec.js" ]; then
	cp "$goroot/lib/wasm/wasm_exec.js" "$assets/wasm_exec.js"
elif [ -f "$goroot/misc/wasm/wasm_exec.js" ]; then
	cp "$goroot/misc/wasm/wasm_exec.js" "$assets/wasm_exec.js"
else
	echo "warning: wasm_exec.js not found under GOROOT; keeping the committed copy" >&2
fi

echo "wrote $assets/reader.wasm ($(du -h "$assets/reader.wasm" | cut -f1))"
