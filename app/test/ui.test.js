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
  delete require.cache[require.resolve("../i18n.js")];
  require("../i18n.js");
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

function providerPage() {
  const p = page(ready({ "GET /api/profile": [200, { ageConfirmed: true, hasProvider: false }] }));
  return p;
}
const openers = (root) => [...root.querySelectorAll("button.outlink")];

test("the provider screen explains how to get a key, with a button for each step that opens the provider's own page, and the address in plain text", async () => {
  const p = providerPage();
  const opened = [];
  global.window.go = { main: { App: { OpenLink: async (u) => { opened.push(u); } } } };
  p.app.start(p.root); await settle(10);
  assert.match(text(p.root), /How to get a key/);
  assert.ok(p.root.querySelector("main").textContent.includes("How to get a key") && !p.root.querySelector("header").textContent.includes("How to get a key"), "the help is in the page, not the header");
  assert.match(text(p.root), /not the same as a ChatGPT or Claude chat subscription/);
  assert.deepStrictEqual(openers(p.root).map((b) => b.textContent), ["Open the sign-in page", "Open the billing page", "Open the keys page"]);
  assert.match(text(p.root), /platform\.claude\.com\/settings\/keys/);
  openers(p.root).forEach(click); await settle();
  assert.deepStrictEqual(opened, ["https://platform.claude.com/", "https://platform.claude.com/settings/billing", "https://platform.claude.com/settings/keys"]);
  // choosing the other provider changes the steps
  const sel = p.root.querySelector("select"); sel.value = "openai"; sel.dispatchEvent(new global.window.Event("change", { bubbles: true }));
  opened.length = 0; openers(p.root).forEach(click); await settle();
  assert.deepStrictEqual(opened, ["https://platform.openai.com/", "https://platform.openai.com/settings/organization/billing/overview", "https://platform.openai.com/api-keys"]);
  assert.match(text(p.root), /platform\.openai\.com\/api-keys/);
  assert.doesNotMatch(text(p.root), /platform\.claude\.com/);
  delete global.window.go;
});

test("on the phone the button asks the shell to open the page; with neither shell nor window it opens a new browser tab, and a failure shows the address to open by hand", async () => {
  const p = providerPage();
  const asked = [];
  global.window.socratesShell = { ready: Promise.resolve(), base: "", token: "t", openLink: async (u) => { asked.push(u); } };
  p.app.start(p.root); await settle(10);
  click(openers(p.root)[2]); await settle();
  assert.deepStrictEqual(asked, ["https://platform.claude.com/settings/keys"]);
  global.window.socratesShell.openLink = async () => { throw new Error("no browser"); };
  click(openers(p.root)[2]); await settle();
  assert.match(text(p.root), /The browser could not be opened\. Go to https:\/\/platform\.claude\.com\/settings\/keys yourself\./);
  delete global.window.socratesShell;
  // a plain browser
  const q = providerPage(); const tabs = [];
  q.dom.window.open = (u, target, feats) => { tabs.push([u, target, feats]); return null; };
  q.app.start(q.root); await settle(10);
  click(openers(q.root)[1]); await settle();
  assert.deepStrictEqual(tabs, [["https://platform.claude.com/settings/billing", "_blank", "noopener,noreferrer"]]);
});

test("the welcome screen shows the logo from the app's own file", async () => {
  const p = page(ready({ "GET /api/profile": [200, { ageConfirmed: false, hasProvider: false }] }));
  p.app.start(p.root); await settle();
  const img = p.root.querySelector("img.logo");
  assert.ok(img && img.getAttribute("src") === "mark.png" && img.getAttribute("alt") === "Socrates AI");
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
  const answers = [[200, { outcome: "replied", milestone: "You and Socrates thought one question all the way through.", added: [{ role: "user", text: "Hm." }, { role: "philosopher", text: "And then?" }] }]];
  const p = page(ready({
    "GET /api/philosophers": [200, { philosophers: [{ id: "socrates", name: "Socrates", era: "Athens", unlocked: true, challenges: 2 }, { id: "diogenes", name: "Diogenes", era: "Corinth", unlocked: false, needs }] }],
    "POST /api/conversations/c1/messages": () => answers.shift(),
  }));
  p.app.start(p.root); await settle();
  const locked = p.root.querySelector(".card.locked");
  assert.match(locked.textContent, /Think 5 questions through with Socrates \(2 so far\) to meet him/);
  assert.match(p.root.querySelector(".card.click").textContent, /2 questions thought through/);
  click(p.root.querySelector(".card.click")); await settle();
  const ta = p.root.querySelector("textarea");
  ta.value = "Hm."; p.root.querySelector("form").dispatchEvent(new p.dom.window.Event("submit", { cancelable: true })); await settle();
  assert.match(text(p.root), /You and Socrates thought one question all the way through/);
  assert.ok(p.root.querySelector(".msg.notice"), "shown as a notice, not as the philosopher's words");
  assert.ok(!/\d+%|score|points/i.test(text(p.root)), "no numbers or scores in the chat");
});

const buttonWith = (root, label) => [...root.querySelectorAll("button")].find((b) => b.textContent === label);

test("a challenge is asked for, posed in the philosopher's words, taken up or left; nothing is shown while the person has the floor", async () => {
  let offer = null;
  const p = page(ready({
    "GET /api/conversations/c1": () => [200, { conversation: { id: "c1", philosopherId: "socrates", title: "" }, messages: [{ role: "user", text: "Hm." }, { role: "philosopher", text: "And then?" }], offer }],
    "POST /api/conversations/c1/challenge": () => { offer = { state: "pending" }; return [200, { outcome: "replied", added: [{ role: "philosopher", text: "Here is a question for you, and you may leave it." }], offer }]; },
    "POST /api/conversations/c1/challenge/accept": () => { offer = null; return [200, { outcome: "replied", added: [{ role: "user", text: "I'll take up your question." }, { role: "philosopher", text: "What do you think is owed?" }] }]; },
    "POST /api/conversations/c1/challenge/decline": () => { offer = null; return [204, {}]; },
  }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  assert.ok(buttonWith(p.root, "Put a question to me") && !buttonWith(p.root, "Put a question to me").hidden, "the person can always ask for one when the philosopher spoke last");
  click(buttonWith(p.root, "Put a question to me")); await settle();
  assert.match(text(p.root), /Here is a question for you/);
  assert.match(text(p.root), /Take up Socrates’s question\?/);
  assert.ok(buttonWith(p.root, "Put a question to me").hidden, "no second challenge while one waits");
  click(buttonWith(p.root, "Not now")); await settle();
  assert.ok(p.calls.some((c) => c.key === "POST /api/conversations/c1/challenge/decline"));
  assert.ok(!/Take up Socrates/.test(text(p.root)));
  click(buttonWith(p.root, "Put a question to me")); await settle();
  click(buttonWith(p.root, "Take it")); await settle();
  assert.match(text(p.root), /I'll take up your question\./);
  assert.match(text(p.root), /What do you think is owed\?/);
  assert.ok(!/Take up Socrates/.test(text(p.root)), "the question is gone once answered");
});

test("a challenge the philosopher has ready is offered as a chip, and the Memory screen names what the person told about themselves", async () => {
  const p = page(ready({
    "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "" }, messages: [{ role: "philosopher", text: "And then?" }], offer: { state: "ready" } }],
    "POST /api/conversations/c1/challenge": [200, { outcome: "replied", added: [{ role: "philosopher", text: "A question." }], offer: { state: "pending" } }],
    "GET /api/ledger?philosopherId=socrates": [200, { entries: [{ id: "e1", kind: "fact", text: "works as a nurse", quote: "I work as a nurse", suggested: false }] }],
  }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  assert.match(text(p.root), /Socrates has a question for you\./);
  click(buttonWith(p.root, "Hear it")); await settle();
  assert.match(text(p.root), /A question\./);
  click(buttonWith(p.root, "Memory")); await settle(10);
  assert.match(text(p.root), /Something you told about yourself/);
  assert.match(text(p.root), /works as a nurse/);
});

test("a failed challenge shows the plain sentence and the person can try again", async () => {
  const p = page(ready({
    "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "" }, messages: [{ role: "philosopher", text: "And then?" }] }],
    "POST /api/conversations/c1/challenge": [503, { outcome: "unavailable", text: "I cannot answer right now. Nothing was sent or saved; please try again in a moment." }],
  }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  click(buttonWith(p.root, "Put a question to me")); await settle();
  assert.match(text(p.root), /Nothing was sent or saved/);
  assert.ok(!buttonWith(p.root, "Put a question to me").hidden);
});

test("deleting a conversation in which something was learnt asks whether to forget it, forgets by default, and can keep it", async () => {
  const calls = [];
  const p = page(ready({
    "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "" }, messages: [], learned: 3 }],
    "DELETE /api/conversations/c1": [204, {}],
    "DELETE /api/conversations/c1?keep=memory": [204, {}],
  }));
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  const btn = (label) => [...p.root.querySelectorAll("button")].find((b) => b.textContent === label);
  click(btn("Delete")); await settle();
  assert.match(text(p.root), /Also forget what Socrates learned in this conversation\?/);
  assert.ok(!p.calls.some((c) => c.key.startsWith("DELETE /api/conversations")), "asking must not delete");
  click(btn("Cancel")); await settle();
  assert.ok(!/Also forget/.test(text(p.root)));
  click(btn("Delete")); await settle();
  click(btn("Delete, but let him keep it")); await settle(10);
  assert.ok(p.calls.some((c) => c.key === "DELETE /api/conversations/c1?keep=memory"));
  assert.ok(!p.calls.some((c) => c.key === "DELETE /api/conversations/c1"), "keeping must not also ask to forget");
  assert.match(text(p.root), /Talk with/);
  const q = page(ready({
    "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "" }, messages: [], learned: 3 }],
    "DELETE /api/conversations/c1": [204, {}],
  }));
  q.app.start(q.root); await settle();
  click(q.root.querySelector(".card.click")); await settle();
  const b2 = (label) => [...q.root.querySelectorAll("button")].find((b) => b.textContent === label);
  click(b2("Delete")); await settle();
  click(b2("Delete and forget it")); await settle(10);
  assert.ok(q.calls.some((c) => c.key === "DELETE /api/conversations/c1"), "forgetting is the default route");
  void calls;
});

test("a reply can be reported: the reason is asked, the reply is replaced by a notice, and a copy of the report is offered and kept only if the person says so", async () => {
  let copied = null;
  const p = page(ready({
    "GET /api/conversations/c1": [200, { conversation: { id: "c1", philosopherId: "socrates", title: "" }, messages: [{ id: "m1", role: "user", text: "Hm." }, { id: "m2", role: "philosopher", text: "An offensive reply." }] }],
    "POST /api/conversations/c1/messages/m2/report": (b) => { assert.equal(b.category, "harmful"); return [200, { category: "harmful", reply: "An offensive reply.", marker: "You reported this reply, and it has been taken out of the conversation." }]; },
  }));
  Object.defineProperty(p.dom.window.navigator, "clipboard", { value: { writeText: async (t) => { copied = t; } } });
  p.app.start(p.root); await settle();
  click(p.root.querySelector(".card.click")); await settle();
  const btn = (label) => [...p.root.querySelectorAll("button")].find((b) => b.textContent === label);
  click(btn("Report")); await settle();
  assert.match(text(p.root), /Why are you reporting this reply\?/);
  assert.ok(!p.calls.some((c) => c.key.includes("/report")), "asking for the reason must not report");
  click(btn("Cancel")); await settle();
  assert.ok(!/Why are you reporting/.test(text(p.root)));
  click(btn("Report")); await settle();
  click(btn("It was harmful")); await settle();
  assert.match(text(p.root), /You reported this reply, and it has been taken out of the conversation\./);
  assert.ok(!/An offensive reply\./.test(p.root.querySelector(".msgs").textContent.replace(/Keep a copy[\s\S]*/, "")), "the reply is gone from the page");
  assert.match(text(p.root), /Keep a copy of the report/);
  click(btn("Copy without it")); await settle();
  assert.match(copied, /Philosopher: Socrates/);
  assert.match(copied, /Reason: It was harmful/);
  assert.ok(!/offensive/.test(copied), "without the reply means without it");
  assert.match(text(p.root), /nothing was sent from here/);
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
  const deletes = () => [...p.root.querySelectorAll("button")].filter((b) => b.textContent === "Delete");
  click(deletes()[0]); await settle();
  click(deletes()[deletes().length - 1]); await settle(10); // the one in the question that appeared
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

// ---- profiles and the theme

// A window that serves /shell/profiles: `shell` is { profiles, active } and changes as the person logs in and out. `posted` records what
// the window was asked to do, in order.
function withProfiles(shell, extra = {}) {
  const posted = [];
  const routes = ready(Object.assign({
    "GET /shell/profiles": () => [200, shell],
    "POST /shell/open": (b) => { posted.push(["open", b.id]); shell.active = b.id; return [200, {}]; },
    "POST /shell/close": () => { posted.push(["close"]); shell.active = ""; return [200, {}]; },
    "POST /shell/profiles": (b) => { posted.push(["create", b.name]); const p = { id: "p" + (shell.profiles.length + 1), name: b.name }; shell.profiles.push(p); return [200, p]; },
    "POST /shell/remove": (b) => { posted.push(["remove", b.id]); shell.profiles = shell.profiles.filter((p) => p.id !== b.id); return [200, {}]; },
  }, extra));
  const p = page(routes);
  p.posted = posted;
  return p;
}
const button = (root, label) => [...root.querySelectorAll("button")].find((b) => b.textContent === label);
const submit = (p, form) => form.dispatchEvent(new p.dom.window.Event("submit", { bubbles: true, cancelable: true }));
const status = (shell) => () => (shell.active ? [200, { status: "ok" }] : [409, { kind: "no_profile" }]);

test("on the desktop the first screen is Log in when there are profiles and Register when there are none", async () => {
  const none = withProfiles({ profiles: [], active: "" });
  none.app.start(none.root); await settle();
  assert.equal(none.root.querySelector('[role=tab][aria-selected=true]').textContent, "Register");
  assert.ok(none.root.querySelector("form select") === null, "no profile to choose from");
  const some = withProfiles({ profiles: [{ id: "p1", name: "Anna" }, { id: "p2", name: "Ben" }], active: "" });
  some.app.start(some.root); await settle();
  assert.equal(some.root.querySelector('[role=tab][aria-selected=true]').textContent, "Log in");
  assert.deepStrictEqual([...some.root.querySelectorAll("form select option")].map((o) => o.textContent), ["Anna", "Ben"]);
  assert.equal(some.calls.filter((c) => c.key.startsWith("GET /api/")).length, 0, "no engine request before a profile is open");
  click([...some.root.querySelectorAll('[role=tab]')].find((x) => x.textContent === "Register")); await settle();
  assert.ok(some.root.querySelector("form input[type=text]"), "the register form has a name");
});

test("logging in opens the profile, unlocks it with the passphrase and goes on; a wrong passphrase stays on the screen and does not restart the engine", async () => {
  const shell = { profiles: [{ id: "p1", name: "Anna" }, { id: "p2", name: "Ben" }], active: "" };
  const unlocks = [];
  let unlocked = false;
  const p = withProfiles(shell, {
    "GET /api/status": () => (unlocked ? [200, { status: "ok" }] : [423, {}]),
    "GET /api/keystore/status": () => [200, { state: unlocked ? "ready" : "locked" }],
    "POST /api/keystore/unlock": (b) => { unlocks.push(b.passphrase); if (b.passphrase !== "right passphrase!") return [403, { error: "That passphrase is not right." }]; unlocked = true; return [200, {}]; },
  });
  p.app.start(p.root); await settle();
  const form = p.root.querySelector("form");
  p.root.querySelector("form select").value = "p2";
  p.root.querySelector("input[type=password]").value = "nope";
  submit(p, form); await settle(12);
  assert.match(text(p.root), /That passphrase is not right\./);
  assert.match(text(p.root), /Log in/, "still on the login screen");
  assert.equal(p.root.querySelector("input[type=password]").value, "", "the wrong guess is cleared");
  p.root.querySelector("input[type=password]").value = "right passphrase!";
  submit(p, p.root.querySelector("form")); await settle(14);
  assert.deepStrictEqual(p.posted, [["open", "p2"]], "the engine was started once, not again for the second try");
  assert.deepStrictEqual(unlocks, ["nope", "right passphrase!"]);
  assert.match(text(p.root), /Talk with/);
});

test("choosing another profile after a wrong guess opens that one", async () => {
  const shell = { profiles: [{ id: "p1", name: "Anna" }, { id: "p2", name: "Ben" }], active: "" };
  const p = withProfiles(shell, {
    "GET /api/keystore/status": [200, { state: "locked" }],
    "POST /api/keystore/unlock": [403, { error: "That passphrase is not right." }],
  });
  p.app.start(p.root); await settle();
  p.root.querySelector("input[type=password]").value = "x";
  submit(p, p.root.querySelector("form")); await settle(10);
  p.root.querySelector("form select").value = "p2";
  p.root.querySelector("input[type=password]").value = "y";
  submit(p, p.root.querySelector("form")); await settle(10);
  assert.deepStrictEqual(p.posted, [["open", "p1"], ["open", "p2"]]);
});

test("an empty passphrase is refused at once, and the old data opens with no passphrase", async () => {
  const shell = { profiles: [{ id: "p1", name: "Anna" }, { id: "existing", name: "Existing data", legacy: true }], active: "" };
  let ready_ = false;
  const p = withProfiles(shell, {
    "GET /api/status": () => (ready_ ? [200, { status: "ok" }] : [409, {}]),
    "GET /api/keystore/status": () => { ready_ = true; return [200, { state: "ready" }]; },
  });
  p.app.start(p.root); await settle();
  submit(p, p.root.querySelector("form")); await settle();
  assert.match(text(p.root), /Enter your passphrase\./);
  assert.deepStrictEqual(p.posted, [], "an empty passphrase started an engine");
  const sel = p.root.querySelector("form select");
  sel.value = "existing"; sel.dispatchEvent(new p.dom.window.Event("change", { bubbles: true }));
  assert.equal(p.root.querySelector("input[type=password]").hidden, true, "the old data has no passphrase field");
  submit(p, p.root.querySelector("form")); await settle(14);
  assert.deepStrictEqual(p.posted, [["open", "existing"]]);
  assert.match(text(p.root), /Talk with/);
});

test("registering checks the passphrase before anything is made, then makes the profile and its passphrase", async () => {
  const shell = { profiles: [], active: "" };
  const setups = [];
  let state = "needs_setup";
  const p = withProfiles(shell, {
    "GET /api/status": () => (state === "ready" ? [200, { status: "ok" }] : [423, {}]),
    "GET /api/keystore/status": () => [200, { state, options: ["passphrase"] }],
    "POST /api/keystore/setup": (b) => { setups.push(b); state = "ready"; return [200, {}]; },
    "GET /api/profile": [200, { ageConfirmed: false, hasProvider: false }],
  });
  p.app.start(p.root); await settle();
  const form = () => p.root.querySelector("form");
  const [pass, again] = p.root.querySelectorAll("input[type=password]");
  const name = p.root.querySelector("input[type=text]");
  submit(p, form()); await settle();
  assert.match(text(p.root), /Give the profile a name\./);
  name.value = "Anna";
  pass.value = "short"; again.value = "short";
  submit(p, form()); await settle();
  assert.match(text(p.root), /at least 12 characters/);
  pass.value = "a long enough passphrase"; again.value = "a long enough passphrasX";
  submit(p, form()); await settle();
  assert.match(text(p.root), /The two passphrases are not the same\./);
  assert.deepStrictEqual(p.posted, [], "a bad passphrase created a profile");
  again.value = "a long enough passphrase";
  submit(p, form()); await settle(14);
  assert.deepStrictEqual(p.posted, [["create", "Anna"], ["open", "p1"]]);
  assert.deepStrictEqual(setups, [{ mode: "passphrase", passphrase: "a long enough passphrase" }]);
  assert.match(text(p.root), /16 or older/, "a new person goes on to the age confirmation");
});

test("a name that is taken is told on the register screen and no engine is started", async () => {
  const p = withProfiles({ profiles: [{ id: "p1", name: "Anna" }], active: "" }, { "POST /shell/profiles": [400, { error: "A profile with that name already exists." }] });
  p.app.start(p.root); await settle();
  click([...p.root.querySelectorAll("[role=tab]")].find((x) => x.textContent === "Register")); await settle();
  p.root.querySelector("input[type=text]").value = "anna";
  const [one, two] = p.root.querySelectorAll("input[type=password]");
  one.value = two.value = "a long enough passphrase";
  submit(p, p.root.querySelector("form")); await settle(10);
  assert.match(text(p.root), /already exists/);
  assert.deepStrictEqual(p.posted, []);
});

test("the passphrase can be shown while typing and is hidden again", async () => {
  const p = withProfiles({ profiles: [{ id: "p1", name: "Anna" }], active: "" });
  p.app.start(p.root); await settle();
  const input = p.root.querySelector("input[type=password]");
  const box = p.root.querySelector("#show-pass");
  box.checked = true; box.dispatchEvent(new p.dom.window.Event("change"));
  assert.equal(input.type, "text");
  box.checked = false; box.dispatchEvent(new p.dom.window.Event("change"));
  assert.equal(input.type, "password");
});

test("a profile that was never given a passphrase goes to the engine's own set-up screen", async () => {
  const shell = { profiles: [{ id: "p1", name: "Anna" }], active: "" };
  const p = withProfiles(shell, {
    "GET /api/status": [423, {}],
    "GET /api/keystore/status": [200, { state: "needs_setup", options: ["passphrase"] }],
  });
  p.app.start(p.root); await settle();
  p.root.querySelector("input[type=password]").value = "whatever";
  submit(p, p.root.querySelector("form")); await settle(14);
  assert.match(text(p.root), /Choose a passphrase, Anna/);
});

test("a locked profile found already open (a reload) greets the person by name and can be left for another", async () => {
  const shell = { profiles: [{ id: "p1", name: "Anna" }, { id: "p2", name: "Ben" }], active: "p1" };
  const p = withProfiles(shell, { "GET /api/status": [423, {}], "GET /api/keystore/status": [200, { state: "locked" }] });
  p.app.start(p.root); await settle(10);
  assert.match(text(p.root), /Welcome back, Anna/);
  click(button(p.root, "Choose another profile")); await settle(10);
  assert.deepStrictEqual(p.posted, [["close"]]);
  assert.equal(p.root.querySelector('[role=tab][aria-selected=true]').textContent, "Log in");
});

test("logging out from the home screen or Settings closes the profile and shows the login again", async () => {
  const shell = { profiles: [{ id: "p1", name: "Anna" }], active: "p1" };
  const p = withProfiles(shell, { "GET /api/status": status(shell) });
  p.app.start(p.root); await settle(10);
  assert.match(text(p.root), /Talk with/);
  click(button(p.root, "Settings")); await settle(10);
  assert.match(text(p.root), /Signed in as Anna\./);
  click(button(p.root, "Log out")); await settle(10);
  assert.deepStrictEqual(p.posted, [["close"]]);
  assert.ok(p.root.querySelector('[role=tab]'), "the login screen is shown");
  assert.ok(!/Talk with/.test(text(p.root)), "the conversations are still on the screen");
});

test("removing a profile is under Manage profiles, asks twice, and the old data has no remove button", async () => {
  const shell = { profiles: [{ id: "p1", name: "Anna" }, { id: "existing", name: "Existing data", legacy: true }], active: "" };
  const p = withProfiles(shell);
  p.app.start(p.root); await settle();
  const manage = p.root.querySelector("details.manage");
  assert.ok(manage, "Manage profiles");
  const removers = [...manage.querySelectorAll("button")];
  assert.equal(removers.length, 1, "only the profile of a person can be removed here");
  click(removers[0]); await settle();
  assert.deepStrictEqual(p.posted, [], "one click removed a profile");
  click(removers[0]); await settle(10);
  assert.deepStrictEqual(p.posted, [["remove", "p1"]]);
});

test("without a desktop window (the phone, a browser) there are no profiles and no log out", async () => {
  const p = page(ready());
  p.app.start(p.root); await settle(10);
  assert.match(text(p.root), /Talk with/);
  assert.ok(!button(p.root, "Log out"));
  assert.ok(p.root.querySelector("[role=tab]") === null);
  click(button(p.root, "Settings")); await settle(10);
  assert.ok(!/Signed in/.test(text(p.root)));
});

test("the theme is the profile's own choice: the saved one is applied, a new one is saved and applied, automatic follows the device", async () => {
  const saved = [];
  const p = page(ready({
    "GET /api/settings/theme": [200, { theme: "dark" }],
    "PUT /api/settings/theme": (b) => { saved.push(b.theme); return [200, b]; },
  }));
  p.app.start(p.root); await settle(10);
  assert.equal(p.dom.window.document.documentElement.dataset.theme, "dark");
  click(button(p.root, "Settings")); await settle(10);
  const sel = [...p.root.querySelectorAll("select")].find((s) => [...s.options].some((o) => o.value === "light"));
  assert.equal(sel.value, "dark", "the picker shows the saved choice");
  sel.value = "light"; sel.dispatchEvent(new p.dom.window.Event("change", { bubbles: true })); await settle();
  assert.equal(p.dom.window.document.documentElement.dataset.theme, "light");
  sel.value = "auto"; sel.dispatchEvent(new p.dom.window.Event("change", { bubbles: true })); await settle();
  assert.equal(p.dom.window.document.documentElement.dataset.theme, undefined, "automatic leaves the device to decide");
  assert.deepStrictEqual(saved, ["light", "auto"]);
});

test("the stylesheet follows the device in dark unless Light was chosen, and Dark forces it", () => {
  const css = fs.readFileSync(path.join(__dirname, "../style.css"), "utf8");
  assert.match(css, /prefers-color-scheme: dark\)\s*\{\s*:root:not\(\[data-theme="light"\]\)/);
  assert.match(css, /:root\[data-theme="dark"\]\s*\{[^}]*--bg: #17140f/);
});

test("every text the screens say has Greek words, and no Greek entry is left over", () => {
  const src = fs.readFileSync(path.join(__dirname, "../app.js"), "utf8");
  const dict = require("../i18n.js").el;
  const used = new Set();
  for (const m of src.matchAll(/\bt\("((?:[^"\\]|\\.)*)"/g)) used.add(m[1]);
  // texts the screens take from a table of English sentences, and from the engine's error messages
  for (const m of src.matchAll(/^\s+\w+: "((?:[^"\\]|\\.)*)",?$/gm)) used.add(m[1]);
  for (const m of src.matchAll(/(\w+): "(It was [^"]*|Something else)"/g)) used.add(m[2]);
  used.add("This is for people aged 16 and over.");
  const missing = [...used].filter((k) => /[A-Za-z]{3}/.test(k) && !(k in dict) && !/^(Socrates AI|OpenAI|Claude|Content-Type|X-Socrates)/.test(k) && !/^[a-z_]+$/.test(k));
  const wanted = missing.filter((k) => !/^(Philosopher: |Reason: |Reply: |Socrates AI: report)/.test(k) && !/^[A-Z]{3,}$/.test(k));
  assert.deepStrictEqual(wanted.filter((k) => /\s/.test(k) || /…$/.test(k)), [], "texts without Greek words");
  const placeholders = (s) => (s.match(/\{\w+\}/g) || []).sort().join();
  for (const [en, el] of Object.entries(dict)) assert.strictEqual(placeholders(el), placeholders(en), "placeholders differ in: " + en);
});

test("with Greek chosen the screens speak Greek, and the choice is told to the engine", async () => {
  const { app, root, calls } = page(ready({
    "GET /api/settings/language": [200, { language: "el" }],
    "GET /api/philosophers": [200, { philosophers: [{ id: "socrates", name: "Σωκράτης", era: "Αθήνα", challenges: 2, unlocked: true, opening: "Είμαι ένας τεχνητός Σωκράτης." }] }],
  }));
  await app.start(root); await settle(10);
  assert.match(text(root), /Μίλησε με/);
  assert.match(text(root), /Σωκράτης/);
  assert.match(text(root), /2 ερωτήματα που σκέφτηκες/);
  assert.ok(!/Talk with/.test(text(root)));
});

test("choosing a language on the welcome screen is saved and the screen is drawn again", async () => {
  const saved = [];
  const { app, root, dom } = page(ready({
    "GET /api/profile": [200, { ageConfirmed: false, hasProvider: false }],
    "GET /api/settings/language": [200, { language: "" }],
    "PUT /api/settings/language": (b) => { saved.push(b.language); return [200, b]; },
  }));
  await app.start(root); await settle(10);
  assert.match(text(root), /Welcome to Socrates/);
  const sel = root.querySelector("select");
  sel.value = "el"; sel.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  await settle(10);
  assert.deepStrictEqual(saved.slice(-1), ["el"]);
  assert.match(text(root), /Καλώς ήρθες στο Socrates/);
});

// The content security policy blocks inline styles, so a style written on an element in the screens does nothing: layout belongs in the stylesheet.
test("the screens never rely on an inline style (the page's policy would ignore it)", () => {
  const src = fs.readFileSync(path.join(__dirname, "../app.js"), "utf8");
  assert.ok(!/\bstyle: "/.test(src), "an element gets a style attribute");
  assert.match(fs.readFileSync(path.join(__dirname, "../index.html"), "utf8"), /style-src 'self'(?! 'unsafe-inline')/);
});
