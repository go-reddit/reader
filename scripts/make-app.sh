#!/usr/bin/env bash
# Assemble a double-clickable macOS application bundle, "Reddit Reader.app".
# The bundle is a self-contained pure-Go binary (CGO=0) with the wasm UI
# embedded - no frameworks to ship, no dylibs to sign.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root/dist}"
app="$out/Reddit Reader.app"
name="reader"

"$root/scripts/build-wasm.sh"

echo "building $name (native, CGO=0)..."
GOWORK=off CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$out/$name" "$root"

echo "assembling app bundle..."
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$out/$name" "$app/Contents/MacOS/$name"

cat > "$app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>            <string>Reddit Reader</string>
  <key>CFBundleDisplayName</key>     <string>Reddit Reader</string>
  <key>CFBundleIdentifier</key>      <string>com.go-reddit.reader</string>
  <key>CFBundleVersion</key>         <string>0.1.0</string>
  <key>CFBundleShortVersionString</key><string>0.1.0</string>
  <key>CFBundlePackageType</key>     <string>APPL</string>
  <key>CFBundleExecutable</key>      <string>reader</string>
  <key>LSMinimumSystemVersion</key>  <string>11.0</string>
  <key>NSHighResolutionCapable</key> <true/>
</dict>
</plist>
PLIST

echo "built: $app"
echo "run it with:  open \"$app\"   (append --args -demo to try it without credentials)"
