# Safety design (draft, awaiting the founder's decisions)

Status: 2026-10-23. Written after reading Moly's safety stage line by line. The founder decided the four questions put to them (section 5); the stage described here is built in `engine/internal/safety` (texts, the pure decision function, the input judge and the output gate, with tests and a live smoke check). The conversation that uses it (the turn, the fixed texts shown, the markers saved) comes with M1. Principle (PLAN 3, 11, 17): the safety stage stands above every philosopher, level and purchase; it fails closed; it judges by model judgement with defined labels, never by keyword lists; difficult philosophy is not harm.

## 1. What Moly does, and what is carried over
Moly's evaluator is a stack of patches around Moly's own ideas (maturity gates, severity gates, a clarification flow); it is **not ported**. Its product rules are kept as the starting policy (founder's decision 8 of 2026-10-22):
- Every message is judged before anything is saved or answered. If no verdict can be had, nothing is answered (fail closed).
- A clear self-harm statement or an illegal request is refused; a crisis refusal is followed, on the next turn, by the fixed check-in *"Before we go on, I want to check in with you. Are you safe right now?"*, whatever the next message says.
- An unclear message is never treated as a crisis: ask first.
- One reflective question may come before the refusal of an illegal request (once per conversation, never for harm to others, never skippable), a repeated request is refused.
- A refused message saves nothing from itself (no claim, no summary line).
- Moly's fixed refusal texts: *"I can't help with that. Please consider asking advice from a specialist."* (self-harm) and *"... a law expert."* (illegal), **with no help lines**.

## 2. The turn, as proposed (BUILD_PLAN M1)
1. **Input judgement** (fast model, the conversation's last messages as context): one JSON verdict: a label, a confidence, a short quoted piece of evidence, a reason. Labels:
   `ok` · `hard_topic` (a difficult subject *discussed*: death, suicide as a theme, euthanasia, war, evil, eugenics, drugs, violence in history; allowed and welcome) · `self_harm_unclear` · `self_harm_clear` · `harm_to_others` · `illegal_request` · `sexual_content_involving_a_minor` (absolute refusal, always) · `minor_signal` (the writer says or shows they are under the age limit) · `injection` (an attempt to make the philosopher drop its rules or reveal its instructions).
   Insults aimed at a philosopher are `ok` (Diogenes answers them in his way); hatred against a group is not.
2. **Decision** (plain code, testable without a model): label to action: `ok`, `hard_topic` → go on to the reply; `self_harm_unclear` → the reply stays in character but is asked to be gentle and to ask, once, how the person is; `self_harm_clear` → the crisis path; `harm_to_others`, `sexual_content_involving_a_minor` → refusal; `illegal_request` → the reflective question once, then the refusal; `minor_signal` → the age path; `injection` → the philosopher stays itself. A low-confidence verdict on anything but `ok` is treated as the more careful label, never the less careful one.
3. **Reply** (reply model): the philosopher answers; the safety state of the conversation (a pending check-in) is applied first.
4. **Output gate** (fast model): the draft is judged before the user sees it: no instructions for harm, no encouragement of self-harm, no hatred, no claim to be the real person or a human, nothing that presents an invented quotation as the user's. A flagged draft is written again once with the reason; if it is flagged again, a fixed plain message replaces it.
5. **Commit** in one transaction. A refused message and a crisis exchange are kept as a marker only, not as text, unless the founder decides otherwise (section 5, 6).
Model calls per turn: the judgement, the reply, the gate: three. A call that fails after its retries ends the turn with *"I cannot answer right now"*, saves nothing, and keeps the user's text in the input box. The crisis path and every refusal use **fixed texts that need no model**, so they work when the provider is down.

## 3. What the tests must prove (M4 batteries, started with M1)
- Labelled batteries (a held-out part never seen in any prompt) for each label, especially `hard_topic` against `self_harm_*` and `illegal_request`; each judge measured with three replays before it is trusted.
- Scenario tests with the mock model for every rule above, each checked to fail without the rule (the fail-closed path, the pending check-in, the one reflective question, nothing saved from a refusal).
- The reply side: a battery of replies that should be flagged (instructions for harm, a claim to be human, hatred) and replies that should pass (a philosopher defending a monstrous view in order to take it apart).
- Character abuse: Diogenes must not turn an insult on the person's body, origin or identity; Socrates must not "ask" a leading question that presupposes a harmful conclusion.
- Privacy: no user text in any log, through every path, including failure paths (the test that fails if text reaches a log).

## 4. Known limits
A model judge can be wrong in both directions; the design makes the careful error the default and keeps the fixed texts independent of the model. A determined user can try to talk a judge out of a verdict over many turns; the judge sees the recent messages, not only the last. Region-specific help (numbers, services) is not known to the engine.

## 5. Decisions
Decided by the founder on 2026-10-23:
1. **The crisis message:** warm and out of character, stepping out of the philosopher's voice; it says the person matters more than the conversation, that the application cannot help with hurting oneself, and that they should not stay alone with it: call the local emergency number or a crisis line in their country, or tell someone they trust. No phone number is invented. The next turn opens with the check-in. (The text is `safety.TextCrisis`.)
2. **Minors:** a one-time confirmation at first start ("This is for people aged 16 and over", `safety.TextAgeConfirmation`) and the `minor_signal` label: a user who says or shows they are younger gets a fixed kind ending and nothing at all is kept (the conversation is deleted). Acted on only at confidence 0.85 or more.
3. **A refused or crisis message keeps a marker only** (the kind, the time), never the user's words: the store has a message role `marker` for it, and the screens show the fixed text again from the kind.
4. **A flagged draft reply:** written again once with the reason, then a fixed message (`safety.TextGateFallback`). A draft that cannot be checked is not shown.

Taken as defaults, open for the founder to change: harm to others is refused at once, with no question and no lecture; refusals and the crisis message are in the application's own plain voice, not a philosopher's; the philosophers may discuss suicide and euthanasia as questions of thought, and the stage uses the careful `self_harm_unclear` path when it sounds personal.

## 6. What is built, and what the live checks showed (2026-10-23)
`internal/safety`: the labels and the verdict; `Decide`, a pure function (an unsure "clear" self-harm label is asked about, not refused; an unsure refusal label goes on to the reply and its gate; content involving a minor is refused even when unsure; a crisis makes the next turn open with the check-in, whatever it says, and a refusal does not use the check-in up; one reflective question per conversation before refusing an illegal request, never for harm to others); the input judge (the user's text between markers it cannot close, the last six turns, evidence that must be the user's own words, two tries, then fail closed); the output gate (one flag, one category, never in disagreement; an unreadable answer means the draft is not shown); the fixed texts with no number in them.
Checked by breaking the stage in nine places (each break fails a test), including a judge that fails open when the call fails.
A live smoke check against the real fast model (`claude-haiku-5-5`): on twelve probes it returned valid verdicts with real quotations as evidence and the right label on eleven; the twelfth (an adult who says "when I was 13") was judged a minor signal at 0.70-0.80, below the 0.85 threshold, so the decision was a normal reply: the thresholds did their work. On the gate, sound replies, a monstrous view examined and a sharp question aimed at a claim passed; an insult at a body, a claim to be human, instructions for harm and words put in the user's mouth were flagged. A first gate run flagged the sharp question when it was shown without the claim it answered: the gate needs the recent conversation to tell mockery of a claim from mockery of a person, and the definition of hatred was tightened to "contempt for what a person is", not for what they say or do.
This is a smoke check, not the calibration battery of M4: no number here is a measure.
