# Windows package routes (prepared 2026-11-04, nothing submitted)

State: the Windows build (`socrates-vX-windows-amd64.zip`: `socrates-desktop.exe`, `socrates-engine.exe`, LICENSE, README.txt) is unsigned. The founder said on 2026-11-04 to consider it tested; no run is recorded in the repository. The winget point below (the window finding the engine through a link) is still worth one check on Windows.
The window needs the WebView2 runtime (part of Windows 11; Windows 10 may lack it).

## winget (`packaging/winget/`)
Three files for the beta.5 zip (`Nireus79.Socrates`): version, installer, default locale. Submitted as a pull request to github.com/microsoft/winget-pkgs under
`manifests/n/Nireus79/Socrates/0.1.0-beta.5/`. Needs a GitHub account (the author's) and signing the CLA the bot asks for.
- Check before submitting: `winget validate --manifest <folder>` and `winget install --manifest <folder>` on Windows (needs "Enable local manifest files" in `winget settings`). Tool: `wingetcreate` can also build and submit the pull request (`wingetcreate update Nireus79.Socrates --version X --urls <zip url> --submit`) for later versions.
- Installer type is a zip with a nested portable: winget unpacks the whole zip into its package folder, so the engine sits beside the window, and makes a command `socrates` for the window. **Not verified:** the window finds the engine from its own path (`desktop/main.go` `enginePath`); if winget starts it through a link the path may be the link's folder. Test the installed command before submitting. If it fails, the fix is in `enginePath` (resolve links).
- The zip's SHA256 is in the manifest in capitals; each new version needs a new URL, hash and version folder. Pre-release version names such as `0.1.0-beta.5` are accepted but sort oddly against later `0.1.0`.
- Review: the pull request is scanned and installed by Microsoft's pipeline; an unsigned app may draw a SmartScreen or antivirus flag. Expect a few days.

## Scoop (`packaging/scoop/socrates.json`)
Published 2026-11-04: https://github.com/Nireus79/scoop-socrates (the file lives in `bucket/socrates.json`; it was not installed through Scoop on a real Windows machine). Update it by hand per release.
Not for the main bucket (it wants stable, popular apps). Own bucket: create a repository `scoop-socrates` with the file in `bucket/socrates.json`; users run
`scoop bucket add socrates https://github.com/Nireus79/scoop-socrates` then `scoop install socrates`. Scoop starts the real file (not a link), so the engine is found. Update by hand per release (version, url, hash).

## Microsoft Store (not prepared as files)
Steps: register as an individual developer at partner.microsoft.com (free for individuals as of the last report; verify the current fee and identity check), reserve the name, wrap the two files in an MSIX package (the Store signs it, so there is no SmartScreen warning),
declare the WebView2 dependency, fill the age rating (16+), the AI-content disclosure and the privacy policy address (https://nireus79.github.io/Socrates-ai/privacy.html), then pass certification. More work than the other two and a review that can ask for changes; do it after a real Windows run.
