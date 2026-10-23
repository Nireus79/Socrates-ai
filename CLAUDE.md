# Claude Code guide: Socrates (Socrates-ai)

Read first: `docs/PLAN.md` (architecture, layers, the game, decisions, open questions; sections 15 to 17 are the newest), then the session records of 2026-10-22 and 2026-10-23. Both are imported here:

@docs/PLAN.md
@docs/SESSION_2026-10-22.md
@docs/SESSION_2026-10-23.md
@docs/CHARACTER_SOCRATES.md
@docs/CHARACTER_DIOGENES.md
@docs/CHARACTERS_COMMON.md
@docs/CHARACTER_PLATO.md
@docs/CHARACTER_ARISTOTLE.md

## State (2026-10-23)
Planning only. No code yet. The founder speaks Greek in the conversation; reply in Greek, write documents and code in English. The repository was emptied of the old Python project (still in git history). Character sheets: Socrates approved, Diogenes agreed (age limit open), Plato and Aristotle drafted; common rules in CHARACTERS_COMMON.md (philosophers revived in today's world, know what came after, bridge to today). Desktop is a standalone app (engine + screens in a window, no extension, no installers; experiment pending). **No corpus and no retrieval (PLAN 18): the philosophers speak from the model's own knowledge; no invented memories.** One conversation, beta free. Next: the experiment, then M0 (fork of Moly, strip contacts). The base code is Moly at `~/vs_projects/Moly/Moly` (Go backend,
TypeScript extension, Capacitor Android, installers). Proof of concept: `~/vs_projects/Socrates` (question logic is in the `socratic-agents` package).

## Rules
- Do not build ahead of decisions. Answer the question asked and wait for a go.
- Commit and push only when asked. Never binaries, logs, databases or keys.
- Moly's product rules apply (one question per reply, safety first, model judgement not keyword lists, per-user keys, privacy, no page scanning).
- Safety scenarios and transcripts are local only and git-ignored.
- Package `io.github.nireus79.socrates`, author Hermes Software, English only at first.
