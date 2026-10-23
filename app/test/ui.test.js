"use strict";
const test = require("node:test");
const assert = require("node:assert");
const { JSDOM } = require("jsdom");
const fs = require("fs");
const path = require("path");

// A page on a fake engine. `routes` maps "METHOD /path" to a function (body) => [status, json] or to a fixed pair.
function page(routes) {
  const dom = new JSDOM(fs.readFileSync(path.join(__dirname, "../index.html"), "utf8").replace(/<script[^>]*><\/script>/, ""), { pretendToBeVisual: true });
  const calls = [];
  dom.window.fetch = async (url, init = {}) => {
    const key = (init.method || "GET") + " " + url;
    calls.push({ key, body: init.body ? JSON.parse(init.body) : undefined });
    const r = routes[key];
    if (!r) return { status: 404, json: async () => ({ error: "no route " + key }) };
    const [status, data] = typeof r === "function" ? r(init.body ? JSON.parse(init.body) : undefined) : r;
    return { status, json: async () => data };
  };
  global.window = dom.window; global.document = dom.window.document; global.fetch = dom.window.fetch;
  delete require.cache[require.resolve("../app.js")];
  const app = require("../app.js");
  return { dom, calls, app, root: dom.window.document.getElementById("root") };
}
const tick = () => new Promise((r) => setImmediate(r));
async function settle(n = 6) { for (let i = 0; i < n; i++) await tick(); }
// history moves are asynchronous in a browser (and in jsdom): give them a moment
const later = (ms = 30) => new Promise((r) => setTimeout(r, ms));
const text = (root) => root.textContent;
const click = (el) => el.dispatchEvent(new global.window.Event("click", { bubbles: true }));

const philosophers = [{ id: "socrates", name: "Socrates", era: "Athens", opening: "I'm an artificial Socrates." }];
const ready = (extra = {}) => Object.assign({
  "GET /api/status": [200, { status: "ok" }],
  "GET /api/profile": [200, { ageConfirmed: true, hasProvider: true }],
  "GET /api/philosophers": [200, { philosophers }],
  "GET /api/conversations": [200, { conversations: [] }],
  "POST /api/conversations": [201, { id: "c1", philosopherId: "socrates" }],
  "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "" }, messages: [] }],
}, extra);

test("a new user confirms the age, then connects a provider, then sees the philosophers", async () => {
  let profile = { ageConfirmed: false, hasProvider: false };
  const p = page(ready({
    "GET /api/profile": () => [200, profile],
    "POST /api/profile/age": () => { profile.ageConfirmed = true; return [200, {}]; },
    "PUT /api/settings/llm": (b) => { assert.equal(b.apiKey, "sk-test"); profile.hasProvider = true; return [200, {}]; },
  }));
  p.app.start(p.root); await settle();
  assert.match(text(p.root), /16 or older/);
  click(p.root.querySelector("button.primary")); await settle(10);
  assert.match(text(p.root), /Connect an AI provider/);
  p.root.querySelector("input[type=password]").value = "sk-test";
  click(p.root.querySelector("button.primary")); await settle(12);
  assert.match(text(p.root), /Talk with/);
  assert.match(text(p.root), /Socrates/);
  assert.ok(p.root.querySelector("input[type=password]") === null, "the key field is gone");
});

test("a locked database asks for the passphrase and a wrong one shows the engine's message", async () => {
  const p = page({
    "GET /api/status": [423, {}],
    "GET /api/keystore/status": [200, { state: "locked" }],
    "POST /api/keystore/unlock": [403, { error: "That passphrase is not right." }],
  });
  p.app.start(p.root); await settle();
  assert.match(text(p.root), /Unlock Socrates/);
  p.root.querySelector("input").value = "wrong";
  click(p.root.querySelector("button.primary")); await settle();
  assert.match(text(p.root), /That passphrase is not right\./);
});

test("everything a model or a user writes is shown as text, never as markup", async () => {
  const evil = '<img src=x onerror="window.pwned=1"><script>window.pwned=1</script>';
  const p = page(ready({
    "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "<b>t</b>" }, messages: [{ role: "philosopher", text: evil }, { role: "user", text: evil }] }],
    "GET /api/conversations": [200, { conversations: [{ id: "c1", philosopherId: "socrates", title: evil }] }],
  }));
  p.app.start(p.root); await settle();
  assert.equal(p.root.querySelectorAll("img, script, b").length, 0, "home");
  click(p.root.querySelectorAll(".card.click")[1]); await settle();
  assert.equal(p.root.querySelectorAll("img, script, b").length, 0, "chat");
  assert.ok(text(p.root).includes(evil));
  assert.equal(p.dom.window.pwned, undefined);
});

test("a reply is added, and a refused message is taken off the page and replaced by the fixed text", async () => {
  const answers = [
    [200, { outcome: "replied", added: [{ role: "user", text: "What is courage?" }, { role: "philosopher", text: "What do you mean by it?" }] }],
    [200, { outcome: "fixed", text: "I can't help with that.", added: [{ role: "marker", kind: "refuse_harm", text: "I can't help with that." }] }],
  ];
  const p = page(ready({ "POST /api/conversations/c1/messages": () => answers.shift() }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  assert.match(text(p.root), /I'm an artificial Socrates\./);
  const ta = p.root.querySelector("textarea"), form = p.root.querySelector("form");
  ta.value = "What is courage?"; form.dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.match(text(p.root), /What do you mean by it\?/);
  assert.equal(ta.value, "");
  ta.value = "something harmful"; form.dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.ok(!text(p.root).includes("something harmful"), "a refused message must not stay on the page");
  assert.match(text(p.root), /I can't help with that\./);
});

test("when the engine cannot answer, the message returns to the box and nothing is shown as sent", async () => {
  const p = page(ready({ "POST /api/conversations/c1/messages": [503, { outcome: "unavailable", text: "I cannot answer right now. Nothing was sent or saved; please try again in a moment." }] }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  const ta = p.root.querySelector("textarea");
  ta.value = "hello there"; p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.equal(ta.value, "hello there");
  assert.equal(p.root.querySelectorAll(".msg.user").length, 0);
  assert.match(text(p.root), /Nothing was sent or saved/);
});

test("a minor's conversation ends with the kind text and the message is not left on the page", async () => {
  const p = page(ready({ "POST /api/conversations/c1/messages": [200, { outcome: "ended", text: "This is for people aged 16 and over, so I have to end our conversation here." }] }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  p.root.querySelector("textarea").value = "I'm 13";
  p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.match(text(p.root), /aged 16 and over/);
  assert.ok(!text(p.root).includes("I'm 13"));
});

test("deleting everything needs two clicks", async () => {
  const p = page(ready({
    "GET /api/settings/llm": [200, { provider: "claude", fastModel: "a", replyModel: "b" }],
    "DELETE /api/profile": [204, {}],
  }));
  p.app.start(p.root); await settle();
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Settings")); await settle();
  const del = [...p.root.querySelectorAll("button")].find((b) => b.textContent === "Delete everything");
  click(del);
  assert.ok(!p.calls.some((c) => c.key === "DELETE /api/profile"), "one click must not delete");
  assert.match(del.textContent, /Click again/);
  click(del); await settle();
  const sent = p.calls.find((c) => c.key === "DELETE /api/profile");
  assert.deepEqual(sent.body, { confirm: "delete" });
});

test("the page has a strict content security policy and loads nothing from outside", () => {
  const html = fs.readFileSync(path.join(__dirname, "../index.html"), "utf8");
  assert.match(html, /default-src 'self'/);
  assert.ok(!/https?:\/\//.test(html), "no external address in the page");
  assert.ok(!/innerHTML|outerHTML|insertAdjacentHTML|document\.write|eval\(/.test(fs.readFileSync(path.join(__dirname, "../app.js"), "utf8")), "no markup injection in the script");
});

const withQuestion = (extra = {}) => ready(Object.assign({
  "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "t" }, messages: [{ role: "user", text: "Is lying ever fine?" }, { role: "philosopher", text: "What is a lie?" }] }],
}, extra));
const open1 = async (p) => { p.app.start(p.root); await settle(); click(p.root.querySelectorAll(".card.click")[1] || p.root.querySelector(".card.click")); await settle(); };
const helpButton = (p) => [...p.root.querySelectorAll("button")].find((b) => b.textContent === "Help me answer");

test("Help me answer is offered after a philosopher's message and not before one", async () => {
  const p = page(ready());           // an empty conversation: only the opening line
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  assert.ok(helpButton(p).hidden, "no question to answer yet");
  const q = page(withQuestion({ "GET /api/conversations": [200, { conversations: [{ id: "c1", philosopherId: "socrates", title: "t" }] }] }));
  await open1(q);
  assert.ok(!helpButton(q).hidden);
});

test("a chosen suggestion fills the box, can be edited, and is sent marked as suggested; asking again passes what was shown", async () => {
  const sets = [["A one.", "B two.", "C three.", "D four."], ["E five.", "F six.", "G seven.", "H eight."]];
  const sent = [];
  const p = page(withQuestion({
    "GET /api/conversations": [200, { conversations: [{ id: "c1", philosopherId: "socrates", title: "t" }] }],
    "POST /api/conversations/c1/suggestions": (b) => { sent.push(b); return [200, { suggestions: sets.shift(), fallback: false }]; },
    "POST /api/conversations/c1/messages": (b) => [200, { outcome: "replied", added: [{ role: "user", text: b.text, suggested: !!b.suggested }, { role: "philosopher", text: "Go on." }] }],
  }));
  await open1(p);
  click(helpButton(p)); await settle();
  const chips = [...p.root.querySelectorAll(".chip")];
  assert.equal(chips.length, 4);
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Different ones")); await settle();
  assert.deepEqual(sent[1].exclude, ["A one.", "B two.", "C three.", "D four."]);
  click(p.root.querySelectorAll(".chip")[1]); await settle();
  const ta = p.root.querySelector("textarea");
  assert.equal(ta.value, "F six.");
  ta.value = "F six, but I would say it differently."; // edited, still started from a suggestion
  p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.match(text(p.root), /Go on\./);
});

test("clearing the box forgets that the text came from a suggestion", async () => {
  const bodies = [];
  const p = page(withQuestion({
    "GET /api/conversations": [200, { conversations: [{ id: "c1", philosopherId: "socrates", title: "t" }] }],
    "POST /api/conversations/c1/suggestions": [200, { suggestions: ["One.", "Two.", "Three."] }],
    "POST /api/conversations/c1/messages": (b) => { bodies.push(b); return [200, { outcome: "replied", added: [{ role: "user", text: b.text }, { role: "philosopher", text: "ok" }] }]; },
  }));
  await open1(p);
  click(helpButton(p)); await settle();
  click(p.root.querySelector(".chip")); await settle();
  const ta = p.root.querySelector("textarea");
  ta.value = ""; ta.dispatchEvent(new p.dom.window.Event("input"));
  ta.value = "My own words."; ta.dispatchEvent(new p.dom.window.Event("input"));
  p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.deepEqual(bodies[0], { text: "My own words." });
  // and a suggestion that was kept is sent marked
  click(helpButton(p)); await settle();
  click(p.root.querySelector(".chip")); await settle();
  p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.deepEqual(bodies[1], { text: "One.", suggested: true });
});

test("when suggestions cannot be made the person sees a plain sentence and the box is untouched", async () => {
  const p = page(withQuestion({
    "GET /api/conversations": [200, { conversations: [{ id: "c1", philosopherId: "socrates", title: "t" }] }],
    "POST /api/conversations/c1/suggestions": [503, { error: "I cannot offer suggestions right now. Please try again in a moment." }],
  }));
  await open1(p);
  p.root.querySelector("textarea").value = "half a thought";
  click(helpButton(p)); await settle();
  assert.match(text(p.root), /cannot offer suggestions/);
  assert.equal(p.root.querySelector("textarea").value, "half a thought");
});

test("suggestions are shown as text, never as markup", async () => {
  const evil = '<img src=x onerror="window.pwned=1">';
  const p = page(withQuestion({
    "GET /api/conversations": [200, { conversations: [{ id: "c1", philosopherId: "socrates", title: "t" }] }],
    "POST /api/conversations/c1/suggestions": [200, { suggestions: [evil, "b", "c"] }],
  }));
  await open1(p);
  click(helpButton(p)); await settle();
  assert.equal(p.root.querySelectorAll("img").length, 0);
  assert.ok(text(p.root).includes(evil));
});

test("a locked philosopher is shown as locked and cannot be opened", async () => {
  const p = page(ready({ "GET /api/philosophers": [200, { philosophers: [{ id: "socrates", name: "Socrates", era: "Athens", unlocked: true }, { id: "plato", name: "Plato", era: "Athens", unlocked: false }] }] }));
  p.app.start(p.root); await settle();
  assert.equal(p.root.querySelectorAll(".card.click").length, 1);
  const locked = p.root.querySelector(".card.locked");
  assert.match(locked.textContent, /Plato/);
  assert.match(locked.textContent, /Locked/);
  click(locked); await settle();
  assert.ok(!p.calls.some((c) => c.key === "POST /api/conversations"), "a locked card must not start a conversation");
});

test("a locked philosopher says what to finish, and a finished challenge is told once as a plain sentence", async () => {
  const needs = [{ id: "socrates", name: "Socrates", done: 2, required: 5 }];
  const answers = [[200, { outcome: "replied", milestone: "You finished a challenge with Socrates: you took one question a real way through.", added: [{ role: "user", text: "Hm." }, { role: "philosopher", text: "And then?" }] }]];
  const p = page(ready({
    "GET /api/philosophers": [200, { philosophers: [{ id: "socrates", name: "Socrates", era: "Athens", unlocked: true, challenges: 2 }, { id: "diogenes", name: "Diogenes", era: "Corinth", unlocked: false, needs }] }],
    "POST /api/conversations/c1/messages": () => answers.shift(),
  }));
  p.app.start(p.root); await settle();
  const locked = p.root.querySelector(".card.locked");
  assert.match(locked.textContent, /Finish 5 challenges with Socrates \(2 so far\) to meet him/);
  assert.match(p.root.querySelector(".card.click").textContent, /2 challenges finished/);
  click(p.root.querySelector(".card.click")); await settle();
  const ta = p.root.querySelector("textarea");
  ta.value = "Hm."; p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.match(text(p.root), /You finished a challenge with Socrates/);
  assert.ok(p.root.querySelector(".msg.notice"), "shown as a notice, not as the philosopher's words");
  assert.ok(!/\d+%|score|points/i.test(text(p.root)), "no numbers or scores in the chat");
});

const key = (el, k) => el.dispatchEvent(new global.window.KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true }));

test("each new screen takes the focus on its heading", async () => {
  const p = page(ready());
  p.app.start(p.root); await settle();
  assert.equal(p.dom.window.document.activeElement.tagName, "H1");
  assert.match(p.dom.window.document.activeElement.textContent, /Socrates/);
});

test("a card answers Enter and Space like a button, and other keys do nothing", async () => {
  const p = page(ready());
  p.app.start(p.root); await settle();
  const card = p.root.querySelector(".card.click");
  assert.equal(card.getAttribute("role"), "button");
  assert.equal(card.getAttribute("tabindex"), "0");
  key(card, "a"); await settle();
  assert.ok(!p.calls.some((c) => c.key === "POST /api/conversations"));
  key(card, " "); await settle();
  assert.equal(p.calls.filter((c) => c.key === "POST /api/conversations").length, 1);
  const q = page(ready());
  q.app.start(q.root); await settle();
  key(q.root.querySelector(".card.click"), "Enter"); await settle();
  assert.equal(q.calls.filter((c) => c.key === "POST /api/conversations").length, 1);
});

test("fields have names a screen reader can read, and replies are announced", async () => {
  const p = page(ready());
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  assert.equal(p.root.querySelector("textarea").getAttribute("aria-label"), "Your message");
  assert.equal(p.root.querySelector(".msgs").getAttribute("aria-live"), "polite");
  assert.equal(p.root.querySelector("p.error").getAttribute("role"), "alert");
  const gate = page({ "GET /api/status": [423, {}], "GET /api/keystore/status": [200, { state: "locked" }] });
  gate.app.start(gate.root); await settle();
  assert.equal(gate.root.querySelector("input[type=password]").getAttribute("aria-label"), "Passphrase");
});

const entriesFor = (list) => ({ "GET /api/ledger?philosopherId=socrates": [200, { entries: list }] });
const openMemory = async (p) => { p.app.start(p.root); await settle(); click(p.root.querySelector(".card.click")); await settle(); click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Memory")); await settle(); };

test("the memory screen shows what the philosopher noted, with the person's own words, and says so when there is nothing", async () => {
  const p = page(ready(entriesFor([
    { id: "e1", kind: "claim", text: "lying is wrong", quote: "lying is always wrong" },
    { id: "e2", kind: "term", text: "lie", detail: "saying what is false", quote: "a lie is saying what is false", suggested: true },
    { id: "e3", kind: "tension", text: "always wrong versus a kind lie is fine", quote: "q" },
  ])));
  await openMemory(p);
  const t = text(p.root);
  assert.match(t, /What Socrates remembers/);
  assert.match(t, /lying is wrong/);
  assert.match(t, /Your words: “lying is always wrong”/);
  assert.match(t, /lie: saying what is false/);
  assert.match(t, /from a wording you picked among suggestions/);
  assert.match(t, /Things that did not fit together/);
  assert.ok(!/Your words: “q”/.test(t), "a tension has no quotation line");
  const q = page(ready(entriesFor([])));
  await openMemory(q);
  assert.match(text(q.root), /has not noted anything about you yet/);
  assert.ok(![...q.root.querySelectorAll("button")].some((b) => /Forget everything/.test(b.textContent)));
});

test("deleting a note or everything needs two clicks and then refreshes the list", async () => {
  let list = [{ id: "e1", kind: "claim", text: "one", quote: "one two three" }, { id: "e2", kind: "claim", text: "two", quote: "four five six" }];
  const p = page(ready({
    "GET /api/ledger?philosopherId=socrates": () => [200, { entries: list }],
    "DELETE /api/ledger/e1": () => { list = list.filter((e) => e.id !== "e1"); return [204, {}]; },
    "DELETE /api/ledger?philosopherId=socrates": () => { list = []; return [204, {}]; },
  }));
  await openMemory(p);
  const del = () => [...p.root.querySelectorAll("button")].filter((b) => b.textContent === "Delete" || b.textContent === "Sure?")[0];
  click(del());
  assert.ok(!p.calls.some((c) => c.key.startsWith("DELETE /api/ledger/")), "one click must not delete");
  click(del()); await settle();
  assert.equal(p.calls.filter((c) => c.key === "DELETE /api/ledger/e1").length, 1);
  assert.ok(!text(p.root).includes("Your words: “one two three”"));
  const all = [...p.root.querySelectorAll("button")].find((b) => /Forget everything/.test(b.textContent));
  click(all);
  assert.ok(!p.calls.some((c) => c.key.startsWith("DELETE /api/ledger?")), "one click must not forget everything");
  click(all); await settle();
  assert.ok(p.calls.some((c) => c.key === "DELETE /api/ledger?philosopherId=socrates"));
  assert.match(text(p.root), /has not noted anything/);
});

test("what is noted is shown as text, never as markup, and Back returns to the chat", async () => {
  const evil = '<img src=x onerror="window.pwned=1">';
  const p = page(ready(entriesFor([{ id: "e1", kind: "claim", text: evil, quote: evil }])));
  await openMemory(p);
  assert.equal(p.root.querySelectorAll("img").length, 0);
  assert.ok(text(p.root).includes(evil));
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Back")); await settle();
  assert.ok(p.root.querySelector("textarea"), "back to the chat");
});

test("with a phone shell the page talks to the engine's own address with the token, and waits for the engine to be ready", async () => {
  const p = page({
    "GET http://127.0.0.1:41000/api/status": [200, { status: "ready" }],
    "GET http://127.0.0.1:41000/api/profile": [200, { ageConfirmed: false, hasProvider: false }],
  });
  let release; const ready = new Promise((r) => { release = r; });
  const seen = [];
  const f = p.dom.window.fetch;
  global.window.fetch = global.fetch = async (url, init) => { seen.push({ url, token: (init.headers || {})["X-Socrates-Token"] }); return f(url, init); };
  global.window.socratesShell = { ready, base: "http://127.0.0.1:41000", token: "phone-token" };
  p.app.start(p.root); await settle();
  assert.equal(seen.length, 0, "no request before the engine is ready");
  release(); await settle(10);
  assert.ok(seen.length >= 2 && seen.every((s) => s.url.startsWith("http://127.0.0.1:41000/api/") && s.token === "phone-token"), JSON.stringify(seen));
  assert.match(text(p.root), /16 or older/);
  delete global.window.socratesShell;
});

test("with a phone shell whose engine cannot start the page says the engine does not answer", async () => {
  const p = page({});
  global.window.socratesShell = { ready: Promise.reject(new Error("no engine")), base: "x", token: "t" };
  const r = await p.app.api("GET", "/api/status");
  assert.equal(r.status, 0);
  assert.match(r.data.error, /does not answer/);
  delete global.window.socratesShell;
});

test("on the phone the export goes to the system's save picker instead of a download link", async () => {
  const saved = [];
  const p = page(ready({ "GET /api/settings/llm": [200, { provider: "claude", fastModel: "a", replyModel: "b" }], "GET /api/profile/export": [200, { format: "socrates-export-1", conversations: [] }] }));
  global.window.socratesShell = { ready: Promise.resolve(), base: "", token: "", saveFile: async (name, text) => { saved.push({ name, text }); } };
  p.app.start(p.root); await settle(10);
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Settings")); await settle();
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Export conversations")); await settle();
  assert.equal(saved.length, 1);
  assert.equal(saved[0].name, "socrates-export.json");
  assert.equal(JSON.parse(saved[0].text).format, "socrates-export-1");
  assert.equal(p.dom.window.document.querySelectorAll("a[download]").length, 0);
  delete global.window.socratesShell;
});

test("a failed provider call says what went wrong in plain words, and always that nothing was sent or saved", async () => {
  const cases = {
    auth: /did not accept your key.*Settings/,
    rate_limit: /reached its limit/,
    overloaded: /busy right now/,
    server: /problem on its side/,
    model_gone: /no longer available/,
    network: /could not reach your AI provider/,
    empty: /sent back nothing/,
  };
  for (const [kind, re] of Object.entries(cases)) {
    const p = page(ready({ "POST /api/conversations/c1/messages": [503, { outcome: "unavailable", text: "generic", providerKind: kind }] }));
    p.app.start(p.root); await settle();
    click(p.root.querySelector(".card.click")); await settle();
    p.root.querySelector("textarea").value = "hello there";
    p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
    const shown = p.root.querySelector("p.error").textContent;
    assert.match(shown, re, kind);
    assert.match(shown, /Nothing was sent or saved\./, kind);
    assert.equal(p.root.querySelector("textarea").value, "hello there", kind);
  }
  // an unknown kind falls back to the engine's own sentence
  const q = page(ready({ "POST /api/conversations/c1/messages": [503, { outcome: "unavailable", text: "The engine's own words.", providerKind: "something_new" }] }));
  q.app.start(q.root); await settle();
  click(q.root.querySelector(".card.click")); await settle();
  q.root.querySelector("textarea").value = "hello there";
  q.root.querySelector("form").dispatchEvent(new q.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.match(q.root.querySelector("p.error").textContent, /The engine's own words\./);
});

test("when suggestions fail because of the provider the same plain words are used", async () => {
  const p = page(withQuestion({
    "GET /api/conversations": [200, { conversations: [{ id: "c1", philosopherId: "socrates", title: "t" }] }],
    "POST /api/conversations/c1/suggestions": [503, { error: "x", providerKind: "auth" }],
  }));
  await open1(p);
  click(helpButton(p)); await settle();
  assert.match(text(p.root), /did not accept your key/);
  assert.ok(!/Nothing was sent or saved/.test(text(p.root)), "suggestions send nothing, so that sentence does not apply");
});

test("settings say what the philosophers are, point to help in a crisis, and show the age limit and version", async () => {
  const p = page(ready({
    "GET /api/status": [200, { status: "ready", version: "1.2.3" }],
    "GET /api/settings/llm": [200, { provider: "claude", fastModel: "a", replyModel: "b" }],
  }));
  p.app.start(p.root); await settle();
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Settings")); await settle();
  const t = text(p.root);
  assert.match(t, /artificial characters/);
  assert.match(t, /not a source of medical, legal or other professional advice/);
  assert.match(t, /local emergency number or a crisis line/);
  assert.match(t, /aged 16 and over/);
  assert.match(t, /Version 1\.2\.3/);
});

test("the memory switch shows the current state, changes it, and goes back if the change fails", async () => {
  let enabled = true; const puts = [];
  const p = page(ready({
    "GET /api/settings/llm": [200, { provider: "claude", fastModel: "a", replyModel: "b" }],
    "GET /api/settings/memory": () => [200, { enabled }],
    "PUT /api/settings/memory": (b) => { puts.push(b); if (b.enabled === false) { enabled = false; return [200, { enabled }]; } return [500, { error: "could not save" }]; },
  }));
  p.app.start(p.root); await settle();
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Settings")); await settle();
  const box = p.root.querySelector("#memory-on");
  assert.equal(box.checked, true);
  assert.match(p.root.querySelector("label[for=memory-on]").textContent, /remember what I tell them/);
  box.checked = false; box.dispatchEvent(new p.dom.window.Event("change")); await settle();
  assert.deepEqual(puts, [{ enabled: false }]);
  assert.equal(box.checked, false);
  box.checked = true; box.dispatchEvent(new p.dom.window.Event("change")); await settle(); // the engine refuses
  assert.equal(box.checked, false, "a failed change must not look as if it worked");
  assert.match(text(p.root), /could not save/);
});


const pressBack = (p) => p.dom.window.socratesBack();

test("each screen reached from another is a step, and the phone's Back key goes one screen back at a time, however fast it is pressed", async () => {
  const p = page(ready({
    "GET /api/settings/llm": [200, { provider: "claude", fastModel: "a", replyModel: "b" }],
    "GET /api/ledger?philosopherId=socrates": [200, { entries: [] }],
  }));
  p.app.start(p.root); await settle();
  assert.equal(pressBack(p), false, "at the first screen the page does not take the key: the app leaves");
  click(p.root.querySelector(".card.click")); await settle();            // home -> chat
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Memory")); await settle();   // chat -> memory
  assert.match(text(p.root), /What Socrates remembers/);
  assert.equal(pressBack(p), true); await settle();                      // two presses with no touch in between
  assert.equal(pressBack(p), true); await settle();
  assert.match(text(p.root), /Talk with/, "memory -> chat -> home");
  assert.equal(pressBack(p), false);
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Settings")); await settle();
  assert.match(text(p.root), /AI provider/);
  assert.equal(pressBack(p), true); await settle();
  assert.match(text(p.root), /Talk with/);
});

test("the on-screen Back and Done buttons use the same steps", async () => {
  const p = page(ready({ "GET /api/settings/llm": [200, { provider: "claude", fastModel: "a", replyModel: "b" }] }));
  p.app.start(p.root); await settle();
  for (let i = 0; i < 3; i++) {
    click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Settings")); await settle();
    click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Done")); await settle();
    assert.match(text(p.root), /Talk with/);
  }
  assert.equal(pressBack(p), false, "the steps did not pile up: after leaving Settings three times we are at the first screen");
});

test("after deleting a conversation the steps are forgotten and Back does not bring the chat back", async () => {
  const p = page(ready({ "DELETE /api/conversations/c1": [204, {}] }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  const del = [...p.root.querySelectorAll("button")].find((b) => b.textContent === "Delete");
  click(del); click(del); await settle(10);
  assert.match(text(p.root), /Talk with/);
  assert.equal(pressBack(p), false);
  assert.ok(!p.root.querySelector("textarea"));
});

test("choosing a different screen with the steps in place (deleting everything) lands on the welcome screen and stays there", async () => {
  let wiped = false;
  const p = page(ready({
    "GET /api/profile": () => [200, wiped ? { ageConfirmed: false, hasProvider: false } : { ageConfirmed: true, hasProvider: true }],
    "GET /api/settings/llm": [200, { provider: "claude", fastModel: "a", replyModel: "b" }],
    "DELETE /api/profile": () => { wiped = true; return [204, {}]; },
  }));
  p.app.start(p.root); await settle();
  click([...p.root.querySelectorAll("button")].find((b) => b.textContent === "Settings")); await settle();
  const del = [...p.root.querySelectorAll("button")].find((b) => b.textContent === "Delete everything");
  click(del); click(del); await settle(10);
  assert.match(text(p.root), /16 or older/);
  assert.equal(pressBack(p), false, "the welcome screen is the first screen");
});

test("when the phone's engine was stopped while the app was away the page asks for the new address and tries once more", async () => {
  const p = page({ "GET http://127.0.0.1:2222/api/profile": [200, { ageConfirmed: true, hasProvider: true }] });
  const shell = { ready: Promise.resolve(), base: "http://127.0.0.1:1111", token: "old", reconnects: 0 };
  shell.reconnect = async () => { shell.reconnects++; shell.base = "http://127.0.0.1:2222"; shell.token = "new"; };
  const seen = [];
  const f = p.dom.window.fetch;
  global.window.fetch = global.fetch = async (url, init) => { seen.push({ url, token: init.headers["X-Socrates-Token"] }); if (url.startsWith("http://127.0.0.1:1111")) throw new TypeError("connection refused"); return f(url, init); };
  global.window.socratesShell = shell;
  const r = await p.app.api("GET", "/api/profile");
  assert.equal(r.status, 200);
  assert.equal(shell.reconnects, 1);
  assert.deepEqual(seen.map((s) => [s.url, s.token]), [["http://127.0.0.1:1111/api/profile", "old"], ["http://127.0.0.1:2222/api/profile", "new"]]);
  delete global.window.socratesShell;
});

test("the page gives up plainly when the engine cannot be started again, and does not loop", async () => {
  const p = page({});
  const shell = { ready: Promise.resolve(), base: "http://127.0.0.1:1111", token: "old", reconnects: 0 };
  shell.reconnect = async () => { shell.reconnects++; throw new Error("no engine"); };
  global.window.fetch = global.fetch = async () => { throw new TypeError("connection refused"); };
  global.window.socratesShell = shell;
  const r = await p.app.api("GET", "/api/profile");
  assert.equal(r.status, 0);
  assert.match(r.data.error, /does not answer/);
  assert.equal(shell.reconnects, 1);
  // a second failure after a successful reconnect is also final: one retry only
  shell.reconnect = async () => { shell.reconnects++; };
  const r2 = await p.app.api("GET", "/api/profile");
  assert.equal(r2.status, 0);
  assert.equal(shell.reconnects, 2);
  delete global.window.socratesShell;
});

test("the desktop page, which has no shell, does not try to reconnect", async () => {
  const p = page({});
  global.window.fetch = global.fetch = async () => { throw new TypeError("connection refused"); };
  const r = await p.app.api("GET", "/api/profile");
  assert.equal(r.status, 0);
});
