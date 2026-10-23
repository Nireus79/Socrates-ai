# Build plan

Status: decided by Claude on the founder's delegation, 2026-10-23 ("Moly as an example, totally redesign if better; quality and safety over time and cost"). Supersedes PLAN 4.9 (reuse table) and PLAN 8 (milestones). See the Status section below for what is built.
Working language of the project from now on: English.

## 1. The decision: rebuild, with Moly as a parts bin
Moly is neither copied whole nor ignored.
- **Rebuilt fresh and clean:** the conversation core, the HTTP surface, the storage schema, the philosopher registry, entitlements, the UI shell. Reason: Moly's `agents/` (135 files, 12,700 lines), its 6,500-line `main.go` and its 9,700 lines at the top level serve a different product (contacts, goal lock, gap maturity, an 11-layer orchestrator, native messaging).
  Pruning them risks dead code and hidden coupling, and in a product where safety comes first we want every line of the safety path to be understood and audited. The new flow is small enough to write cleanly.
- **Ported with their tests, then reviewed line by line:** the small, self-contained, safety-, privacy- and key-critical parts that Moly's beta and the F-Droid review have already exercised. Rewriting these would reintroduce bugs that were already found.

| Port from Moly (with tests, renamed, reviewed) | Size | Notes |
|---|---|---|
| `keystore/`, `secrets/` (sealing, process hardening) | ~600 lines | self-contained; the database key in the system keychain, a passphrase, or an explicit key file |
| `privacy/` (log scrubbing), `safety/` | ~160 lines | no dependencies |
| `security.go` (loopback only, host check against DNS rebinding, origin policy, per-run token), `ratelimit.go`, `lifecycle.go`, `stop.go`, `keygate.go` | ~640 lines | the origin policy is adapted to the desktop shell and the phone shell (requests without an Origin header, token added by the shell) |
| LLM layer: `tools/llm_client.go`, `llm_models.go`, `llm_swap.go`, `model_roles.go`, `approved_models.json`, the mock client; `llm_settings.go` | ~2,000 lines | providers Claude, OpenAI, Ollama; roles resolved to models; per-user keys; to be separated from the layer code around them |
| Safety stage: `agents/safety_stage.go` and `tools/constitutional_evaluator.go` with `config/constitution.yaml` | ~800 lines | read and audited first, then ported or rewritten; crisis refusal, the fixed check-in on the next turn, "an unclear message is never a crisis" |
| the user export, import and delete semantics, one transaction per turn (`turn_commit.go`) | ~300 lines | the schema is new; **the SQLCipher driver is not carried over** (unmaintained since 2020): the store is pure-Go SQLite with field-level AES-256-GCM, see THREAT_MODEL.md |
| Title and summary writers | ~500 lines | rewritten over the new store if the old ones are too coupled |
| UI pieces: `KeystoreGate`, the settings for key and model, the chat primitives, the phone `shim.js` idea | | the screens are re-laid out for a game, not a sidebar |

**Not carried:** contacts, people memory, identity, relations, conflict handling, goal lock, gap maturity, `DecideReply` and its exits, the 11-layer orchestrator, the native host, installers, the browser extension (optional and later).

## 2. Names (final)
Android package `io.github.nireus79.socrates`; Go module `socrates`; folders `engine/` (Go), `app/` (screens), `desktop/` (Wails shell), later `mobile/`; data folder `~/.socrates`; keychain service `socrates`; environment prefix `SOCRATES_`; token header `X-Socrates-Token`; author Hermes Software; licence MIT.

## 3. The new engine
```
engine/
  cmd/socrates-engine/        small main: wiring only
  internal/transport/         HTTP handlers, security policy, rate limit, lifecycle, key gate      [ported, adapted]
  internal/keystore/ secrets/ privacy/                                                          [ported]
  internal/store/             encrypted SQLite, migrations: conversations, messages, summaries, settings, user keys, entitlements, later the ledger  [new]
  internal/llm/               providers, role to model, per-user key, mock                           [ported, separated]
  internal/safety/            stage on every message and on every reply, fails closed               [ported after audit]
  internal/philosophers/      data-driven registry: id, name, era, persona, unlock data             [new]
  internal/conversation/      the turn: safety -> philosopher reply -> safety on the reply -> commit [new]
  internal/entitlements/      what a user may do; grants everything in the free beta               [new]
  internal/userdata/          export, import, delete the whole profile                              [ported semantics]
```
The turn is one reply call to the model (persona, conversation, running summary) between two safety checks; no rules engine (PLAN 18, principle 9). Replies are free in length for now (founder).

## 4. Milestones (replace PLAN 8)
- **M0 Foundation.** Repository layout; the ported modules with their tests green; the new store and migrations; the LLM layer; the transport skeleton (status, key-store routes, local auth). The engine starts and answers; nothing converses yet. A written threat model and a privacy test that fails if user text reaches a log.
- **M1 The philosopher turn.** Persona registry, the turn service with the safety stage on both sides, title and summary, scenario tests with the mock model and a real temporary database, live checks with real models; a first UI and the Wails window. **First usable alpha: Socrates, end to end.**
- **M2 The other three philosophers and entitlements.** Diogenes, Plato, Aristotle as data; unlock and the entitlement layer (all granted in the beta); first live reviews by the founder.
- **M3 Memory and help.** The ledger of claims per philosopher (kept simple), "Help me answer" suggestions, the score from the increase of maturity (PLAN 16) if still wanted.
- **M4 Safety and quality.** The philosophy safety battery (hard topics versus harm, the crisis route, abuse of the Diogenes character, minors' limit), the factual-fidelity battery (no invented episodes), three-model live runs; an independent review of the safety path with the code-review skill.
- **M5 Platforms.** Android (engine inside the app, Play Billing and the entitlement layer), desktop packaging and signing, store material; Steam or direct sale decided then. The browser extension stays optional.
- **M6 Release.** Public beta, then listings.

### Status (2026-10-23, late)
- **M0 Foundation: done.** Secrets, keystore, privacy, transport door, encrypted store, LLM layer, settings API, the program, safety stage, profile export/import/delete. `make check` green.
- **M1 The philosopher turn: done.** Philosopher registry and persona (a `Manner` per person), the turn (safety, reply, output gate, one transaction, title and summary), scenario tests on the mock, the screens, the Wails window, whole-journey tests against the real binary, simulated browser runs. Live checks were run twice with the founder's key; from now on all tests are simulated.
- **M2 Other philosophers and entitlements: done** as far as decided: all four exist as data and are unlocked in the beta; the entitlement layer exists. The unlock rules and numbers are undecided.
- **M3 Memory and help: done.** The ledger (claims, terms, tensions; quote-validated; per philosopher) and "Help me answer". The maturity score is not built (PLAN 16 unconfirmed).
- **M4 Safety and quality: not started** beyond unit scenarios and one live smoke check. The batteries need a live run (founder's decision on cost) and an independent review of the safety path.
- **M5 Platforms: in progress.** Engine compiles for Windows, macOS, Linux arm64, Android; the Android shell is written (`mobile/`); the first APK build is the next check. Not done: Play Billing, signing, store material, Windows/macOS runs and packaging.
- **M6 Release: not started.**

## 5. How we work
- Safety and privacy tests come first and gate every merge: `go vet ./... && go test -count=1 ./...` and the UI tests must be green.
- Each ported module arrives with its original tests, plus new tests for what changed; each is read in full before it is trusted.
- One commit per step; pushed when the founder asks. Documents stay in English; the repository holds no binaries, logs, databases or keys (the three logo files are waiting for the founder's word).
- Safety scenarios and transcripts stay local and git-ignored.

## 6. Risks
- The safety evaluator is the largest ported piece (730 lines) and carries Moly's assumptions; it is audited before it is trusted, and the philosophy battery comes before any live use with other people.
- Without grounding texts a philosopher can still misremember a detail; measured by the fidelity battery (PLAN 18).
- Time: more sessions than modifying Moly would take; accepted by the founder in favour of quality and safety.
