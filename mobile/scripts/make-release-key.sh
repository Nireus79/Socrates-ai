#!/bin/bash
# Usage: scripts/make-release-key.sh /path/outside/the/repo/socrates-release.jks
# Creates the Play upload key once. Keep the file and its password safe and OUT of the repository: losing it
# means losing the ability to update the app under the same upload key. The password is read from the terminal.
set -euo pipefail
OUT=${1:?path of the keystore to create (outside the repository)}
case "$(cd "$(dirname "$OUT")" && pwd)/" in
  "$(cd "$(dirname "$0")/../.." && pwd)"/*) echo "refusing: the key must live outside the repository" >&2; exit 1;;
esac
[ -e "$OUT" ] && { echo "refusing: $OUT exists" >&2; exit 1; }
read -r -s -p "Keystore password (12+ characters): " PW; echo
[ "${#PW}" -ge 12 ] || { echo "too short" >&2; exit 1; }
keytool -genkeypair -v -keystore "$OUT" -alias socrates -keyalg RSA -keysize 4096 -validity 10000 \
  -storepass "$PW" -keypass "$PW" -dname "CN=Socrates, O=Hermes Software"
chmod 600 "$OUT"
echo "Created $OUT. Build with: SOCRATES_KEYSTORE=$OUT SOCRATES_KEYSTORE_PASSWORD=... scripts/build-apk.sh release"
