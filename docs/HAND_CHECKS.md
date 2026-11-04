# Hand checks before and after publishing (written for beta.4, kept for each release)

Things only a person with the right machine, phone or key can do. The Windows build was checked by the founder on 2026-11-04; nothing was run on a Mac (dropped), and the F-Droid build was only installed as a test copy signed with a throwaway key. Use your own API key only for the checks that say so; set a spending limit at the provider first.

## Windows (checked by the founder, 2026-11-04)
1. Download `socrates-v0.1.0-beta.4-windows-amd64.zip` from the release and check it with the `.sha256` file. Unpack it into one folder (keep `socrates-engine.exe` next to `socrates-desktop.exe`).
2. Start `socrates-desktop.exe`. Expect "Windows protected your PC": More info, Run anyway. If the window stays blank, install the WebView2 runtime (Windows 10).
3. Register a profile (a name; choose a passphrase or none). Expect to land on the provider screen. Log out and log in again; a wrong passphrase must be refused.
4. Provider screen: the "How to get a key" buttons must open your browser at the provider's page. Enter your key; it must be tried before it is kept.
5. Talk with each philosopher: send a message, press Help me answer, press Stop while a reply is awaited (your words must come back to the box).
6. Settings: Appearance (Automatic, Light, Dark), Language (Greek), About and help (About, User manual), Delete everything.
7. Check `C:\Users\<you>\.socrates\socrates.log` exists and holds no message text and no key.
8. Remove: close the window, delete the folder, then `.socrates` if you want your data gone.

## Linux
Unpack `socrates-v0.1.0-beta.4-linux-amd64.tar.gz`, run `socrates-desktop`. Needs `libgtk-3-0` and `libwebkit2gtk-4.1-0`. Same steps 3 to 7 as above (the log is `~/.socrates/socrates.log`).

## Android (install from F-Droid when listed, or the signed APK)
1. Install over the old build or fresh. Check the signature of the APK against the key's fingerprint (`apksigner verify --print-certs`; SHA-256 starts `5f4294`, ends `d9c4`).
2. First start: age confirmation, provider and key, then the four philosophers.
3. A conversation with each philosopher; Help me answer; Put a question to me; the Progress screen; Memory (read and delete a note).
4. Press Back until the app closes, then check in Settings, Apps that Socrates is not running. Open it, switch to another app for four minutes, check again (the engine stops by itself after about three).
5. Settings, Report a problem: Prepare report, read it (no message text, no key), Send opens the mail app.
6. Dark and Light theme, Greek, the sponsor card (Hide works).
7. Airplane mode: a message must say it could not reach the provider and come back to the box.

## Your own full test (needs your key)
One real conversation of ten messages with each philosopher, a crisis-sounding message (the fixed text and check-in must appear and the words must not be kept), a harmful request (refusal), a hard historical question (Aristotle on slavery: honest answer with context, then a question about today). Read the Greek texts of `app/i18n.js` and the crisis message (`engine/internal/safety/texts.go`) as a native reader.

## After publishing
- Settings, Pages, Source = GitHub Actions (once). Then open `https://nireus79.github.io/Socrates-ai/` and `.../privacy.html`.
- Publish the draft release (it is a pre-release). Check that the notes, the five files and the `.sha256` files are there.
- Put the tag's full commit hash into `mobile/fdroid/io.github.nireus79.socrates.yml` (`git rev-parse 'v0.1.0-beta.4^{commit}'`), then open the merge request on F-Droid's GitLab from your account (see `mobile/fdroid/README.md`).
- Watch the issues and the contact address for the first reports.

## Not done by anyone
macOS build; Google Play (waits for a closed test); a Windows run; signing certificates for Windows; the daily limits and billing (after the beta).
