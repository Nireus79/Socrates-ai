#!/bin/bash
# Usage: scripts/build-apk.sh [debug|release]   Builds the screens, syncs Capacitor and runs Gradle; prints the APK path.
# Needs JDK 21, the Android SDK (platform 36, build-tools 36), the engine built (scripts/build-engine.sh) and Node.
set -euo pipefail
cd "$(dirname "$0")/.."
VARIANT=${1:-debug}
scripts/build-www.sh
[ -d node_modules ] || npm ci
npx cap sync android >/dev/null
cd android
if [ "$VARIANT" = release ]; then ./gradlew --no-daemon assembleRelease; else ./gradlew --no-daemon assembleDebug; fi
find app/build/outputs/apk -name '*.apk' -newer ../scripts/build-apk.sh | head -3
