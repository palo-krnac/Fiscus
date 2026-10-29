#!/bin/sh
# Zostaví Fiskus.app (univerzálna binárka arm64 + amd64) a obraz .dmg.
# Použitie (na macOS): packaging/macos/build-app.sh 1.2.3 dist
set -eu
VERSION=$1
DIST=$2
HERE=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
APP="$WORK/Fiškus.app"

mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
lipo -create -output "$APP/Contents/MacOS/fiskus" \
  "$DIST/fiskus-darwin-arm64" "$DIST/fiskus-darwin-amd64"
chmod 755 "$APP/Contents/MacOS/fiskus"
sed "s/__VERSION__/$VERSION/g" "$HERE/Info.plist" > "$APP/Contents/Info.plist"
cp "$HERE/../icons/fiskus.icns" "$APP/Contents/Resources/fiskus.icns"
# podpis „ad hoc“ (bez certifikátu Apple): Apple Silicon vyžaduje aspoň ten
codesign --force --deep --sign - "$APP"

ln -s /Applications "$WORK/Aplikácie"
hdiutil create -volname "Fiškus $VERSION" -srcfolder "$WORK" -ov -format UDZO \
  "$DIST/fiskus-$VERSION-macos.dmg"
rm -rf "$WORK"
