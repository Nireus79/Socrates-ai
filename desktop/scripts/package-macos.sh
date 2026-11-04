#!/usr/bin/env bash
# Builds the Socrates window and the engine for macOS (Apple Silicon and Intel in one universal app) and packs Socrates.app in a zip.
#   desktop/scripts/package-macos.sh v0.1.0-beta.6
# Output: dist/socrates-<version>-macos-universal.zip and its .sha256 (from the repository root).
# Must run ON a Mac (the window needs Apple's frameworks through cgo); the release workflow runs it on GitHub's macos-14 machine.
# Only a free ad-hoc signature is made: the app is NOT signed with an Apple developer certificate and NOT notarised, so macOS asks the person to
# allow it once (System Settings, Privacy & Security, "Open Anyway"). Nothing here has been run on a real Mac by the author.
set -euo pipefail

version="${1:?version, for example v0.1.0-beta.6}"
case "$version" in v[0-9]*) ;; *) echo "the version must look like v0.1.0" >&2; exit 2 ;; esac
[ "$(uname -s)" = Darwin ] || { echo "this script runs on macOS only" >&2; exit 2; }

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root/desktop"
name="socrates-${version}-macos-universal"
work="$root/dist/$name-work"
app="$work/Socrates.app"
rm -rf "$work" "$root/dist/$name".*
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$work/arm64" "$work/amd64"

make frontend

ldflags="-X main.version=${version}"
# what `wails build` passes on a Mac and a plain `go build` does not: the Apple framework the window needs (the link failed without it), and the oldest macOS
export CGO_LDFLAGS="-framework UniformTypeIdentifiers -mmacosx-version-min=11.0"
export CGO_CFLAGS="-mmacosx-version-min=11.0"
for arch in arm64 amd64; do
  (cd ../engine && GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$work/$arch/socrates-engine" ./cmd/socrates-engine)
  GOOS=darwin GOARCH=$arch CGO_ENABLED=1 go build -trimpath -tags production -o "$work/$arch/socrates-desktop" .
done
lipo -create -output "$app/Contents/MacOS/socrates-engine" "$work/arm64/socrates-engine" "$work/amd64/socrates-engine"
lipo -create -output "$app/Contents/MacOS/socrates-desktop" "$work/arm64/socrates-desktop" "$work/amd64/socrates-desktop"
chmod +x "$app/Contents/MacOS/"*

# a release must never carry the stand-in model door of the simulated tests
if grep -aq "SOCRATES_TEST_PROVIDER_URL" "$app/Contents/MacOS/socrates-engine"; then
  echo "refusing to pack: the engine was built with the testprovider tag" >&2
  exit 1
fi

# the icon, from the 512 px picture (the names are the ones iconutil asks for)
iconset="$work/icon.iconset"
mkdir -p "$iconset"
for s in 16 32 128 256 512; do
  sips -z $s $s icon.png --out "$iconset/icon_${s}x${s}.png" >/dev/null
  sips -z $((s * 2)) $((s * 2)) icon.png --out "$iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/icon.icns"

short="${version#v}"
cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>Socrates</string>
  <key>CFBundleDisplayName</key><string>Socrates</string>
  <key>CFBundleIdentifier</key><string>io.github.nireus79.socrates.desktop</string>
  <key>CFBundleExecutable</key><string>socrates-desktop</string>
  <key>CFBundleIconFile</key><string>icon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${short%%-*}</string>
  <key>CFBundleVersion</key><string>${short}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.education</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSHumanReadableCopyright</key><string>Hermes Software, MIT</string>
</dict>
</plist>
PLIST
cp "$root/LICENSE" "$work/"
cat > "$work/README.txt" <<TXT
Socrates $version for macOS (beta, NOT signed with an Apple certificate, NOT notarised, never run on a Mac by its author)

1. Drag Socrates.app to Applications.
2. The first time, macOS says it cannot verify the app. Open System Settings, Privacy & Security, scroll down to the message about Socrates and press "Open Anyway". On older macOS: right-click the app, then Open.
3. You need your own Claude or OpenAI key; the app explains how to get one. Everything stays on this Mac (in ~/.socrates).
TXT

# the free ad-hoc signature, then a check that the bundle is whole
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict "$app"
lipo -info "$app/Contents/MacOS/socrates-desktop" "$app/Contents/MacOS/socrates-engine"

mkdir -p "$root/dist"
(cd "$work" && ditto -c -k --keepParent Socrates.app "$root/dist/$name.zip.tmp" && cd "$root/dist" && mv "$name.zip.tmp" "$name.zip")
# the README and the licence go in the same archive, beside the app
(cd "$work" && zip -q "$root/dist/$name.zip" README.txt LICENSE)
(cd "$root/dist" && shasum -a 256 "$name.zip" > "$name.zip.sha256")
rm -rf "$work"
echo "$root/dist/$name.zip"
