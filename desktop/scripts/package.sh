#!/usr/bin/env bash
# Builds the desktop window and the engine for one platform and packs them for a GitHub release.
#   desktop/scripts/package.sh linux   v0.1.0-beta.1
#   desktop/scripts/package.sh windows v0.1.0-beta.1
# Output: dist/socrates-<version>-<platform>-amd64.(tar.gz|zip) and its .sha256 (from the repository root).
# Linux needs libgtk-3-dev and libwebkit2gtk-4.1-dev; Windows is cross-compiled and needs go-winres
# (go install github.com/tc-hib/go-winres@v0.3.3). Nothing is signed.
set -euo pipefail

platform="${1:?platform: linux or windows}"
version="${2:?version, for example v0.1.0-beta.1}"
case "$platform" in linux | windows) ;; *) echo "unknown platform: $platform" >&2; exit 2 ;; esac
case "$version" in v[0-9]*) ;; *) echo "the version must look like v0.1.0" >&2; exit 2 ;; esac

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root/desktop"
name="socrates-${version}-${platform}-amd64"
stage="$root/dist/$name"
rm -rf "$stage" "$root/dist/$name".*
mkdir -p "$stage"

# the screens go into the window
make frontend

ldflags="-X main.version=${version}"
if [ "$platform" = linux ]; then
  (cd ../engine && CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$stage/socrates-engine" ./cmd/socrates-engine)
  CGO_ENABLED=1 go build -trimpath -tags "${TAGS:-webkit2_41},production" -o "$stage/socrates-desktop" .
  engine="$stage/socrates-engine"
  ext=""
else
  go-winres simply --arch amd64 --icon icon.png --manifest gui --product-name Socrates \
    --file-description "Socrates: ask a philosopher" --copyright "Hermes Software, MIT" --out rsrc
  (cd ../engine && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$stage/socrates-engine.exe" ./cmd/socrates-engine)
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags production -ldflags "-H windowsgui" -o "$stage/socrates-desktop.exe" .
  engine="$stage/socrates-engine.exe"
  ext=".exe"
fi

# a release must never carry the stand-in model door of the simulated tests
if grep -aq "SOCRATES_TEST_PROVIDER_URL" "$engine"; then
  echo "refusing to pack: the engine was built with the testprovider tag" >&2
  exit 1
fi

cp "$root/LICENSE" "$stage/"
cat > "$stage/README.txt" <<TXT
Socrates $version (beta, unsigned)

Start socrates-desktop${ext}. Socrates keeps everything on this device. You need your own Claude or OpenAI key; the app explains how to get one.
The data lives in ~/.socrates (on Windows, in your user profile folder, under .socrates).
Keep socrates-engine next to socrates-desktop: the window starts it.
TXT

cd "$root/dist"
if [ "$platform" = linux ]; then
  tar -czf "$name.tar.gz" "$name"
  archive="$name.tar.gz"
else
  (cd "$name" && zip -qr "../$name.zip" .)
  archive="$name.zip"
fi
sha256sum "$archive" > "$archive.sha256"
rm -rf "$stage"
echo "$root/dist/$archive"
