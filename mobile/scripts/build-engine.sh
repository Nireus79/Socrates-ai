#!/bin/bash
# Builds the Go engine as libsocratesengine.so for each Android CPU type into the app's jniLibs (Android runs files from there).
# Needs Go and the Android NDK (ANDROID_NDK_HOME, or ANDROID_HOME/ndk/<version>). Default: arm64-v8a (the phones);
# ABIS="arm64-v8a x86_64" adds the emulator.
set -euo pipefail
cd "$(dirname "$0")/.."
NDK=${ANDROID_NDK_HOME:-$(ls -d "${ANDROID_HOME:?set ANDROID_HOME or ANDROID_NDK_HOME}"/ndk/* | sort -V | tail -1)}
BIN=$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin
API=24
OUT=$PWD/android/app/src/main/jniLibs
ABIS=${ABIS:-"arm64-v8a"}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
rm -rf "$OUT"
for abi in $ABIS; do
  case $abi in
    arm64-v8a) export GOARCH=arm64 GOARM= CC=$BIN/aarch64-linux-android$API-clang ;;
    x86_64)    export GOARCH=amd64 GOARM= CC=$BIN/x86_64-linux-android$API-clang ;;
    *) echo "unknown ABI $abi" >&2; exit 1 ;;
  esac
  mkdir -p "$OUT/$abi"
  echo "engine for $abi"
  (cd ../engine && GOOS=android CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.version=$VERSION -extldflags=-Wl,-z,max-page-size=16384" -o "$OUT/$abi/libsocratesengine.so" ./cmd/socrates-engine)
done
