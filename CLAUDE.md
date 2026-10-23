# Claude Code guide: Socrates (Socrates-ai)

Read first: `docs/PLAN.md` (architecture, layers, the game, decisions, open questions; sections 15 to 18 are the newest), `docs/BUILD_PLAN.md`, then the session records of 2026-10-22 and 2026-10-23. They are imported here:

@docs/PLAN.md
@docs/SESSION_2026-10-22.md
@docs/SESSION_2026-10-23.md
@docs/CHARACTER_SOCRATES.md
@docs/CHARACTER_DIOGENES.md
@docs/BUILD_PLAN.md
@docs/CHARACTERS_COMMON.md
@docs/CHARACTER_PLATO.md
@docs/CHARACTER_ARISTOTLE.md

## State (2026-10-23, late)
M0 and most of M1 to M3 are built, committed and pushed (see `docs/BUILD_PLAN.md` "Status" and the session record, sections 31 to 46). The founder speaks English from now on (Greek only if asked); documents and code are English.
What exists:
- `engine/` (Go, module `socrates`, `make check` is the gate): transport door, encrypted store (field-level AES-GCM), keystore, LLM layer (Claude, OpenAI, Ollama), safety stage (judge, pure decision, output gate, fixed texts), the turn (`internal/conversation`), philosophers as data, suggestions ("Help me answer"), the ledger (claims, terms, tensions, per philosopher, quote-validated), entitlements (beta grants everything), profile export/import/delete, idle report.
- `app/` the screens (plain JS, no build step; `npm test` = jsdom tests). `desktop/` the Wails v2 window (own Go module; `make -C desktop build`). `mobile/` the Android shell (Capacitor; see its README).
- No corpus and no retrieval (PLAN 18). Replies are brief by a `Manner` sentence per philosopher.
The maturity score is built (PLAN 16: inquiries, challenges, five with each earlier philosopher to unlock the next; production code in `internal/progress`, bypassed in the beta unless `SOCRATES_ENFORCE_UNLOCKS=1`).
Open: tuning of the thresholds on real conversations, daily numbers for turns and suggestions, Play Billing, store listing and signing, the live quality batteries (M4), Windows/macOS runs, the logo files (untracked, waiting for the founder).
**Testing rule (founder, 2026-10-23): all tests are simulated** (scripted mock model, stand-in model servers, jsdom, headless browser). Never use the founder's API key or any paid provider without being asked; the founder does the final manual test. Do not ask permission for every task: proceed through the build plan, and ask only for real product decisions.

## Rules
- Do not build ahead of decisions. Answer the question asked and wait for a go.
- Commit and push only when asked. Never binaries, logs, databases or keys.
- Moly's product rules apply (one question per reply, safety first, model judgement not keyword lists, per-user keys, privacy, no page scanning).
- Safety scenarios and transcripts are local only and git-ignored.
- Package `io.github.nireus79.socrates`, author Hermes Software, English only at first.
