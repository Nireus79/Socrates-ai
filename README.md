# Socrates

A conversation with artificial Greek philosophers (Socrates, Diogenes, Plato, Aristotle), each revived in today's world and speaking as himself. They ask hard
questions, do not give verdicts, remember what you told them (in your own words, which you can see and delete), and offer starting points when you are stuck.
It runs on your own AI provider key (Claude or OpenAI). No server of ours, no ads in the chat, nothing you write in a log,
conversations encrypted on your device.

Status: early build. The engine, the screens, the desktop window (Linux first) and the Android shell exist; see [docs/BUILD_PLAN.md](docs/BUILD_PLAN.md) for what is done and what is open,
and [docs/PLAN.md](docs/PLAN.md) for the product.

```
engine/    the Go engine (keys, encrypted store, safety stage, the philosophers' turn)     make -C engine check
app/       the screens (plain JavaScript, no build step)                                   cd app && npm test
desktop/   the Wails window around engine and screens                                      make -C desktop build
mobile/    the Android app (Capacitor)                                                     see mobile/README.md
docs/      plan, character sheets, threat model, safety design, session records
```

The earlier Python project that lived in this repository (a Socratic assistant for software projects, version 0.1.1) was removed on
2026-10-22. It remains in the git history. The research prototype is [Nireus79/Socrates](https://github.com/Nireus79/Socrates).

License: MIT, see [LICENSE](LICENSE).
