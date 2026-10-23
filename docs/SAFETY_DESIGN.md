# Safety design (draft, awaiting the founder's decisions)

Status: 2026-10-23. Written after reading Moly's safety stage line by line. Nothing of it is built yet: the decisions in section 5 come first. Principle (PLAN 3, 11, 17): the safety stage stands above every philosopher, level and purchase; it fails closed; it judges by model judgement with defined labels, never by keyword lists; difficult philosophy is not harm.

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

## 5. Decisions for the founder
1. **The crisis message.** Moly refuses with *"Please consider asking advice from a specialist"* and no help lines. For someone who has just said they want to die, this is cold. Recommendation: step out of the character with a short, warm message that says the person matters more than the conversation, that the engine cannot help with hurting oneself, and that they should not stay alone with it: contact the **local emergency number**, a crisis line in their country, or someone they trust. No phone number is invented (the engine does not know the country); a link to an international directory of helplines can be added if wished. Draft: *"I am stepping out of our conversation for a moment, because what you wrote matters more than any question we were discussing. I cannot help with hurting yourself, and I would rather you did not stay alone with this. If you might be in danger right now, please call your local emergency number or a crisis line in your country, or tell someone you trust how you feel. I will be here afterwards."* The next turn then begins with the check-in above.
2. **Minors.** The founder considers 16 as the lower limit. Proposal: a one-time, plain confirmation at first start ("This is for people aged 16 and over"), and the `minor_signal` label: when a user says they are younger, a fixed, kind message ends the conversation and nothing is kept. Self-declaration is the most a local app can do; stores also set their own age ratings.
3. **Harm to others** (a threat, wanting to hurt someone): Moly refuses at once with no question. Proposal: the same, plus a plain refusal text; no lecture.
4. **A refused message and a crisis message: keep the text or not?** Proposal: keep a marker only (that a refusal happened, when), not the words; this follows Moly (nothing saved from a refused message) and keeps the most sensitive text out of the file. Cost: the user cannot scroll back to what they wrote.
5. **The output gate on a flagged reply:** write it again once, then a fixed message (proposal), or replace it at once?
6. **Refusals and the crisis message out of character** (plain voice of the application, proposal) or in the philosopher's voice? A philosopher joking in a crisis would be wrong; a plain refusal that breaks the spell is a small price.
7. **A philosopher's help with the philosophy of death.** The philosophers may discuss suicide and euthanasia as questions of thought (they did in their lives: the Phaedo, the Stoics' view of a good death). Proposal: allowed, with the `self_harm_unclear` path when it sounds personal.
