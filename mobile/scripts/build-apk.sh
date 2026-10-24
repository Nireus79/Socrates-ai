#!/bin/bash
# Usage: scripts/build-apk.sh [debug|release|bundle]   Builds the screens, syncs Capacitor and runs Gradle; prints the APK path.
# Needs JDK 21, the Android SDK (platform 36, build-tools 36), the engine built (scripts/build-engine.sh) and Node.
set -euo pipefail
cd "$(dirname "$0")/.."
VARIANT=${1:-debug}
if [ "$VARIANT" != debug ] && [ -e .test-engine-build ]; then
  echo "refused: the engine in jniLibs was built with GO_TAGS=testprovider (emulator runs only). Run scripts/build-engine.sh again without it." >&2; exit 1
fi
scripts/build-www.sh
[ -d node_modules ] || npm ci
npx cap sync android >/dev/null
cd android
if [ "$VARIANT" = release ]; then ./gradlew --no-daemon assembleRelease
elif [ "$VARIANT" = bundle ]; then ./gradlew --no-daemon bundleRelease   # the Play Store takes an app bundle, not an APK
else ./gradlew --no-daemon assembleDebug; fi
if [ "$VARIANT" != debug ] && [ -z "${SOCRATES_KEYSTORE:-}" ]; then echo "note: SOCRATES_KEYSTORE not set; the release APK is UNSIGNED and cannot be installed" >&2; fi
find app/build/outputs -name '*.apk' -o -name '*.aab' | head -3
