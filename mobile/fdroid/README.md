# F-Droid recipe for Socrates

`io.github.nireus79.socrates.yml` goes to F-Droid's `fdroiddata` repository as `metadata/io.github.nireus79.socrates.yml`
(a merge request on gitlab.com/fdroid/fdroiddata from the author's fork). F-Droid's own pipeline then builds the app from the tag.
Modelled on Moly's recipe, which passed that pipeline; the lessons below are from Moly's merge request (Moly docs/NEXT_VERSION.md).

Before the merge request:
1. Push the tag (for example `v0.1.0-beta.3`; the version in `mobile/android/app/build.gradle` must match, see `scripts/check-version.sh`).
2. Replace `FULL_COMMIT_HASH_OF_THE_TAG` with `git rev-parse 'v0.1.0-beta.3^{commit}'`. The tag is annotated, so the tag name or the tag object's hash fails; it must be the commit.
3. Copy the file from a text editor, never from the terminal view (the terminal adds a margin and wraps long lines, which broke Moly's first upload).

Rules that F-Droid's checks enforce:
- No comments in the yml (`fdroid rewritemeta` moves things around and wraps the anti-feature text).
- `AutoUpdateMode: Version` (plain) when `UpdateCheckMode` is `Tags`.
- The `sudo:` block must not install Java: the build image already has JDK 21. Go 1.26, Node and npm come from Debian forky.
- `fdroid lint` and `fdroid rewritemeta` need F-Droid's `config/` folder next to the file, otherwise they report wrong errors.
- Reproducible builds are off (as for Moly): F-Droid signs with its own key, so the GitHub APK and F-Droid's cannot update each other. This cannot be switched on later for the same app.

Not yet run: this recipe has never been built by F-Droid. Listing text and images are in `fastlane/metadata/android/en-US/` at the root of the repository (phone screenshots are missing).
