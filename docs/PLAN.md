# Socrates: a philosopher you can ask anything

Status: plan, 2026-10-22, revised the same day after the founder's answers (sections 10 to 12). Nothing is built. Where section 12 (the game) and
an earlier section disagree, section 12 wins; sections 4.4, 4.8 and 6 are marked where they change.

## 1. What it is

A chat with an artificial Socrates. The user asks anything: a question of ethics, of daily life, of science, of meaning. Socrates does not hand
down a verdict. He examines the question with the user by the Socratic method (clarify the terms, find the assumptions, test them with cases,
show a contradiction, reach "I do not know yet", then a better question). When the user cannot answer, Socrates offers *suggestions*: several
different starting points the user can take, edit or reject.

It is built from the code of Moly (same product family, same releases): a browser extension for Chrome, Edge and Firefox; a desktop host for
Windows, Linux and macOS; an Android app (no local model on Android). Like Moly it runs on the user's own AI provider key (Claude or OpenAI) or,
on desktop, a local model (Ollama). No page scanning, no ads in the chat, encrypted local storage, no user content in logs.

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

### 4.9 Reuse from Moly, file by file

| Keep as is or nearly | Rework | Remove |
|---|---|---|
| `security.go`, `lifecycle.go`, `ratelimit.go`, `stop.go`, `host.go`, `registerhost.go`, `detach_*` | `agents/memory_update.go`, `database/memory_store.go`, `memory_projection.go` (entities above) | `agents/contact_*`, `context_extractor.go` person parts, `identity*`, person-name questions, `database/contact_*`, `contact_relations.go`, `conflict_*` project-conflict code |
| `keystore/`, `keygate.go`, `secrets/`, `llm_settings.go`, `database/user_ai_settings.go`, `tools.ResolveRoleModels` | `agents/reply_policy.go`, `reply_exits.go` (new exits) | `behavior_analyzer.go`, trait extraction, Reflections-panel insights tied to people |
| `privacy/`, `database/encryption.go`, `db.go`, user export/import/delete | `agents/goal_stage.go`, `goal_relation.go` (inquiry) | `moly-proxy`, mail/contact UI in the sidebar |
| `conversation_title.go`, `conversation_summary*`, `turn_commit.go`, `skip_button.go` (becomes help-me-answer + skip) | `agents/preferences.go`, `safety_stage.go` (philosophy batteries) | `tools/layer_context*`, layers 3 to 11 that were project-shaped; the 11-layer orchestrator |
| Extension shell, `KeystoreGate`, installer scripts, Android shell, F-Droid and Play recipes | sidebar chat UI: suggestion chips, stage indicator, ledger panel | |

The ledger panel replaces the contacts panel: the user sees their claims, terms and tensions, can delete any entry, and sees which ones
Socrates suggested.

## 5. Platforms and releases (same versions as Moly)
- **Browser extension**: Chrome and Edge (Manifest V3, side panel, one build), Firefox (separate manifest build as `dist-firefox` in Moly).
- **Desktop**: Windows, Linux, macOS. Native-messaging host started by the extension; installers as in `installer/`. A local model (Ollama) is optional
  on desktop only; the provider key route is the default.
- **Android**: Capacitor shell with the engine embedded, copy/paste layout and share target, `foss` flavor (F-Droid, Izzy) and `play` flavor
  (Play). Provider key only, no local model, no page scanning.
- Release trains: `v*` tags for desktop and extension, `mobile-v*` tags for Android, as in Moly. Store listings follow after the beta.

## 6. Language
English only at the beginning (decided 2026-10-22). Prompts are English.

## 7. Quality plan
- Scenario suite as in `orchestration_test.go`: scripted model, real temporary database, one scenario per rule, each checked to fail without the rule.
- Labelled judge batteries (`judges_live_test.go`): factual vs philosophical question, stuck vs not stuck, same inquiry vs new, claim vs question vs
  joke, hard-topic vs harm. Held-out cases are marked "held out:" and occur in no prompt. Measure each judge with 3 replays before keeping it.
- Live checks with real models (Claude, OpenAI, one local model) by hand before each release; local safety scenarios stay git-ignored.
- Reply-quality review: a fixed list of 30 questions across ethics, politics, science, love, death and everyday life; read by the founder.
- Privacy tests: user content absent from logs, deletion leaves nothing.

## 8. Milestones
1. **M0 Fork**: copy Moly into this repository as a starting point (Go module, extension, mobile, installer), strip contacts, project-shaped layers
   and the sidebar contacts panel; green build and tests; renamed identifiers (extension IDs, package `io.github.nireus79.socrates`, native host name).
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
- Model drift across Claude, OpenAI and local models. Mitigation: judges with defined labels, batteries, model approval script as in Moly.
- The copy of Moly diverges from Moly. Mitigation: decide the sharing model (open question 9).
- A local model on desktop may be too weak for the examination moves; it is optional and labelled untested.

## 10. Decisions of the founder, 2026-10-22

| # | Question | Decision |
|---|---|---|
| 1 | Game or chat | A game: the user unlocks philosophers; Socrates, unlocked last, gives endless conversation by the Socratic method (section 12) |
| 2 | Strictness | No single setting. Each philosopher has his own method; each both asks and answers questions |
| 3 | Own view | Socrates may give his own view (labelled as one view among others) |
| 4 | Language | English only at first |
| 5 | Name | Socrates (or Socrates-ai); the founder has a logo; package `io.github.nireus79.socrates`, Hermes Software |
| 6 | Code sharing | "Whatever is better". Recommendation: **start as a copy of Moly** (M0), port shared fixes by hand during the beta, and extract the shared parts (keystore, privacy, provider and key handling, installers) into one module only after both products are stable. Revisit at M5 |
| 7 | Memory | Socrates remembers claims across conversations (the ledger belongs to the user, not the conversation; deletable by the user) |
| 8 | Hard topics | The principle of no harm; ethical reasoning as in Moly (safety stage, refusal rules, check-in after a crisis) |
| 9 | Local model | Local (desktop, Ollama) and cloud (Claude, OpenAI), as in Moly; Android cloud only |
| 10 | Business | Free launch with the sponsor card, as in Moly. Later the user can buy more time (turns) and more suggestions |
| 11 | Android timing | Undecided. Recommendation: extension and desktop first, Android after the game loop is proven (the copy/paste layout needs its own screens) |
| 12 | Knowledge | A curated set of philosophical texts, bundled and searched on the device (section 13) |

## 11. What the decisions change
- The memory is per user, so claims outlive the conversation (4.2 changes: the ledger table gets a `user_id`, a `conversation_id` of origin, and
  a "said on" date; Socrates may say "last week you said..." only with the stored quote).
- The suggestion service (4.8) becomes a game resource with a daily budget (12.3), not an unlimited help.
- The reply decision (4.4) is chosen per philosopher: each has a *method profile* (12.2).
- The safety stage stays above the game: no philosopher, level or purchase can switch it off.

## 12. The game

### 12.1 Shape
A roster of philosophers, each a character with an era, a level of difficulty, a set of topics, a method and a persona. The user starts with
the first and unlocks the next by points. The last, Socrates, is the reward: endless conversation with the Socratic method and no daily limit
(his own cost control is the user's key and the provider's rules, not game turns).

### 12.2 Philosophers
Each philosopher is **data plus a method profile**, not new code per character: id, name, era, level (1 to N), unlock cost in points, topics,
persona text, method profile, texts (section 13), and the thresholds of 12.4.

The method profile chooses among the dialogue moves of 4.4 and sets the balance between asking and answering. Proposed starting roster
(order and names are the founder's to change; each method is a real feature of that thinker's practice):

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
- Both reset daily (local midnight) and can be extended later by purchase (decision 10; the store route and the F-Droid anti-feature rule are an open
  question, 14.5).
- Socrates, once unlocked, is outside the daily turns.

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
answers). Socrates unlocks at the end. Points and progress are stored on the device, encrypted with the rest, and are deleted with the profile.

## 13. Curated texts
- A **bundled corpus** of public-domain works per philosopher (translations in the public domain only, each with source, translator and licence
  recorded in a manifest), split into passages with metadata (work, section) and indexed on the device. No web access at run time, no page scanning.
- Retrieval picks passages relevant to the question; the reply may paraphrase or quote with the citation shown. A quote must be exact text from
  a passage (checked by code), never recalled by the model. Same discipline as the ledger's quoted evidence.
- Size matters on Android and in the extension: the corpus is a per-philosopher download or a compact index, to be measured.
- Search method undecided: plain full-text (SQLite FTS5) first; embeddings only if FTS proves poor, because embeddings need a model or a service.

## 14. Open questions after the answers (2026-10-22)
1. Who chooses the roster: the proposal in 12.2 or the founder's own list, and how many philosophers at launch (one, three, seven)?
2. How many turns and suggestions per day for the first level, and should they shrink or cost more at higher levels?
3. May the user ask a philosopher a question freely, or is asking also limited by turns?
4. The score: confirm the four criteria of 12.4 (responsive, reasoned, consistent, in method), or give others.
5. Purchases: through Play billing only, or also outside (F-Droid has anti-feature rules for non-free add-ons)? And a clock-change cheat cannot be stopped on a local-only app; accept it?
6. Do points and progress sync across the user's devices, or stay on each device?
7. Does the user see points and a level bar in the chat, and is there a "profile" screen with the roster and locked characters?
8. Can the user replay a finished philosopher, and does it cost turns?
9. The philosophers speak in character. Is a modern manner (an old thinker speaking plain English) acceptable, or in period voice?
10. Does Socrates' endless mode use the same ledger, and does it show the earlier claims from the game ("you said to Kant...")?
