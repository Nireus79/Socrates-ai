# Socrates

An app for talking with artificial Greek philosophers (Socrates, Diogenes, Plato, Aristotle), each revived in today's world and speaking as himself. They ask hard
questions, do not give verdicts, remember what you told them (in your own words, which you can see and delete), and offer starting points when you are stuck.
It runs on your own AI provider key (Claude or OpenAI). No server of ours, no ads in the chat, nothing you write in a log,
conversations encrypted on your device.

Status: free beta, published (0.1.0-beta.5). The engine, the screens, the desktop window (built and run on Linux; the Windows build is cross-compiled and has not been run; no macOS build) and the Android app exist; see [docs/BUILD_PLAN.md](docs/BUILD_PLAN.md) for what is done and what is open,
and [docs/PLAN.md](docs/PLAN.md) for the product.

## Get it
- Website: [nireus79.github.io/Socrates-ai](https://nireus79.github.io/Socrates-ai/) (download page and privacy policy).
- Windows, Linux and Android (APK): the [releases page](https://github.com/Nireus79/Socrates-ai/releases) or the [itch.io page](https://nireus79.itch.io/socrates).
- Obtainium (Android, updates itself from the releases): [add Socrates](obtainium://add/https://github.com/Nireus79/Socrates-ai), or paste `https://github.com/Nireus79/Socrates-ai` into Obtainium's "Add app". Turn on "Include prereleases", because the beta releases are marked as pre-releases.
- Android through F-Droid: submitted, waiting for review.

```
engine/    the Go engine (keys, encrypted store, safety stage, the philosophers' turn)     make -C engine check
app/       the screens (plain JavaScript, no build step)                                   cd app && npm test
desktop/   the Wails window around engine and screens                                      make -C desktop build
mobile/    the Android app (Capacitor)                                                     see mobile/README.md
docs/      plan, character sheets, threat model, safety design, session records
```

The earlier Python project that lived in this repository (a Socratic assistant for software projects, version 0.1.1) was removed on
2026-10-22. It remains in the git history. The research prototype is [Nireus79/Socrates](https://github.com/Nireus79/Socrates).

Development: written by an independent programmer in Greece, working with AI coding assistants (Claude Code). I design the product, review the code and test it.

Contact: Hermes_creative@proton.me. All products: [nireus79.github.io](https://nireus79.github.io/).

License: MIT, see [LICENSE](LICENSE).
