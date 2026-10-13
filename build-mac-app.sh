#!/bin/sh
# Builds Hatrack.app, the macOS menu bar app, for Apple silicon and Intel in one bundle,
# with the hat CLI inside it for "Add Account…". Needs Go and the Xcode command line tools.
#
#   ./build-mac-app.sh             build and copy into /Applications (or ~/Applications)
#   NO_INSTALL=1 ./build-mac-app.sh  build ./Hatrack.app only
#   VERSION=v0.2.0 ./build-mac-app.sh
set -eu
[ "$(uname -s)" = Darwin ] || { echo "Hatrack.app builds on macOS only" >&2; exit 1; }

version="${VERSION:-dev}"
app=Hatrack.app
macos="$app/Contents/MacOS"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ld="-s -w -X main.version=$version"
for arch in arm64 amd64; do
  echo "building for $arch"
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "$ld" -o "$work/Hatrack-$arch" ./cmd/hatrack-gui
  CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "$ld" -o "$work/hat-$arch" ./cmd/hat
done

rm -rf "$app"
mkdir -p "$macos" "$app/Contents/Resources"
lipo -create -output "$macos/Hatrack" "$work/Hatrack-arm64" "$work/Hatrack-amd64"
lipo -create -output "$macos/hat" "$work/hat-arm64" "$work/hat-amd64"

short="$(echo "$version" | sed -n 's/^v\([0-9][0-9.]*\).*/\1/p')"
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>Hatrack</string>
  <key>CFBundleIdentifier</key><string>com.baodq97.hatrack</string>
  <key>CFBundleName</key><string>Hatrack</string>
  <key>CFBundleDisplayName</key><string>Hatrack</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${short:-0.0.0}</string>
  <key>CFBundleVersion</key><string>${short:-0.0.0}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF
# ad-hoc signature: Apple silicon will not run an unsigned bundle at all
codesign --force --deep --sign - "$app"
echo "built $app"

[ -n "${NO_INSTALL:-}" ] && exit 0
dest=/Applications
[ -w "$dest" ] || { dest="$HOME/Applications"; mkdir -p "$dest"; }
rm -rf "$dest/$app"
cp -R "$app" "$dest/"
echo "installed $dest/$app"
