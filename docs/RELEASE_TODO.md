# What remains until release

Written 2026-10-24, after commit 4039ba4. The beta is feature-complete; what is left is testing, paperwork and platform checks. "You" is the founder; "me" is Claude.

## Needed from you
1. **Phone run.** Plug in the phone, enable USB debugging, accept the prompt. I build and install a release build and drive it with your key (you enter the key in the app yourself). First run on real arm64 hardware: Back key, engine restart after idle, signed build, keyboard. Untested: Android 8 to 12, share-in, the save-as export picker, low-memory kills.
2. **Your own full test**, including a read of the Greek texts (`app/i18n.js`, `engine/internal/safety/texts.go`, the crisis message above all) by a native reader, and one click on the new key-help buttons in the desktop window (the click itself was only tested on the Android emulator).
3. **Android signing key.** `mobile/scripts/make-release-key.sh` creates it outside the repo (`SOCRATES_KEYSTORE`, `SOCRATES_KEYSTORE_PASSWORD`). Back it up: a lost key means the app can never be updated.
4. **Play console** (details and "check" marks in `docs/PLAY_LISTING.md`): privacy policy URL (the English `docs/PRIVACY.md` needs hosting somewhere), website (optional), phone screenshots, content rating, data safety and AI-content declarations, a reviewer key with a small spend limit, and whether a new personal account must run a closed test first (believed yes, not verified). The Greek listing and Greek privacy page were dropped: Greek is functionality only.
5. **Desktop route.** Linux builds and runs. Windows is only cross-compiled (`make -C desktop windows`; needs WebView2, not run); macOS needs a Mac (not attempted). Nothing is signed or packaged. Choose the route (GitHub releases, itch.io, Steam); an unsigned Windows build shows a SmartScreen warning.

## Mine, once the above is ready
- Cut the tags (`v*` desktop, `mobile-v*` Android) and write release notes.
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
- The emulator and browser test scripts live in the session scratchpad, not in the repository.
