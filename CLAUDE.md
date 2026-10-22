# Claude Code guide: Socrates (Socrates-ai)

Read first: `docs/PLAN.md` (architecture, layers, the game, decisions, open questions), then `docs/SESSION_2026-10-22.md`. Both are imported here:

@docs/PLAN.md
@docs/SESSION_2026-10-22.md

## State (2026-10-22)
Planning only. No code yet. The repository was emptied of the old Python project (still in git history). Next: the founder answers the open
questions in PLAN.md section 14; then M0 (fork of Moly, strip contacts). The base code is Moly at `~/vs_projects/Moly/Moly` (Go backend,
TypeScript extension, Capacitor Android, installers). Proof of concept: `~/vs_projects/Socrates` (question logic is in the `socratic-agents` package).

## Rules
- Do not build ahead of decisions. Answer the question asked and wait for a go.
- Commit and push only when asked. Never binaries, logs, databases or keys.
- Moly's product rules apply (one question per reply, safety first, model judgement not keyword lists, per-user keys, privacy, no page scanning).
- Safety scenarios and transcripts are local only and git-ignored.
- Package `io.github.nireus79.socrates`, author Hermes Software, English only at first.
