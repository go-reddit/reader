#!/usr/bin/env bash
# BSD 3-Clause License
#
# Copyright (c) 2026, the go-reddit/reader authors
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the conditions of the BSD 3-Clause
# License (see the repository LICENSE file) are met.
#
# ---------------------------------------------------------------------------
# Negative control for the bricolint hand-drawn-UI guard.
#
# A guard that never fires proves nothing. This script proves bricolint bites:
#
#   1. the tree is clean            -> go vet exits 0
#   2. inject a raw painter call    -> a real painter-using Draw method now
#      into internal/ui/scene.go       makes a bare p.FillRect(...) call
#   3. run the guard                -> go vet exits NON-zero (the guard bites)
#   4. restore the file             -> go vet exits 0 again
#
# The injection targets a REAL site: Scene.Draw in internal/ui/scene.go, where a
# *painter.PixelPainter named `p` and a theme `th` are already in scope. The
# original file is restored on every exit path via a trap.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

export GOWORK=off

# Resolve the bricolint binary: honour $BRICOLINT, else fall back to GOPATH/bin.
BRICOLINT="${BRICOLINT:-$(go env GOPATH)/bin/bricolint}"
if [ ! -x "$BRICOLINT" ]; then
  echo "negative-control: bricolint not found at '$BRICOLINT'" >&2
  echo "  set \$BRICOLINT or run: go install github.com/go-widgets/bricolint/cmd/bricolint@v0.1.0" >&2
  exit 2
fi

target="internal/ui/scene.go"
# Anchor: the first fill of the whole-scene background inside Scene.Draw. Both
# `p` (the PixelPainter) and `th` (the theme) are in scope on this line.
anchor='fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)'
inject='	p.FillRect(painter.Rect{X: 0, Y: 0, W: 1, H: 1}, th.Background) // bricolint-negative-control: injected raw primitive'

# Restore the target ONLY once a real backup has been taken (restore_armed=1)
# and it is non-empty, so a failure before the cp below cannot make the trap
# copy an empty temp over $target and wipe it.
backup="$(mktemp)"
restore_armed=0
restore() { [ "$restore_armed" = 1 ] && [ -s "$backup" ] && cp "$backup" "$target"; rm -f "$backup"; return 0; }
trap restore EXIT
cp "$target" "$backup"; restore_armed=1

run_vet() { go vet -vettool="$BRICOLINT" ./... >/dev/null 2>&1; }

# --- 1. clean tree must pass ------------------------------------------------
if ! run_vet; then
  echo "negative-control: FAIL — guard flagged the clean tree (expected exit 0)" >&2
  exit 1
fi
echo "negative-control: clean tree passes (exit 0) — ok"

# --- 2. inject a raw painter primitive into a real Draw method --------------
if ! grep -qF "$anchor" "$target"; then
  echo "negative-control: FAIL — anchor line not found in $target" >&2
  echo "  the app changed; update the anchor to a real painter-using Draw site" >&2
  exit 1
fi
awk -v anchor="$anchor" -v inject="$inject" '
  { print }
  index($0, anchor) { print inject }
' "$target" > "$target.tmp" && mv "$target.tmp" "$target"

# --- 3. the guard must now bite ---------------------------------------------
if run_vet; then
  echo "negative-control: FAIL — guard stayed silent after injecting p.FillRect (expected non-zero)" >&2
  exit 1
fi
echo "negative-control: injected raw p.FillRect is flagged (non-zero exit) — the guard bites"

# --- 4. restore and confirm clean again -------------------------------------
restore
trap - EXIT
if ! run_vet; then
  echo "negative-control: FAIL — guard still flags after restore (expected exit 0)" >&2
  exit 1
fi
echo "negative-control: restored tree passes (exit 0) — ok"
echo "negative-control: PASS"
