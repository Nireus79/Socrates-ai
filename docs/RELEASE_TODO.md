# What remains until release

Written 2026-10-24, after commit 4039ba4. The beta is feature-complete; what is left is testing, paperwork and platform checks. "You" is the founder; "me" is Claude.

## Decided 2026-11-03 (founder): F-Droid first, Google Play waits
- Signing key made (`~/android-dev/socrates/`, backed up on USB, secrets `SOCRATES_KEYSTORE_*` set in the repository). It signs only the APK on GitHub releases; F-Droid signs its own build.
- Version lives in `mobile/android/app/build.gradle`, checked against the tag by `scripts/check-version.sh`; one `v*` tag builds desktop and Android (`release-desktop.yml`, never run on GitHub yet).
- Draft F-Droid recipe: `mobile/fdroid/io.github.nireus79.socrates.yml` (not run in F-Droid's build image; needs the tag it names). Listing text and images: `fastlane/metadata/android/en-US/` (phone screenshots missing: add `phoneScreenshots/1.png` and more from the finished app). The unused Google services plugin was removed from the Android build for F-Droid's scanner.
- Next: cut the first tag, run the recipe in the fdroidserver build image, then open the merge request on F-Droid's GitLab (fdroiddata).

## Needed from you
1. **Phone run.** Done on 2026-10-26 on a Mi A2 (Android 10, arm64) with a debug build (SESSION 65); a release build and the idle restart on that phone are still to do. Original note: plug in the phone, enable USB debugging, accept the prompt. I build and install a release build and drive it with your key (you enter the key in the app yourself). First run on real arm64 hardware: Back key, engine restart after idle, signed build, keyboard. Untested: Android 8 to 12, share-in, the save-as export picker, low-memory kills.
2. **Your own full test**, including a read of the Greek texts (`app/i18n.js`, `engine/internal/safety/texts.go`, the crisis message above all) by a native reader, and one click on the new key-help buttons in the desktop window (the click itself was only tested on the Android emulator).
3. **Android signing key.** `mobile/scripts/make-release-key.sh` creates it outside the repo (`SOCRATES_KEYSTORE`, `SOCRATES_KEYSTORE_PASSWORD`). Back it up: a lost key means the app can never be updated.
4. **Play console** (details and "check" marks in `docs/PLAY_LISTING.md`): privacy policy URL (the English `docs/PRIVACY.md` needs hosting somewhere), website (optional), phone screenshots, content rating, data safety and AI-content declarations, a reviewer key with a small spend limit, and the closed test: confirmed by the founder on 2026-11-02 that Play requires one, and that Moly's own closed test is not finished yet, so Socrates waits for it. The Greek listing and Greek privacy page were dropped: Greek is functionality only.
5. **Desktop route.** Profiles (log in and register) are built (SESSION 65); click through them in the real window. Linux builds and runs. Windows is only cross-compiled (`make -C desktop windows`; needs WebView2, not run); macOS needs a Mac (not attempted). Nothing is signed or packaged. Choose the route (GitHub releases, itch.io, Steam); an unsigned Windows build shows a SmartScreen warning.

## Mine, once the above is ready
- Cut one `v*` tag (desktop and Android; set versionName and versionCode in build.gradle first; the signing secrets SOCRATES_KEYSTORE_BASE64 and SOCRATES_KEYSTORE_PASSWORD must exist) and write release notes.
- Packaging for the chosen desktop route.
- Rebuild release artifacts from a clean engine: the Android engine must be built without `GO_TAGS=testprovider` (`build-apk.sh release|bundle` refuses otherwise).
- Fix what the phone run and your tests find.

## After the beta (waits for your decisions)
- Play Billing, ads, consent screens, the Support screens, the sponsor and tip links (GitHub Sponsors file is done; the key-help link opener exists and can be reused). Open: ad network, what the supporter product contains, prices (`docs/MONETIZATION.md`).
- Daily limits for turns and suggestions (numbers need beta data).
- Tune the question-finishing thresholds on real people (`progress.Default`).
- Stricter age limits (the whole app is 16+ for now).

## Known limits
- Local models were removed (PLAN 19): Claude or OpenAI with the person's own key only.
- Greek: screens, fixed texts and answers are covered; store listing, privacy page and character sheets stay English. Only a spot check of Greek fidelity exists (12 questions).
- Live runs so far: safety battery, reply battery, fidelity (23 questions), thresholds (three talks), Greek spot check. A longer live fidelity run and long conversations are not done.
- The emulator and browser test scripts are not in the repository: copies are in `~/socrates-test-scripts` (README.txt there says how to run them). Test engines are built with `-tags testprovider` and are never released.

## Open on 2026-11-02 (founder)
- Play needs a closed test; Moly's is still running, so the Android release waits.
- Scoring: on 2026-11-02 the founder asked for progress to be visible for every philosopher even when unlocked; built as the Progress screen and per-philosopher profiles (PLAN 16). The per-answer grade of PLAN 12.4 is still not built.

## Desktop release route (decided by the founder, 2026-11-02: GitHub releases)
- `desktop/scripts/package.sh linux|windows vX.Y.Z` builds the window and the engine and packs `dist/socrates-<version>-<platform>-amd64.(tar.gz|zip)` with a `.sha256`. It refuses to pack an engine built with the `testprovider` tag (checked by building one). Both packages were built locally on 2026-11-02 (Linux contents and `--version` checked; the Windows zip was only built).
- `.github/workflows/release-desktop.yml`: a pushed tag `v*` runs the gate (engine `make check`, app `npm test`), builds both packages and opens a **draft pre-release**; nothing goes public until the founder publishes the draft. The workflow was linted (actionlint) but has never run on GitHub.
- To cut a beta: `git tag v0.1.0-beta.1 && git push origin v0.1.0-beta.1`, then publish the draft. Unsigned: Windows shows SmartScreen. Not run on Windows or macOS (macOS is not built).
