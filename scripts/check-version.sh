#!/bin/bash
# The Android version in mobile/android/app/build.gradle is the one place the version lives (F-Droid and Play both read it from the
# repository). With a tag as the argument (v0.1.0-beta.3) the tag must equal "v" + versionName; without one, only the shape is checked.
# versionCode must be a whole number and must grow with every release (compared with the previous tag's value when git has it).
set -euo pipefail
cd "$(dirname "$0")/.."
g=mobile/android/app/build.gradle
name=$(sed -n 's/^ *versionName *"\(.*\)" *$/\1/p' "$g")
code=$(sed -n 's/^ *versionCode *\([0-9]*\) *$/\1/p' "$g")
[ -n "$name" ] && [ -n "$code" ] || { echo "versionName or versionCode not found in $g" >&2; exit 1; }
case "$name" in [0-9]*.[0-9]*.[0-9]*) ;; *) echo "versionName '$name' does not look like 0.1.0 or 0.1.0-beta.3" >&2; exit 1 ;; esac
tag=${1:-}
if [ -n "$tag" ] && [ "$tag" != "v$name" ]; then echo "the tag $tag and versionName $name in $g disagree" >&2; exit 1; fi
prev=$(git tag --list 'v*' --sort=-v:refname 2>/dev/null | grep -vx "${tag:-none}" | head -1 || true)
if [ -n "$prev" ]; then
  pc=$(git show "$prev:$g" 2>/dev/null | sed -n 's/^ *versionCode *\([0-9]*\) *$/\1/p' || true)
  if [ -n "$pc" ] && [ -n "$tag" ] && [ "$code" -le "$pc" ]; then echo "versionCode $code must be above $pc of $prev" >&2; exit 1; fi
fi
echo "version $name, code $code${tag:+, tag $tag}: ok"
