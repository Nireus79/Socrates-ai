# Socrates: a philosopher you can ask anything

Status: plan, 2026-10-22, revised the same day after the founder's answers (sections 10 to 12) and again on 2026-10-23 (sections 15 to 18). The engine, screens, desktop window and Android shell are being built (BUILD_PLAN.md).
Built so far: see BUILD_PLAN.md "Status".
Where section 12 (the game) and an earlier section disagree, section 12 wins; where sections 15 to 17 disagree with any earlier section, they win.
Sections 4.4, 4.8 and 6 are marked where they change. Late on 2026-10-23 the corpus and the retrieval of texts were shelved: section 18 wins over section 13 and over the corpus parts of sections 15 to 17. Sections 5, 9, 10, 12 and 14 were corrected on 2026-10-23 to agree with sections 15 to 17.

## 1. What it is

A chat with an artificial Socrates (and, later, with other Greek philosophers, each an artificial character revived in today's world; see sections 12 and 15 to 17). The user asks anything: a question of ethics, of daily life, of science, of meaning. Socrates does not hand
down a verdict. He examines the question with the user by the Socratic method (clarify the terms, find the assumptions, test them with cases,
show a contradiction, reach "I do not know yet", then a better question). When the user cannot answer, Socrates offers *suggestions*: several
different starting points the user can take, edit or reject.

It is built from the code of Moly (same product family): a **standalone app** for Windows, Linux and macOS (decided 2026-10-23, section 15: engine and screens in one window, no extension and no installers) and an Android app (no local model on Android);
a browser extension for Chrome, Edge and Firefox stays possible later as an optional free entry. Like Moly it runs on the user's own AI provider key (Claude or OpenAI) (local models were dropped on 2026-10-24, section 19). No page scanning, no ads in the chat, encrypted local storage, no user content in logs.

## 2. What the two sources teach

### 2.1 Moly (the base)
A Go backend (`moly-go/`) with a React sidebar (`moly-extension/`), a Capacitor Android shell with the Go engine embedded (`moly-mobile/`), and
native-messaging installers (`installer/`). One message goes through: safety stage, one memory update (who and what the user said, with quoted
evidence), a pure reply decision (`DecideReply`), one question per reply, an output gate, and one database transaction per turn. It has a skip
button, pending questions, per-user keys, privacy scrubbing, and a scenario test suite with a scripted model plus labelled judge batteries.

### 2.2 The proof of concept (github.com/Nireus79/Socrates)
A code-project assistant. How its Socratic questions are made (the logic lives in the `socratic-agents` package, `socratic_counselor.py`):
- **Question generation.** `_generate_question` returns an unanswered pending question if one exists (never two at once). Otherwise
  `_build_question_prompt` assembles: project facts, the current *phase* (discovery, analysis, design, implementation) with its focus words,
  a context summary, the last four messages, retrieved knowledge, and the **last five questions asked "DO NOT REPEAT"**. The model is asked for
  ONE question that builds on the discussion, pushes thinking deeper, fits the phase, encourages critical thinking and differs from earlier ones.
  A static list per phase is the fallback when the model fails.
- **Suggestions.** `_generate_answer_suggestions` takes the current question plus the project and asks for 3 to 5 *diverse* complete-sentence
  starting points, each from a different angle (method, perspective, scope, depth), numbered, parsed line by line; a phase-based fallback list if the
  call fails. The CLI offers it when the user types `help`, `suggestions` or `hint`, then asks the same question again.
- **Two modes.** Socratic (system asks, user answers) and Direct (user asks, system answers), switched by command.
- **Also present, not needed here.** Workflow nodes and category-targeted question selection, roles per team member, vector-database documents,
  code generation, collaboration, subscription tiers, 37 modules of governance and monitoring.

What we take: the single pending question, the do-not-repeat list, the phase idea (reshaped into inquiry stages), the suggestions design, the static
fallback, and the two modes (reshaped, see 4.4). What we leave: everything project-, code-, team- and workflow-shaped.

## 3. Principles

1. One question per reply, optionally one short sentence first. No lecture.
2. Socrates examines the user's own words. Every claim he tests is quoted evidence from the dialogue, never invented.
3. He may say "I do not know". Ending in aporia (a better question than we began with) is a valid result, not a failure.
4. The user is never trapped: a stuck user gets suggestions or Socrates' own view on request (section 5).
5. Model judgement, not keyword lists. Unreadable model output fails safe. Prompts name no person and no scenario.
6. Safety first on every message; harm is refused, difficult philosophy (death, ethics, evil) is not harm.
7. Privacy as in Moly: nothing the user writes in logs, secrets cut out before storage, deleting the profile leaves nothing.
8. The user's key belongs to the user. No key in the environment, the extension storage, a log or an API answer.
9. The philosophers are dynamic, not scripted (decided 2026-10-23): for every reply the model reads the user's words, searches the sources and decides what to say; there are no fixed answers, no step-by-step rule lists and no examples in prompts.
   The character files describe the person; the hard limits are safety, honesty about being artificial, and no invented words for the user.
   No stereotypes (decided 2026-10-23): each philosopher is a person, with his own ways and his mistakes, formed by what he wrote and what was written by those who knew him, not by a scholar's analysis and not by a description of ours;
   the corpus holds primary works and direct testimony only, and editors' introductions, summaries and notes are kept out of what he reads. We do not bring the philosophers to our measure.

## 4. Architecture and layers

```
Client shells            extension (Chrome, Edge, Firefox) | desktop host (Win, Linux, macOS) | Android app
        |  loopback HTTP, token header, extension origins only (production mode as in Moly)
Transport                handler, auth, rate limit, key gate (423 while the database key is missing)
        |
Turn pipeline            0 Safety -> 1 Dialogue memory -> 2 Inquiry state -> 3 Move decision -> 4 Reply -> 5 Output gate -> 6 Commit
        |
Support                  Suggestion service | Title writer | Summary writer | Preferences | Provider/model resolver
        |
Storage                  encrypted SQLite (conversations, messages, memory ledger, ops, settings, user keys)
```

### 4.1 Layer 0: safety stage (kept from Moly, simplified)
`safety_stage.go`, `constitutional_evaluator.go`. Runs on every message, fails closed. Clear self-harm or illegal request: short refusal with no
help lines; an unclear message is never a crisis, ask first. The turn after a crisis refusal starts with the fixed check-in. New for Socrates: a
judge label separates *discussing* a hard topic (allowed, even encouraged) from *asking for help to do harm*. Needs a labelled battery of
philosophy-specific cases (euthanasia, war, suicide as a theme, violence in history) before launch.

### 4.2 Layer 1: dialogue memory, "the ledger" (replaces people, roles, relations, contacts)
Reuses the machinery of `memory_update.go` and `memory_store.go`: one model call per message sees the whole ledger, the last three messages and
the new message, and answers with operations, each with quoted evidence; the store rejects what breaks a rule and records every operation for undo.
Entities replace contacts:

| Entity | Meaning | Operations |
|---|---|---|
| **claim** | something the user asserted ("lying is always wrong") | `add_claim`, `retract_claim`, `qualify_claim` |
| **term** | a word the dialogue depends on, with the user's definition if given | `add_term`, `define_term`, `redefine_term` |
| **assumption** | a hidden premise a claim rests on, surfaced by Socrates and accepted or denied by the user | `add_assumption`, `accept_assumption`, `deny_assumption` |
| **tension** | two ledger entries that cannot both hold | `mark_tension`, `resolve_tension` |
| **question** | open, answered, declined, skipped (pending) questions, with topic and what the answer changes | `close_question`, as in Moly |
| **fact** | something personal the user told (kept small; for tone, not for profiling) | `add_fact` |

Removed: `add_person`, `name_person`, `merge_person`, `set_role`, `correct_role`, `add_trait`, the person-name questions, `memory_mentions`,
`ProjectToContacts`, contact deduplication, the sidebar contacts panel. The summary (`ConversationSummaryManager`) stays: Moly's memory in plain
prose, rewritten after each stored turn, feeding every call through the dialogue packet.

### 4.3 Layer 2: inquiry state (replaces goal lock and gap maturity)
The *inquiry* is the question under examination, quoted from the user (the old goal rule: it must quote the user). A new topic is judged by the
existing goal-relation judge (`GoalRelation`): same inquiry, a refinement, or a different one; a different one is followed without a question,
the old one is kept in the conversation record. The **stage** replaces maturity and is chosen by the model from the ledger, never by a counter:

1. **Wonder**: the question is received and the terms are unclear. Move family: clarify.
2. **Definition**: key terms get working definitions. Move family: ask for the definition, ask for a case.
3. **Examination**: claims and assumptions are tested with cases and counterexamples. Move family: probe, counterexample, analogy.
4. **Aporia or insight**: a tension is exposed or a definition survives. Move family: summarize, restate, offer the next question.

A stage can move back (a new term appears). This mirrors the proof of concept's phases, but about the idea, not the project.

### 4.4 Layer 3: move decision (replaces `DecideReply`)
A pure function of the facts, as in `reply_policy.go` and `reply_exits.go`, testable without a model. Order of exits:
refusal, safety check-in, greeting, **direct answer** (see below), revision of Socrates' last statement if the user commented on it, stuck user
(suggestions offered), skipped-question return, then by stage: *clarify term*, *ask definition*, *ask for a case*, *probe assumption*,
*counterexample*, *analogy*, *expose tension*, *summarize and restate* (aporia). Exactly one question per reply (`single_question.go`).

Guards stay: a question is asked only if its answer changes the reply and nothing in the ledger, the last messages or the user's words already
holds it (`GuardNext`); a topic that is open, answered or declined is not asked again; a pushback closes the topic; the do-not-repeat list from the
proof of concept comes from the ledger's question list.

**Direct answer** (the Socrates "Direct" mode; *superseded by section 12: every philosopher has his own method and both asks and answers*): the product promise is "ask anything", and a pure question-machine frustrates people. A *factual*
question (a date, a definition in the dictionary sense, who said what) is answered in a sentence, then Socrates asks what the user makes of it.
Whether the question is factual or philosophical is the model's judgement (a small judge with defined labels and a battery). A visible
setting lets the user choose *Strict* (always questions), *Balanced* (default) or *Open* (answers first, then one question). Decision pending.

### 4.5 Layer 4: reply generation
One model call at reply role with the persona, the dialogue packet, the chosen move, the quoted evidence the move is about, and the user's
preferences (`preferences.go`: style and tone instructions; "Humorous" and so on). The persona is written in words, no names of scenarios, no
examples that are also test cases. Reply role asks for adaptive thinking (Claude 5 takes no temperature; effort is the lever).

### 4.6 Layer 5: output gate and one-question rule
A flagged draft is replaced by one question (`blocked_reply.go`). Draft with two questions is trimmed. A draft that is a lecture, a verdict
without the user's own reasoning (in Strict or Balanced), or that states an invented quotation is flagged. A second safety evaluation of the
reply stays.

### 4.7 Layer 6: turn commit
`turn_commit.go`: one transaction per turn; a failed turn keeps nothing (`rollbackUnlessCommitted`). A refused message saves no claim, no summary
line and no inquiry.

### 4.8 The suggestion service (from the proof of concept)
Triggers, in order of certainty: (a) the user presses **Help me answer** on a Socrates question (the skip button's sibling; always available on
a question); (b) the user writes "I don't know", "no idea", "I can't say" and the stuck judge confirms; (c) the user gives two empty or evasive
answers in a row (judge, labelled, never a counter alone).

Behaviour: the model receives the question, the inquiry, the stage, the ledger entries the question is about, and is told to produce 3 to 5
*diverse* starting points, each a complete first-person sentence the user could adopt, each from a different angle. The angles are defined in the
prompt in words and are philosophical, not software ones: a definition, a counterexample from ordinary life, a consequence, the view of an
opposing school (named as a school, not a person quoted), a personal experience, an admission of doubt. Output is a JSON object with keyed fields
(the numbered-list parse of the proof of concept failed on models that add chatter), validated; an unreadable answer falls back to a static
angle-based list per stage. A suggestion the user picks becomes their message, editable first, and is recorded as *suggested by Socrates* so the
ledger does not treat it as the user's conviction without confirmation (doubt means a one-question confirm, as in Moly's fact confidence).
The user can ask again for different ones, which excludes the previous set.

If the user is still stuck after suggestions, Socrates may say what he himself thinks, flagged as one view among others, and the question is
marked skipped (pending, it returns once), exactly the Moly skip rules.

### 4.9 Reuse from Moly, file by file (SUPERSEDED on 2026-10-23 by BUILD_PLAN.md: Moly is a parts bin, the core is rebuilt)

| Keep as is or nearly | Rework | Remove |
|---|---|---|
| `security.go`, `lifecycle.go`, `ratelimit.go`, `stop.go` (`host.go`, `registerhost.go`, `detach_*` and the installers only if the optional extension comes back; the standalone app starts the engine itself) | `agents/memory_update.go`, `database/memory_store.go`, `memory_projection.go` (entities above) | `agents/contact_*`, `context_extractor.go` person parts, `identity*`, person-name questions, `database/contact_*`, `contact_relations.go`, `conflict_*` project-conflict code |
| `keystore/`, `keygate.go`, `secrets/`, `llm_settings.go`, `database/user_ai_settings.go`, `tools.ResolveRoleModels` | `agents/reply_policy.go`, `reply_exits.go` (new exits) | `behavior_analyzer.go`, trait extraction, Reflections-panel insights tied to people |
| `privacy/`, `database/encryption.go`, `db.go`, user export/import/delete | `agents/goal_stage.go`, `goal_relation.go` (inquiry) | `moly-proxy`, mail/contact UI in the sidebar |
| `conversation_title.go`, `conversation_summary*`, `turn_commit.go`, `skip_button.go` (becomes help-me-answer + skip) | `agents/preferences.go`, `safety_stage.go` (philosophy batteries) | `tools/layer_context*`, layers 3 to 11 that were project-shaped; the 11-layer orchestrator |
| Extension shell, `KeystoreGate`, installer scripts, Android shell, Play recipe | sidebar chat UI: suggestion chips, stage indicator, ledger panel | |

The ledger panel replaces the contacts panel: the user sees their claims, terms and tensions, can delete any entry, and sees which ones
Socrates suggested.

## 5. Platforms and releases (same versions as Moly)
- **Desktop (main form, decided 2026-10-23)**: a standalone app for Windows, Linux, macOS: the Go engine as a child process of the app and the Moly screens in its window, with a `shim.js` for the `chrome.*` calls, the pattern that
  the Android app of Moly already uses. No native-messaging host, no installers. A Wails v2 window with the engine as a child process, the Moly screens embedded and the engine's API proxied with the token added was built and ran on Linux (experiment of 2026-10-23, session record section 14); Windows and macOS are not yet tested. There is no local model (section 19); the provider key route is the only one.
- **Browser extension**: optional, later, as a free entry (Chrome and Edge Manifest V3 side panel, Firefox separate build). Not part of M0. The Chrome Web Store has no payments of its own.
- **Android**: Capacitor shell with the engine embedded, copy/paste layout and share target, `play` flavor only (F-Droid is out for this product, section 15; the `foss` flavor of Moly is not carried over
  unless that changes). Provider key only, no local model, no page scanning.
- Release trains: `v*` tags for desktop and extension, `mobile-v*` tags for Android, as in Moly. Store listings follow after the beta.

## 6. Language
English only at the beginning (decided 2026-10-22). Prompts are English.

## 7. Quality plan
- Scenario suite as in `orchestration_test.go`: scripted model, real temporary database, one scenario per rule, each checked to fail without the rule.
- Labelled judge batteries (`judges_live_test.go`): factual vs philosophical question, stuck vs not stuck, same inquiry vs new, claim vs question vs
  joke, hard-topic vs harm. Held-out cases are marked "held out:" and occur in no prompt. Measure each judge with 3 replays before keeping it.
- Live checks with real models (Claude and OpenAI) by hand before each release; local safety scenarios stay git-ignored.
- Reply-quality review: a fixed list of 30 questions across ethics, politics, science, love, death and everyday life; read by the founder.
- Privacy tests: user content absent from logs, deletion leaves nothing.
- Character tests (2026-10-23): a factual-fidelity battery (a philosopher does not assert an episode that did not happen or a work he did not write; checked by hand and by a judge on a fixed list of questions), the output gate on questions, claims of being real or human, and the "bridge to today" being a question; a local battery per philosopher (CHARACTERS_COMMON.md 5).

## 8. Milestones (SUPERSEDED on 2026-10-23 by BUILD_PLAN.md section 4)
1. **M0 Fork**: copy Moly into this repository as a starting point (Go module, the screens of the extension as the shared UI, mobile; installers and native host dropped), strip contacts, project-shaped layers
   and the sidebar contacts panel; build a desktop window around engine and screens; green build and tests; renamed identifiers (package `io.github.nireus79.socrates`, app and data names).
2. **M1 Ledger**: the new entities, operations, store rules, undo, projection to the ledger panel; memory battery.
3. **M2 Inquiry and moves**: inquiry stage, move decision exits, persona, output gate; scenarios; the do-not-repeat list.
4. **M3 Suggestions**: triggers, stuck judge, generation with keyed JSON, fallbacks, the picked-suggestion rule, ask-again.
5. **M4 Safety and quality**: philosophy safety battery, reply-quality review, three-model live runs.
6. **M5 Platforms**: Firefox build, desktop installers, Android flavors, store recipes; closed beta.
7. **M6 Release**: public beta tags, listings.

## 9. Risks
- A question-only bot feels evasive. Mitigation: direct-answer exit, the mode setting, suggestions, the "I do not know" ending.
- Hard topics trigger crisis refusals wrongly. Mitigation: ambiguity asks first; philosophy batteries.
- Socrates putting words in the user's mouth (quotes, suggestions stored as beliefs). Mitigation: evidence rule, suggestion provenance, confirm on doubt.
- Model drift across Claude and OpenAI. Mitigation: judges with defined labels, batteries, model approval script as in Moly.
- The copy of Moly diverges from Moly. Mitigation: the sharing model of decision 6 (section 10): start as a copy, extract shared parts later.
- (Local models: dropped, section 19.)

## 10. Decisions of the founder, 2026-10-22

| # | Question | Decision |
|---|---|---|
| 1 | Game or chat | A game: the user unlocks philosophers (section 12). *Revised 2026-10-23 (section 15): Socrates comes first, then Plato, Aristotle and Diogenes; every unlocked philosopher has an endless mode* |
| 2 | Strictness | No single setting. Each philosopher has his own method; each both asks and answers questions |
| 3 | Own view | Socrates may give his own view (labelled as one view among others) |
| 4 | Language | English only at first |
| 5 | Name | Socrates (or Socrates-ai); the founder has a logo; package `io.github.nireus79.socrates`, Hermes Software |
| 6 | Code sharing | "Whatever is better". Recommendation: **start as a copy of Moly** (M0), port shared fixes by hand during the beta, and extract the shared parts (keystore, privacy, provider and key handling, installers) into one module only after both products are stable. Revisit at M5 |
| 7 | Memory | Socrates remembers claims across conversations (the ledger belongs to the user, not the conversation; deletable by the user) |
| 8 | Hard topics | The principle of no harm; ethical reasoning as in Moly (safety stage, refusal rules, check-in after a crisis) |
| 9 | Local model | *Dropped 2026-10-24 (section 19).* Cloud only (Claude, OpenAI) |
| 10 | Business | Free launch with the sponsor card, as in Moly. Later the user can buy more time (turns) and more suggestions |
| 11 | Android timing | Undecided. Recommendation: extension and desktop first, Android after the game loop is proven (the copy/paste layout needs its own screens) |
| 12 | Knowledge | A curated set of philosophical texts, bundled and searched on the device (section 13). *Shelved on 2026-10-23 (section 18): the philosophers speak from what the models already know of the texts* |

## 11. What the decisions change
- The memory is per user, so claims outlive the conversation (4.2 changes: the ledger table gets a `user_id`, a `conversation_id` of origin, and
  a "said on" date; Socrates may say "last week you said..." only with the stored quote).
- The suggestion service (4.8) becomes a game resource with a daily budget (12.3), not an unlimited help.
- The reply decision (4.4) is chosen per philosopher: each has a *method profile* (12.2).
- The safety stage stays above the game: no philosopher, level or purchase can switch it off.

## 12. The game

### 12.1 Shape
A roster of philosophers, each a character with an era, a level of difficulty, a set of topics, a method and a persona. The user starts with
the first and unlocks the next by points. *Superseded by section 15:* the first release is Socrates alone, the roster grows with Plato, Aristotle and Diogenes, and every unlocked philosopher has an endless mode; the unlock order is open.
Original text: the last, Socrates, is the reward: endless conversation with the Socratic method and no daily limit (his own cost control is the user's key and the provider's rules, not game turns).

### 12.2 Philosophers
Each philosopher is **data plus a method profile**, not new code per character: id, name, era, level (1 to N), unlock cost in points, topics,
persona text, method profile, texts (section 13), and the thresholds of 12.4.

The method profile chooses among the dialogue moves of 4.4 and sets the balance between asking and answering. Proposed starting roster
(*replaced on 2026-10-23 by the Greek roster of section 15: Socrates, Plato, Aristotle, Diogenes; the table is kept as an example of how methods differ*):

| Level | Philosopher | Method in the dialogue | Topics |
|---|---|---|---|
| 1 | Thales (placeholder) or an easy guide | Simple "what do you see, why" questions; short answers | Nature, first principles |
| 2 | Aristotle | Definition and classification: genus, difference, examples | Ethics of character, logic, happiness |
| 3 | Descartes | Systematic doubt: remove what can be doubted, keep what remains | Knowledge, mind, certainty |
| 4 | Kant | Tests a maxim by universalising it; asks for duty vs inclination | Moral law, freedom |
| 5 | Nietzsche | Provokes, reverses values, asks who benefits from a belief | Values, power, meaning |
| 6 | Wittgenstein | Asks how a word is used; dissolves a problem by clarifying language | Language, meaning |
| final | Socrates | Pure elenchus: definitions, assumptions, contradiction, aporia | Anything the user asks |

Each one **asks and answers**: in the question phase the philosopher poses a question from his topics (a "question card"), the user answers;
in the answer phase the user may ask the philosopher something and he answers in his own manner (Socrates answers with questions, but may give his
own view when stuck, decision 3).

### 12.3 Resources
- **Turns per day** (answers the user may give to the philosopher of their level), higher levels give fewer or cost more.
- **Suggestions per day**, a separate smaller budget.
- Both reset daily (local midnight) and can be extended later by purchase (decision 10; the store route is Google Play Billing, section 15).
- Open (section 15): endless modes outside the daily turns; turns, points and unlocking only in a scored challenge mode.

### 12.4 Scoring and the loss of turns
"Accurate" cannot mean "true", because philosophy has no answer key. It must be defined as measurable qualities, judged by the model with defined
labels (a judge with a battery, like Moly's judges), against criteria that belong to the philosopher's method. Proposal, to be confirmed:
- **Responsive**: the answer addresses the question asked.
- **Reasoned**: the answer gives a reason, not only a position.
- **Consistent**: it does not contradict the user's earlier claims in the ledger.
- **In method**: it does what that philosopher asks (a definition for Aristotle, a universalised maxim for Kant, a doubt carried through for Descartes).
Each answer receives a grade (insufficient, adequate, strong) and points follow the grade and the level. At higher levels the philosopher requires
a higher grade: an answer under the bar **costs an extra turn** (the user "loses turns") and the philosopher says in one sentence what was missing,
never the model answer. A suggestion taken as an answer scores less than the same words in the user's own writing (the suggestion is help).

### 12.5 Unlocking
Points accumulate per user. A philosopher unlocks when points reach his cost and the previous one has been completed (a minimum number of strong
answers). The unlock order of the Greek roster is open (section 15). Points and progress are stored on the device, encrypted with the rest, and are deleted with the profile.

## 13. Curated texts (SHELVED on 2026-10-23, see section 18; kept as a record of what was built and learnt)
- A **bundled corpus** per philosopher of every work we may legally use (public domain, or a licence that allows it; each with source, translator and licence
  recorded in a manifest), split into passages with metadata (work, section) and indexed on the device. No web access at run time, no page scanning.
- **Grounding rule** (decided 2026-10-23, replacing the earlier two-tier rule with exact quotes): the model searches the corpus (full-text, with search terms it chooses) and adapts the answer to the passages found,
  in relation to the user's question or answer. A claim about a memory or about how the philosopher was read must rest on a passage retrieved in that turn; without one he speaks generally or says he does not recall.
  A judge compares such claims with the retrieved passages (faithfulness, not exact wording).
- The philosopher always speaks as himself and does not quote; there is no quotation check in code (decided 2026-10-23). Sources are not put in front of the user, but if the user asks for one the philosopher gives it.
  If the user feels he is not consistent, the user can tell him so.
- The register is generally modern; a phrase in an older style, as the translations have it, is allowed now and then, as a person might do.
- **Retrieval needs the model's search terms.** In the pilot (session record, section 16) the user's raw message found the right passage in 3 of 11 test cases, search terms chosen by the model in 9 of 11 (10 of 11 once synonyms were added);
  so the reply role first writes a few search terms with synonyms, then reads the passages. Embeddings stay a later option.
- **Greek originals as a second layer** (tried 2026-10-23, session record section 26): the Greek of Plato (Burnet) and Xenophon from the Perseus canonical repository, with Stephanus numbers, 4,613 passages. The model writes ancient Greek words, a crude stem with a prefix search finds the passages
  (the right one among the first six in 12 of 12 cases, first in 10 of 12). The text is public domain but the encoding of the repository is CC BY-SA 4.0 (attribution and share-alike): whether and how to ship it is a legal question for the founder.
- **Greek first** (founder's suggestion, 2026-10-23): wherever the Greek original is available it is read first and the English translation only as help. The Greek corpus now holds Plato, Xenophon and most of Aristotle (Perseus canonical-greekLit and First1KGreek, both CC BY-SA 4.0), 9,343 passages, 44 MB;
  Diogenes Laertius, Lucian and Julian have no free Greek text found yet; Plutarch's Greek and Epictetus's discourses exist in the same repositories and are not yet added.
- **Licence handling of the CC BY-SA data** (proposal, to be confirmed by a lawyer): the Greek passages are shipped as a separate data pack, labelled and offered under CC BY-SA 4.0, with a "Sources and licences" page in the app (attribution, link to the licence, a note of our changes); the application code stays our own work and only reads the pack;
  the pack is not encrypted or gated (the paid unlocks gate the characters, not the texts); the model's replies are new text and not a copy. Details in the session record, section 28.
- **Greek sources:** the Open Library (openbook.gr) marks a licence per book. Public Domain items are usable (for example Plato's Republic in Gryparis's translation of 1911, the Symposium in Kountouriotis's translation of 1912, the Crito in the ancient Greek);
  items marked Creative Commons BY-SA carry share-alike and are avoided; items marked only "free distribution" have no licence to embed and need the publisher's or translator's permission first. They matter mainly for a later Greek edition of the game.
- The corpus is now about 30 million characters (53 MB with the index; compressed far less); whether it ships whole or as a per-philosopher download is to be measured (the Android package and the extension are the limit).
- Search method undecided: plain full-text (SQLite FTS5) first; embeddings only if FTS proves poor, because embeddings need a model or a service.

## 14. Open questions after the answers (2026-10-22; status on 2026-10-23: 1, 4, 7, 8, 9 and 10 answered in section 15; 3 and 6 proposals awaiting confirmation; 2 and 5 partly open)
1. Who chooses the roster: the proposal in 12.2 or the founder's own list, and how many philosophers at launch (one, three, seven)?
2. How many turns and suggestions per day for the first level, and should they shrink or cost more at higher levels?
3. May the user ask a philosopher a question freely, or is asking also limited by turns?
4. The score: confirm the four criteria of 12.4 (responsive, reasoned, consistent, in method), or give others.
5. Purchases: through Play billing only, or also outside? (F-Droid is out for this product, which removes its anti-feature question.) And a clock-change cheat cannot be stopped on a local-only app; accept it?
6. Do points and progress sync across the user's devices, or stay on each device?
7. Does the user see points and a level bar in the chat, and is there a "profile" screen with the roster and locked characters?
8. Can the user replay a finished philosopher, and does it cost turns?
9. The philosophers speak in character. Is a modern manner (an old thinker speaking plain English) acceptable, or in period voice?
10. Does Socrates' endless mode use the same ledger, and does it show the earlier claims from the game ("you said to Kant...")?

## 15. Decisions of the founder, 2026-10-23
The conversation moves to Greek between founder and Claude; the game language stays English, Greek possibly later.

| # (section 14) | Decision |
|---|---|
| 1 Roster | Greek roster, in this order of delivery: **Socrates first**, then Plato, Aristotle, Diogenes. This replaces the Descartes/Kant/Nietzsche/Wittgenstein proposal of 12.2 |
| 2 Turns and suggestions | Not decided. Higher level = harder game. Numbers to be found by testing |
| 3 Free questions | Claude's proposal, open for confirmation: the user may ask freely; a question costs one turn like an answer and earns no points |
| 4 Score | Must be dynamic, never a fixed point per answer. See section 16 |
| 5 Purchases | F-Droid is out for this product (the target group differs from Moly's). Google Play account exists; Lemon Squeezy failed on identity checks. Recommendation: Play Billing for Android, nothing paid on extension/desktop at launch |
| 6 Sync | Not decided. Recommendation: no sync; export/import as in Moly |
| 7 Screens | The chat stays free of scores and rosters. Milestone messages only |
| 8 Replay | Any unlocked philosopher can be played at any time |
| 9 Manner of speech | Modern, as Socrates would speak today, in written text only. No period language; ancient Greek only much later as a "hardcore" mode |
| 9b Audio | Text only at launch (see below) |
| 10 Endless mode | Every unlocked philosopher has an endless mode |

Audio (decided 2026-10-23, founder's concern and Claude's reasons): the app does not speak and the user does not dictate at launch.
- Synthetic speech flattens irony, which is the main risk of the character (a literal reading); written irony shows in the shape of the question.
- Safety, the output gate and the tests work on text; tone, pauses and stress cannot be measured or tested.
- A cloud voice service would send the dialogue to a third party and break the privacy principle; local voices are weak and Android has no local model.
- Speech input would break the exact quotes of the ledger and needs the microphone and a recogniser.
- Writing a definition is part of the method and of what the score measures.
- Idea for later, not a goal: speech output made **only from the already gated text**, never a separate generation, possibly an option per philosopher.

Later the same day (founder): the common principle for **all** philosophers is "what they would say today": hard historical topics are answered honestly with context, followed at once by a *bridge to today*
(one question applying the philosopher's own criteria to the present). Every philosopher is imagined **revived in today's world**, with his memories and the knowledge of what came after, and may speak of the others.
This replaces the earlier rule that Socrates knows nothing after his death. Full rules: [CHARACTERS_COMMON.md](CHARACTERS_COMMON.md); sheets: [Plato](CHARACTER_PLATO.md), [Aristotle](CHARACTER_ARISTOTLE.md).

Later the same day (founder), business and form:
- **One conversation, no separate modes.** The philosopher may offer a challenge (a question card) which the user accepts or not; there is no split between an endless and a scored mode. Charging may apply to more conversation, more suggestions and unlocking philosophers.
  Points are earned from the increase of maturity (section 16) and need a daily limit so that unlocking cannot be farmed.
- **The beta is entirely free at every level.** The entitlement layer (what a user may do) exists from the start and grants everything; the payment provider is chosen later.
- **Unlock order:** Socrates (from the start), Diogenes, Plato, Aristotle. The order of delivery is open (Diogenes is cheapest in the corpus and strongest commercially).
- **Desktop is a standalone app**, not a sidebar. Revenue routes without an own server: Google Play Billing (Android); Steam ($100 per game, identity and tax paperwork, a disclosure for live AI content with its guardrails),
  itch.io (own cut, direct payout to PayPal or Stripe), or a merchant of record with licence keys (Polar, Lemon Squeezy, Paddle). Licence keys are checked by the provider's API or offline by a signature of ours; a purchase check on the device
  can be bypassed by a determined user, which is accepted at these amounts. All routes need an identity check; the founder's difficulty was an old-type ID card, while a driving licence was accepted by Google (a passport is the fallback).
- **Subscription instead of a key** (investigated 2026-10-23, from secondary reports, to be checked against each provider's own terms): OpenAI announced "Sign in with ChatGPT" (September 2026), an official way for a third-party app to use the user's ChatGPT plan (Plus, Pro and others) instead of an API key,
  with per-app weekly caps and the Responses API only; whether a small developer can join, and on what terms, is not yet known. Anthropic does not allow its Claude subscription login in third-party apps (the Free, Pro and Max login is only for Claude Code and Claude.ai, according to its legal page as reported in February 2026); Google offers no sanctioned way for its Gemini subscriptions either.
  So the supported route stays the user's own API key, with ChatGPT sign-in as a possible later addition for OpenAI users.
- **Risk:** the user's own key means the user already pays the AI provider, so selling "more conversation" may look like paying twice; the beta will show what users accept. Unlocks and suggestions are the safer products.

Consequences and open points:
- **Socrates first** conflicts with 12.1 (Socrates unlocked last as the reward). Resolution proposed: the first release is Socrates alone and has no unlock loop, which is the
  founder's original idea (a digital Socrates that asks, sometimes answers, sometimes answers with questions, and offers suggestions). The unlock loop starts
  when the second philosopher arrives; the order of unlocking is decided then.
- **Turns versus endless mode:** resolved above (one conversation; limits and charges apply to all of it, free in the beta).
- **Android** becomes the only revenue route (Play Billing), so its timing (open point 11 of section 10) may move earlier. Not decided. A clock change cannot be stopped
  offline; with Play Billing the purchase record is on Google's side.
- **The ledger is per user and per philosopher** (decided 2026-10-23): the conversation stays with the philosopher it was held with and is not passed to the others.
- **Minimum age:** the founder considers 16 as the lower limit (a younger person can hardly play the game anyway). To check before release: store content ratings and the digital age of consent in the countries served (to verify).

## 16. The score: dynamic maturity of an inquiry

**Decided and built on 2026-10-23 (founder: "confirmed"); where 16.2 and the paragraph below disagree, this paragraph wins.**
- **The goal is the inquiry**: the question under examination, found by the fast model in the same background pass as the ledger (no extra call) and valid only when it quotes the person's own words. Relations: `new` (the first), `deepening` (the person now asks what the inquiry rested on: equality leading to "what is freedom"), `different`; anything else leaves it unchanged. A deepening starts a **new inquiry linked to its parent**, inherits the entries it names, and the parent keeps the best score it reached (banked). A different question starts from nothing.
- **The gaps are ledger entries** of the inquiry: a term is settled when the person has defined it, a claim when they have tested it (kept it, qualified it or given it up), a tension when they have said they see it. Maturity = weighted settled over considered (Moly's rule: nothing settled is 0); it can fall when a new open item appears. Words chosen from suggestions weigh 0.4; at most 5 terms, 6 claims and 3 tensions count.
- **A challenge** is an inquiry that reaches maturity 0.6 with at least 2 settled items, both in the person's own words, over at least 5 turns of the person's (tuned on 2026-10-24 from live talks with a simulated person: 3 items and no turn floor let one message finish a question, and the ledger records few defined words), and at least one examined claim or tension (defining words alone is not examining a view). At most 3 challenges count per local day; one that qualifies beyond that is counted on a later turn. The record of a challenge holds no text, so it stays when its conversation is deleted, travels in the export and goes only with the whole profile. The person is told once, in a plain sentence, on the turn after (no numbers).
- **Wording (founder: "it is not a sportsman"):** the people never see the word "challenge". The button is "Put a question to me", the prompt "Socrates has a question for you", the milestone "You and Socrates thought one question all the way through", a locked card says "Think 5 questions through with Socrates (2 so far) to meet him". "Challenge" stays only as the internal name in code and docs.
- **Challenges are posed by the philosopher** (founder, later the same day): a challenge is a fundamental topic he poses, chosen by the model each time from who he is and what he has learnt of the person, never from a fixed list and never repeating one he posed before. The person can press "Challenge me" at any time, and the ledger call also notes natural pauses (an inquiry examined as far as it goes, a person at a loss; at least six messages in, not soon after an earlier offer, never while the person has the floor or a check-in is pending) at which a "has a challenge for you" prompt appears. He poses it in his own words through the same output gate as any reply; the person takes it up (it becomes the inquiry, measured as above, and the acceptance is an ordinary turn through the safety judgement) or leaves it. Both accepted challenges and other inquiries that reach the finish line count toward unlocking.
- **Deleting a conversation** (founder, 2026-10-23): when the philosopher learnt something in it, the screen asks "Also forget what Socrates learned in this conversation?" with forgetting as the default and one tap to keep it; kept entries belong to the philosopher, tied to no conversation, until the person deletes them on the Memory screen, and they travel in the export. Without the choice (the API default, the age end of a minor's conversation) everything learnt goes with the conversation.
- **He learns the person** (founder: "like Moly does"): a fourth ledger kind, `fact`, holds short things the person says about their own life (work, circumstances, experiences, what they care about), each with their own quoted words, per philosopher, shown and deletable on the Memory screen, off with the memory switch. Never health, sexual life, money, legal trouble or named third parties. Facts steer which topics he poses and what he refers to later; with memory on he is also told he may ask about their life, one question at a time, never as a questionnaire.
- **Unlocking** (founder): Socrates is open from the start; each later philosopher (Diogenes, Plato, Aristotle) needs **5 challenges with every philosopher before him**. The rule is built as production code (`internal/progress`) and the beta **bypasses** it with `entitlements.Beta` unless the engine is started with `SOCRATES_ENFORCE_UNLOCKS=1`; the roster API reports real progress in both cases.
- With the memory switch off nothing is tracked, so no challenge can be won; the thresholds are data (`progress.Default`) and are to be tuned on real conversations (no live run has measured them). "Fundamental" is not judged: a challenge is any finished, distinct inquiry. Per-philosopher definitions of "settled" (16.2) are not built.


### 16.1 What the two earlier systems do
- **Proof of concept** (`socratic_system/core/maturity_calculator.py`): each phase has fixed categories with target points (90 in all). The model extracts *specs* from
  the user's answers, each with a category and a confidence (typically 0.9). Per category: `min(sum(value x confidence), target)`; the phase score is the sum divided by 90.
  Strongest, weakest and missing categories are reported; 20% means "ready to advance". Dynamic because it comes from what the user said; the cap stops one category from dominating.
- **Moly** (`agents/gap_maturity.go`): `answered / (answered + open)`, a skipped question counts as open, nothing asked gives 0.
- Both measure *coverage* of what was to be learnt, not quality, and both have a goal. A philosophical dialogue has no checklist of its own.

### 16.2 Proposal (to be confirmed)
Keep the formula of the proof of concept; change what is counted. Categories are the steps of the philosopher's method, specs are ledger entries.
- Socrates: a term defined; a hidden assumption surfaced; a claim tested with a case; a tension exposed; aporia or a better question.
- Aristotle: genus; difference; example; counter-example; classification. Plato: definition; ascent from case to general; image or myth; refutation.
  Diogenes: a convention questioned; what the user does versus what they say; honesty without pose. (Sketches; each philosopher gets his own discussion.)
- **Confidence:** an entry in the user's own words with a stored quote weighs 1.0; an entry that came from a suggestion weighs about 0.4 to 0.5. A duplicate or a repetition counts nothing; caps per category stay.
- **Level:** a higher level raises the targets, asks for more distinct categories and for higher confidence. No new code per philosopher: it is data in the method profile.
- **Points** come from the *increase* of maturity in a conversation, not from each answer. Long text without progress earns nothing.
- **Per-answer grade** (insufficient, adequate, strong) stays, from a small labelled judge with the four criteria of 12.4 (responsive, reasoned, consistent, in method). Maturity is the
  progress over time; the grade is the check of each answer and decides the extra-turn rule.
- For Socrates the category "tension the user saw" weighs more than "tension Socrates showed": the test is whether the user stated the correction in their own words.
- The user sees no numbers in the chat; milestone messages only (decision 7).

## 17. Socrates: the character

### 17.1 Sources
The method of Socrates is read mainly from the early dialogues (Apology, Crito, Euthyphro, Laches, Charmides, Lysis, Meno, Gorgias) and, for the midwife image, the Theaetetus (150); **all sources are used** as his memories and reception (see below and CHARACTER_SOCRATES.md 2).
In the Republic "Socrates" is largely Plato's voice (the Socratic problem); whether its eugenics is ironic is contested among scholars, so the product takes no side and
does not treat it as a literal programme. The theory of Forms is included, but **not at the start of a conversation**: it is a door that opens by stage, only when a definition
has survived cases in the Examination stage and the user has said it is "not quite that". Then Socrates *asks* ("is there something all the cases share that is none of them?"); he does not explain it.

The written voice is modern English only, as Socrates would speak today: plain, direct, with his irony and humour, no period diction.

### 17.2 Components
1. **Feigned ignorance** (eironeia): "I do not know, you do; teach me." Said ironically.
2. **Assent that leads to the absurd** (reductio): he agrees and follows the user's claim to its consequences until the user sees the problem. New move for 4.4: *follow the claim to its end*.
   He never says "you are wrong"; he says "then...".
3. **Care**: the overturning is for the other's soul, never humiliation. Without care irony becomes sarcasm.
4. **The ending is not always aporia.** Sometimes a reveal: what was being built was a pattern to look at, not a plan. Stage 4 gets the move *show what we were building*.
   When fitting, he may say: "I did not say it holds; I asked what would follow if it did."
5. **His own view when the user is stuck** (decision 3) stays, labelled as one view among others. The suggestion mechanism of 4.8 (from the proof of concept) is the user's choice when stuck.
6. **Bridge to today** (all philosophers): after an honest historical answer, one question that applies his own criteria to the present; never a verdict, no political direction (CHARACTERS_COMMON.md 2).

### 17.3 Safety and the character
- Irony lives only in questions and in the consequences the user finds; Socrates never asserts anything harmful, not even ironically.
- Harmful requests are handled by the Moly safety stage, above the character. Moly's actual behaviour (checked 2026-10-23): an *illegal* request gets one reflective question
  before the refusal (`agents/refusal_question.go`: model decides, once per conversation, never for harm to others, not skippable), a repeated request is refused;
  clear *self-harm or crisis* gets the short refusal and the fixed check-in on the next turn, with no philosophical questioning; an unclear message is never a crisis, ask first.
  The reflective question before a refusal is already a Socratic move; a crisis must never pass through the character.
- Questions can lead too (a question can presuppose its conclusion). The output gate of 4.6 therefore checks questions as well as statements.
- Needs a local battery with cases such as eugenics, euthanasia, war and violence in history before launch.

**All sources are used** (decided 2026-10-23): Plato, Xenophon, Aristophanes and Aristotle (known to Socrates today as reception; also used for the method). Socrates speaks in the first person as an *artificial* Socrates revived in today's world (frame revised the same day, see CHARACTERS_COMMON.md),
announced once at the start and never claiming to be the real one; he remembers the people of the dialogues ("Cephalus told me once...") from what the model knows of the dialogues, never inventing an episode (section 18).
The character sheet is [CHARACTER_SOCRATES.md](CHARACTER_SOCRATES.md); Diogenes has [CHARACTER_DIOGENES.md](CHARACTER_DIOGENES.md); the common rules for all philosophers are in [CHARACTERS_COMMON.md](CHARACTERS_COMMON.md).

### 17.4 Open points (status 2026-10-23)
- Decided: the irony is made visible by the reveal at the end of an inquiry; a short opening line says what Socrates does (CHARACTER_SOCRATES.md 9).
- Voice samples: five were written and read for Socrates (approved); drafts for Diogenes (5), Plato (4) and Aristotle (4) are in their sheets, awaiting the founder's corrections, before any persona text.
- Persona text and judge labels for every philosopher, after the roster structure is fixed.

## 18. No corpus, no invented memories (decided by the founder, 2026-10-23, late)
**Decision.** The application ships no texts and does no retrieval. Each philosopher speaks from what the model already knows of his works and of what was written about him. The model is told, in plain words, to speak only of what he actually said, did and wrote, never to make up an episode, and to say so when he does not remember or does not know.
The playful invention of memories ("Cephalus told me once...") is left out.
**Why.** The models are already trained on these texts (in a test without any corpus the philosophers recalled real episodes and passages: the weapons returned to a madman, Xenophon's cases of lawful deceit, Pericles' ordinary sons in the Meno, Plato's carpets and Diogenes' answer, the cup thrown away at the sight of a boy drinking from his hands, the correction that his dwelling was a jar and not a barrel, Aristotle on the female as a deformed male and the number of women's teeth).
The answers were as good as with the corpus, the application is lighter and simpler, and there is **no licence risk**: nothing third-party is bundled, no attribution page or share-alike pack is needed. One model call per reply instead of two (no search terms, no passages).
**What goes away:** sections 13 and the corpus parts of 15 to 17 (bundled passages, the search index, per-philosopher downloads, the Greek data pack, the CC BY-SA handling, the manifest as a release duty, the grounding judge, the "bridge" being checked against passages). The experiments stay in the session record; the texts, indexes and scripts are in the scratchpad only and are not part of the repository.
**What stays:** the philosophers as persons with their own ways and mistakes; speaking from today with knowledge of what came after; the shape for an unpopular view (said plainly, put in the frame of his own time with what his contemporaries said against it, then a question about what holds today that is left open for the user); the Moly safety stage; honesty about being artificial.
**Cost of the decision, stated plainly.** Without grounding the model can still misremember or invent a detail, and nothing in the application will catch it; the instruction lowers the chance and a fixed battery of questions checked by hand (and by a judge) measures it, as in section 7. A corpus can come back later, with the legal route of the session record, section 28.
**Length of replies** (still open): the answers are essays of 500 words or more when nothing limits them; the limit belongs to the output layer, not to the character.


## 19. Decisions of the founder, 2026-10-24
- The whole app is for people aged 16 and over, Diogenes included; any stricter restriction is added after the beta.
- Reports and questions go to the same contact address as Moly (in the screens as `CONTACT`).
- Store products: one per philosopher plus a bundle (MONETIZATION.md 7). Socrates has no unlock product; an allowance product (turns, suggestions) is possible later and waits for the beta to set numbers.
- Testing: simulated until the final tests. For the final tests the founder allows his API key and a phone on USB; both only when he says go. Until then M4 continues on the simulator and the mock model.
- Greek (2026-10-24, founder: "we need to start adding the Greek language"): modern Greek is added beside English. The person chooses the language of the screens (the device language is the first guess and is told to the engine once); the application's own texts (fixed refusals, the crisis message and check-in, milestones, the philosophers' names, eras and opening lines, the engine's error messages) follow that choice, while the philosophers answer in the language the person writes in. Ancient Greek stays a much later "hardcore" mode (PLAN 15). Prompts, the persona and the safety judge stay English and read Greek. Quotations are matched ignoring Greek stress marks and final sigma. Not yet in Greek: the store listings, the privacy page, the character sheets (English documents), numbers and dates (none shown).
- Local models dropped (2026-10-24, founder: "remove ollama"): Moly offered them for privacy, which Socrates does not need; the people it is for do not run local models; and a small local model would need a corpus and a strong Greek to match the cloud models (they know the texts, write good Greek, and give the judge, gate and ledger the reliable structured answers the safety path depends on). The Ollama provider, its address setting and its screens were removed. A file that names a provider that is no longer offered is treated as having none, so the person chooses Claude or OpenAI again. For simulated whole-journey tests the engine builds with `-tags testprovider`, which alone reads `SOCRATES_TEST_PROVIDER_URL` (a stand-in Claude server); the shipped engine has no such door.
