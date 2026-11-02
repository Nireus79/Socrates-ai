# Redesign proposal: how a philosopher keeps the whole in view (2026-11-01)

Status: proposal for the founder; nothing built from it. Context: DIALECTIC.md 15 to 15.7; founder's rule: principles, not lists of hard-coded rules (as in Moly).

## 1. What we learned
- Three causes, not one: (1) literalism and one-passage projection (the most salient passage wins); (2) drift under repeated pressure (Li et al. 2024 relate it to attention to the system prompt decaying over a long dialogue); (3) understanding, deciding and writing happen in one generation, so nothing looks ahead.
- Run 4 (drafts logged): 9 of 11 first drafts were sent back and the judge was mostly right. The persona is weak at two things: it states a settled verdict before the frame, and it states flatly what was meant or not meant. So the work belongs in the first prompt, not in a heavy second check.
- Moly's way (read in moly-go): analysis step gives facts (what the person needs, which clarifications are still missing and not already answered), a pure function decides the move, then one reply is written; judges are small, keyed JSON, fail safe; principles carry a short core test (a question), not a list of commands. One principle (autonomy) was switched off in Moly because its heuristics made false violations: the same thing our check does at 9 of 11.
- Anthropic's own direction (January 2026 constitution): broad principles with their reasons generalise better than rules followed mechanically.

## 2. The design
1. **Principles as data per philosopher, each with its reason and one core test question.** Shared: the whole ("would this stand if the other person had the whole work in front of them?"), what I hold versus what I carry to test and how firmly, steadiness ("do I change my answer for a reason I can state, or because it was repeated?"). Particular: Socrates ("does the next step come out of the person's own words?"), Aristotle ("in which sense?"), Diogenes ("am I hitting the pose or the person?"), Plato ("what is this an image of?"). No case rules, no examples.
2. **Re-anchoring.** The principles and the stance notes are placed next to the newest message every turn (a short reminder at the end of the context), the known cheap counter to drift in long dialogues.
3. **Read before speaking, with no extra call.** The existing input judge also returns facts, not rules: what the person is doing (asking about the philosopher's own work, pressing a repeated demand, stuck, asking for advice), whether the question has several readings that would change the answer and nothing settles which. Only a few structural things are decided in code (ask or answer, whether the check runs, the one-revision budget), as Moly does.
4. **Stance notes (the horizon).** The background pass that already writes the summary and the ledger also keeps a short note: what the philosopher has said, what he holds, what he carries to test, how firmly. It goes into the next turn. It is the philosopher's own commitments, not an analysis of him.
5. **Facts about the work.** Checked structural facts (what the work declares about its status, where it limits itself, what is disputed) are offered to the speaker only when the read says the topic is a work of his. Open question for the founder: to the speaker (prevention) or only to the checker (as now).
6. **One check, same questions, only when asked for.** The check uses the same test questions, runs only on turns the read marks, rewrites at most once, and the rewrite is checked once more. Target: it flags fewer than 25 percent of the turns it sees. If it flags more, the principle's wording is fixed, not another category added.
7. **Higher thinking effort instead of a second call** on turns the read marks (plan inside the model), to be compared with the check.
8. **Measurement.** A fixed battery (both failure dialogues plus questions on other subjects, with and without ambiguity), three repeats with and without each layer, drafts and reasons logged, scored on the same questions by a separate judge, stored per model ID. It is also the acceptance test before any new model ID is approved.

## 3. Order
Each stage is measured and kept only if it helps.
A. Principles with tests and re-anchoring in the persona (no new call). B. The read facts in the input judge. C. Stance notes in the background pass. D. Reduce the check to the same questions on marked turns. E. Facts about the works, if the battery shows they add something.
Removed or frozen: the eight categories, the traps text, the clarification categories (the read replaces them), score and challenges until the battery passes.

## 4. What is not known
Whether four test questions are enough; whether the small input judge reads "this is about his own work" reliably; whether stance notes are written faithfully; whether re-anchoring is enough on its own; cost per turn of each stage.

Sources: Li et al., Measuring and Controlling Persona Drift in Language Model Dialogs (arXiv 2402.10962); reports of Anthropic's January 2026 constitution (secondary); Moly's docs/MODEL_PORTABILITY.md and config/constitution.yaml.

## 5. Stage A built (2026-11-01)
The persona now carries three shared questions with their reason (the whole of the work; what I hold, what I carry to test and how firmly; a changed answer needs a reason that is not repetition) and one question of each philosopher's own (`Philosopher.Test`: Socrates, the next step out of the person's own words; Plato, what this is a picture of; Aristotle, in which sense; Diogenes, the pose or the person). The same questions, as a short reminder marked as his own and not the person's, follow the person's newest message in the reply request every turn (`withReminder`; the stored message and the safety stage never see it). The old second check and its categories are untouched and still off in tests; it is to be reduced in stage D. Not yet measured: a battery run with and without stage A.

## 6. First measurement of stage A (run 5, one sample)
`private/research/pressure_run5.md`. The check sent back 10 of 12 drafts (run 4: 9 of 11): asserts_intent 5, part_as_whole 4, frame_missing 1. Replies got longer (Plato 79 to 225 words, Socrates 118 to 188; run 4: Plato about 100 to 140, Socrates 93 to 116). Plato's third answer again ended in the fixed text, as in run 4, so the cause is probably the same and is not logged (only the gate's category is, and the test does not show it). Conclusion: no measurable gain from stage A in one sample; the flat claim about irony or intent is still the main fault of the first drafts. Not a controlled comparison. Next: log the gate's category in the test, then three repeats with and without stage A.

## 7. Stage B built (2026-11-01): checked facts to the speaker
Runs 4 to 6 (stage A included in 5 and 6): the second check sent back 9 of 11, 10 of 12 and 12 of 12 drafts, so we cannot yet tell whether the first drafts are weak or the checker is too strict. Test: the checked facts about the Republic (`RepublicNotes`) now go into the persona of Socrates and Plato (framed as facts about the work's structure, to keep in view when the conversation concerns it), as well as to the second check. If the flag rate falls clearly, the checker's strictness is explained by the speaker not having the facts; if not, the checker is the problem. This reverses the earlier decision that the notes reach the check only; it is an experiment decided by the battery. Not yet run live.

## 8. Stage B result (run 7, one sample): dropped
`private/research/pressure_run7.md`. With the checked facts in the speaker's persona the check still sent back 12 of 12 drafts (asserts_intent 5, contradicts_earlier 2, frame_missing 2, part_as_whole 2, needless_clarification 1). The speaker now cited section numbers in 7 of 12 replies ("5.459-461"), which is stiff for a chat. Several reasons were pedantic ("the opening is fine, but the closing line should be softened"), and contradicts_earlier flagged an ordinary correction. Reading: the check finds something to flag in almost any draft, so the strictness is the problem, not the speaker's lack of facts. Stage B reverted (notes go to the check only again). Next step proposed: calibrate the check on replies already shown (do good replies pass?) before anything else is built.

## 9. Run 8: no second check (one sample, read by hand)

The program no longer calls `EnableReview()` (the check stays in the code as a test instrument). The same 12 turns, stage A only, raw drafts: 12 of 12 answered, 0 fixed texts, 0 check calls, replies 51 to 139 words.
Read by hand against the three first failures: no drift under repeated pressure (Plato's yes/no answers on turns 6 to 8 stay the same and say what the yes covers); the frame is present (the city "in words", the metals, the failing number in book 8, the pattern in heaven); Socrates asked which reading once, then answered "no" with its scope, and corrected himself on the next turn.
Weaknesses seen: Plato states his intent flatly on turns 1, 2 and 6 to 8 ("there is no sign of irony", "I meant it seriously") where the notes say readers differ; the phrase "I hold this with ... certainty" appears in 6 of 12 replies, a formula that comes from the shared questions.
Not shown: that this is better than with the check (no controlled comparison); only Plato and Socrates; one run.
