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
