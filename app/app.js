// The screens of Socrates. Plain JavaScript, no build step, no framework. Everything that comes from the engine, a model or the user is put
// on the page with textContent, never as markup. The page talks to the same origin only (the shell adds the engine's token).
"use strict";

const CONTACT = "Hermes_creative@proton.me";
const AGE_TEXT = "This is for people aged 16 and over.";

function el(tag, props, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (k === "class") e.className = v;
    else if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else if (v === true) e.setAttribute(k, "");
    else if (v !== false && v != null) e.setAttribute(k, v);
  }
  for (const kid of kids.flat()) if (kid != null && kid !== false) e.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
  return e;
}

// A shell that cannot serve /api itself (the phone app: the engine is a separate process on the loopback address) sets window.socratesShell
// = { ready: Promise, base, token, reconnect?, saveFile? } before this script runs. The desktop window does not: it serves /api on the page's own origin.
function shellInfo() { return typeof window !== "undefined" && window.socratesShell ? window.socratesShell : null; }

async function api(method, path, body) {
  const sh = shellInfo();
  if (sh) {
    try { await sh.ready; } catch (_) { return { status: 0, data: { error: t("The engine does not answer.") } }; }
  }
  const send = () => {
    const headers = body ? { "Content-Type": "application/json" } : {};
    let url = path;
    if (sh) { url = sh.base + path; headers["X-Socrates-Token"] = sh.token; }
    return fetch(url, { method, headers, body: body ? JSON.stringify(body) : undefined });
  };
  let res;
  try {
    res = await send();
  } catch (_) {
    // The phone stops the engine when the app has been away a while; it holds a new address and token once started again. Ask for them
    // and try once more before giving up.
    if (!(sh && sh.reconnect)) return { status: 0, data: { error: t("The engine does not answer.") } };
    try { await sh.reconnect(); res = await send(); } catch (_) { return { status: 0, data: { error: t("The engine does not answer.") } }; }
  }
  let data = null;
  if (res.status !== 204) { try { data = await res.json(); } catch (_) { data = null; } }
  return { status: res.status, data: data || {} };
}

// What a failed provider call means to the person, by the kind the engine reports. Nothing was sent or saved in any of these cases.
const PROVIDER_PROBLEMS = {
  auth: "Your AI provider did not accept your key. Check it in Settings, under AI provider.",
  rate_limit: "Your AI provider says you have reached its limit for now. Try again later, or check your plan with the provider.",
  overloaded: "Your AI provider is busy right now. Please try again in a moment.",
  server: "Your AI provider had a problem on its side. Please try again in a moment.",
  model_gone: "The model you chose is no longer available. Choose the provider again in Settings to pick a current one.",
  network: "I could not reach your AI provider. Check your connection and try again.",
  empty: "The model sent back nothing. Please try again.",
};
function providerProblem(d, tail = true) {
  const base = PROVIDER_PROBLEMS[d && d.providerKind] || (d && (d.text || d.error)) || "I cannot answer right now.";
  return t(base) + (tail ? " " + t("Nothing was sent or saved.") : "");
}

const S = { philosophers: [], root: null, current: null, lang: "en" };
// The words of the screens: the English text is the key (i18n.js holds the other languages); {name}-style placeholders take their values from vars.
function t(key, vars) {
  const dict = (typeof window !== "undefined" && window.SocratesI18n && window.SocratesI18n[S.lang]) || {};
  let s = Object.prototype.hasOwnProperty.call(dict, key) ? dict[key] : key;
  if (vars) s = s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
  return s;
}
const LANGUAGES = [["en", "English"], ["el", "Ελληνικά"]];
function detectedLang() {
  const n = (typeof window !== "undefined" && window.navigator && window.navigator.language) || "";
  return /^el(\b|-)/i.test(n) ? "el" : "en";
}
// The language is kept by the engine (it chooses the words of the fixed texts and the philosophers' names); until the person has chosen,
// the language of the device is used and told to the engine once.
async function loadLang() {
  const r = await api("GET", "/api/settings/language");
  if (r.status === 200 && LANGUAGES.some(([c]) => c === r.data.language)) S.lang = r.data.language;
  else if (r.status === 200) await api("PUT", "/api/settings/language", { language: S.lang });
}
// A choice of language; the screen is drawn again in the new one.
function languagePicker(redraw) {
  const sel = el("select", { "aria-label": "Language" }, ...LANGUAGES.map(([c, n]) => el("option", { value: c, selected: c === S.lang }, n)));
  sel.addEventListener("change", async () => {
    S.lang = sel.value;
    await api("PUT", "/api/settings/language", { language: S.lang });
    const r = await api("GET", "/api/philosophers");
    if (r.status === 200) S.philosophers = r.data.philosophers || [];
    redraw();
  });
  return sel;
}

// A button that asks twice: the first click arms it, the second (within five seconds) does it. No browser dialogs, which not every shell shows.
function confirmButton(label, cls, armed, action) {
  const b = el("button", { class: cls }, label);
  let timer = null;
  b.addEventListener("click", () => {
    if (timer) { clearTimeout(timer); timer = null; b.textContent = label; return action(); }
    b.textContent = armed;
    timer = setTimeout(() => { timer = null; b.textContent = label; }, 5000);
  });
  return b;
}

// A new screen takes the focus on its heading, so that a keyboard or screen-reader user starts at the top of what is new.
function mount(...nodes) {
  S.root.replaceChildren(...nodes);
  const h = S.root.querySelector("h1");
  if (h) { h.setAttribute("tabindex", "-1"); h.focus({ preventScroll: true }); }
}
// A card that acts like a button answers Enter and Space.
function activate(fn) { return (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); fn(); } }; }
// Navigation. A screen reached from another is a step on a stack of how to draw it, so Back goes to the screen before. The page does not use
// the browser's history: a WebView ignores history steps that were made without a touch, which would swallow quick presses of the phone's
// Back button. The phone shell calls window.socratesBack(); it answers whether the page took the key (false at the first screen: the app leaves).
const nav = { stack: [] };
function show(render) { nav.stack.push(render); return render(); }
function goBack() {
  if (!nav.stack.length) return false;
  nav.stack.pop();
  const prev = nav.stack[nav.stack.length - 1];
  (prev || home)();
  return true;
}
function back() { if (!goBack()) home(); }
// Back to the first screen, forgetting every step.
function unwind() { nav.stack = []; }

function screen(title, body, actions) {
  return [el("header", {}, el("h1", {}, title), actions || null), el("main", {}, body)];
}
function errMsg(d) { return t((d && d.error) || "That did not work."); }
function errorLine(text) { return el("p", { class: "error", role: "alert" }, text || ""); }

// ---- start: the engine, the key, the age, the provider

async function boot() {
  unwind();
  mount(...screen("Socrates", el("p", { class: "muted" }, t("Starting…"))));
  for (let i = 0; i < 120; i++) {
    const r = await api("GET", "/api/status");
    if (r.status === 200) return afterReady();
    if (r.status === 423) return keyGate();
    await new Promise((ok) => setTimeout(ok, 500));
  }
  mount(...screen("Socrates", [el("p", { class: "error" }, t("The engine did not start.")), el("button", { onclick: boot }, t("Try again"))]));
}

async function keyGate() {
  const r = await api("GET", "/api/keystore/status");
  const st = r.data || {};
  const err = errorLine();
  const finish = async (res) => {
    if (res.status === 200) return boot();
    err.textContent = errMsg(res.data);
  };
  if (st.state === "locked") {
    const pass = el("input", { type: "password", autocomplete: "current-password", "aria-label": t("Passphrase") });
    return mount(...screen(t("Unlock Socrates"), el("div", { class: "card stack" },
      el("p", {}, t("Your conversations are encrypted. Enter your passphrase to open them.")), pass, err,
      el("button", { class: "primary", onclick: async () => finish(await api("POST", "/api/keystore/unlock", { passphrase: pass.value })) }, t("Unlock")))));
  }
  if (st.state === "needs_setup") {
    const pass = el("input", { type: "password", autocomplete: "new-password", "aria-label": t("New passphrase") });
    const opts = st.options || [];
    const body = el("div", { class: "card stack" }, el("p", {}, t("Socrates keeps your conversations encrypted on this device. Choose where the key is kept.")));
    if (opts.includes("keychain")) body.append(el("button", { class: "primary", onclick: async () => finish(await api("POST", "/api/keystore/setup", { mode: "keychain" })) }, t("Use the system keychain (recommended)")));
    if (opts.includes("passphrase")) body.append(el("label", {}, t("A passphrase (at least 12 characters; if you forget it, your conversations cannot be opened)")), pass,
      el("button", { class: opts.includes("keychain") ? "" : "primary", onclick: async () => finish(await api("POST", "/api/keystore/setup", { mode: "passphrase", passphrase: pass.value })) }, t("Use a passphrase")));
    if (opts.includes("file")) body.append(el("p", { class: "muted" }, t("Or keep the key in a file next to your data. Anyone who can read your files can then read your conversations.")),
      el("button", { onclick: async () => finish(await api("POST", "/api/keystore/setup", { mode: "file", confirmInsecure: true })) }, t("Keep the key in a file")));
    body.append(err);
    return mount(...screen(t("Welcome"), body));
  }
  mount(...screen("Socrates", el("div", { class: "card stack" },
    el("p", { class: "error" }, st.reason || t("The key for your conversations could not be found.")),
    el("button", { onclick: async () => { await api("POST", "/api/keystore/recheck"); boot(); } }, t("Check again")))));
}

async function afterReady() {
  await loadLang();
  const p = await api("GET", "/api/profile");
  if (p.status !== 200) return mount(...screen("Socrates", el("p", { class: "error" }, t("Your profile could not be read."))));
  if (!p.data.ageConfirmed) return ageScreen();
  if (!p.data.hasProvider) return providerScreen(true);
  const r = await api("GET", "/api/philosophers");
  S.philosophers = (r.data.philosophers || []);
  return home();
}

// The logo is a fixed file of the app, not anything an engine or a model sent.
const logo = () => el("img", { class: "logo", src: "mark.png", alt: "Socrates AI" });

function ageScreen() {
  mount(...screen(t("Welcome to Socrates"), el("div", { class: "card stack" }, logo(),
    el("p", {}, t("Socrates is a conversation with artificial philosophers, made from what was written about them. They ask hard questions and do not give verdicts.")),
    el("p", {}, t(AGE_TEXT)),
    languagePicker(ageScreen),
    el("button", { class: "primary", onclick: async () => { await api("POST", "/api/profile/age"); boot(); } }, t("I am 16 or older")))));
}

// Pages of the two providers that the key help leads to. A link is opened in the system's browser, by the shell, and only if it is
// one of these two sites over https; the page never navigates itself and never sends anything to them.
const PROVIDER_SITES = /^https:\/\/platform\.(claude|openai)\.com\//;
async function openLink(url, err) {
  if (!PROVIDER_SITES.test(url)) return;
  const shell = shellInfo();
  const bound = typeof window !== "undefined" && window.go && window.go.main && window.go.main.App; // the desktop window
  try {
    if (shell && shell.openLink) await shell.openLink(url);
    else if (bound && bound.OpenLink) await bound.OpenLink(url);
    else window.open(url, "_blank", "noopener,noreferrer");
  } catch (_) {
    err.textContent = t("The browser could not be opened. Go to {url} yourself.", { url });
  }
}

// How to get a key, for the provider chosen: three steps, each with a button that opens the right page, and the address in plain text.
function keyHelp(kind, err) {
  const claude = kind === "claude";
  const base = claude ? "https://platform.claude.com" : "https://platform.openai.com";
  const step = (text, label, path) => el("div", { class: "keystep" }, el("p", {}, text),
    el("button", { class: "outlink", onclick: () => openLink(base + path, err) }, label),
    el("div", { class: "muted" }, base.replace("https://", "") + path));
  return el("div", { class: "keyhelp" },
    el("h2", {}, t("How to get a key")),
    el("p", { class: "muted" }, t("An API key is a password that lets Socrates talk to your provider's AI. It is not the same as a ChatGPT or Claude chat subscription, which does not include one.")),
    step(t("1. Make an account on the provider's site for developers, or sign in."), t("Open the sign-in page"), "/"),
    step(t("2. Add a small amount of credit. You pay the provider directly, only for what you use. Set a spending limit in your account if it offers one."), t("Open the billing page"), claude ? "/settings/billing" : "/settings/organization/billing/overview"),
    step(t("3. Create a key, name it Socrates and copy it. It is shown only once."), t("Open the keys page"), claude ? "/settings/keys" : "/api-keys"),
    el("p", {}, t("4. Come back here, paste the key above and press Save and test.")));
}

function providerScreen(first, back) {
  const err = errorLine();
  const provider = el("select", { "aria-label": t("Provider") }, el("option", { value: "claude" }, "Claude (Anthropic)"), el("option", { value: "openai" }, "OpenAI"));
  const key = el("input", { type: "password", autocomplete: "off", placeholder: t("Paste your API key"), "aria-label": t("API key") });
  const keyBox = el("div", {}, el("label", {}, t("Your API key")), key, el("p", { class: "muted" }, t("The key stays on this device, encrypted. It is tested once, then used only to talk to your provider.")));
  const help = el("div", {}, keyHelp(provider.value, err));
  provider.addEventListener("change", () => { help.replaceChildren(keyHelp(provider.value, err)); });
  const save = el("button", { class: "primary" }, t("Save and test"));
  save.addEventListener("click", async () => {
    err.textContent = ""; save.disabled = true; save.textContent = t("Testing…");
    const r = await api("PUT", "/api/settings/llm", { provider: provider.value, apiKey: key.value });
    save.disabled = false; save.textContent = t("Save and test");
    if (r.status === 200) { key.value = ""; return boot(); }
    err.textContent = errMsg(r.data);
  });
  mount(...screen(first ? t("Connect an AI provider") : t("AI provider"), el("div", {}, el("div", { class: "card stack" },
    el("p", {}, t("Socrates uses your own account with an AI provider. You pay the provider directly; Socrates has no server and sees nothing.")),
    el("label", {}, t("Provider")), provider, keyBox, err, save), el("div", { class: "card" }, help)), back ? el("button", { class: "link", onclick: back }, t("Back")) : null));
}

// ---- home: the philosophers and the conversations

async function home() {
  const [c] = await Promise.all([api("GET", "/api/conversations")]);
  const byId = Object.fromEntries(S.philosophers.map((p) => [p.id, p]));
  const body = el("div", {});
  if (S.notice) { body.append(errorLine(S.notice)); S.notice = ""; }
  body.append(el("h2", { class: "muted" }, t("Talk with")));
  for (const p of S.philosophers) {
    if (p.unlocked === false) {
      const who = (p.needs || []).map((n) => t("{name} ({done} so far)", { name: n.name, done: n.done })).join(t(" and "));
      const need = p.needs && p.needs.length ? t(" Think {n} questions through with {who} to meet him.", { n: p.needs[0].required, who }) : "";
      body.append(el("div", { class: "card locked", "aria-disabled": "true" }, el("h2", {}, p.name), el("div", { class: "muted" }, p.era + t(" · Locked.") + need)));
      continue;
    }
    const done = p.challenges > 0 ? (p.challenges === 1 ? t(" · 1 question thought through") : t(" · {n} questions thought through", { n: p.challenges })) : "";
    body.append(el("div", { class: "card click", role: "button", tabindex: "0", onclick: () => startWith(p), onkeydown: activate(() => startWith(p)) },
      el("h2", {}, p.name), el("div", { class: "muted" }, p.era + done)));
  }
  const list = c.data.conversations || [];
  if (list.length) {
    body.append(el("h2", { class: "muted" }, t("Earlier conversations")));
    for (const k of list) {
      const who = (byId[k.philosopherId] || {}).name || k.philosopherId;
      body.append(el("div", { class: "card click", role: "button", tabindex: "0", onclick: () => show(() => openChat(k.id)), onkeydown: activate(() => show(() => openChat(k.id))) },
        el("h2", {}, k.title || t("New conversation")), el("div", { class: "muted" }, who)));
    }
  }
  mount(...screen("Socrates", body, el("button", { class: "link", onclick: () => show(settings) }, t("Settings"))));
}

async function startWith(p) {
  const r = await api("POST", "/api/conversations", { philosopherId: p.id });
  if (r.status === 201) return show(() => openChat(r.data.id));
  S.notice = r.data.error || t("The conversation could not be started.");
  home();
}

// ---- the chat

async function openChat(id) {
  const r = await api("GET", "/api/conversations/" + encodeURIComponent(id));
  if (r.status !== 200) return home();
  const conv = r.data.conversation, msgs = r.data.messages || [];
  const who = S.philosophers.find((p) => p.id === conv.philosopherId) || { name: conv.philosopherId, opening: "" };
  const list = el("div", { class: "msgs", "aria-live": "polite" });
  const main = el("main", {}, list);
  if (!msgs.length && who.opening) list.append(el("div", { class: "msg opening" }, who.opening));
const REASONS = { harmful: "It was harmful", hateful: "It was hateful or insulting", sexual: "It was sexual", untrue: "It was untrue about the real world", other: "Something else" };
  // Reporting takes the reply out of the conversation (nothing is sent anywhere); the person may keep a copy of the report to send to the developer.
  const report = (m, bubble) => {
    const choose = el("div", { class: "help" }, el("div", { class: "muted" }, t("Why are you reporting this reply?")),
      ...Object.keys(REASONS).map((k) => el("button", { class: "chip", type: "button", onclick: () => send(k) }, t(REASONS[k]))),
      el("button", { class: "link", type: "button", onclick: () => choose.remove() }, t("Cancel")));
    const send = async (category) => {
      choose.replaceChildren(el("span", { class: "muted" }, t("One moment…")));
      const res = await api("POST", "/api/conversations/" + encodeURIComponent(id) + "/messages/" + encodeURIComponent(m.id) + "/report", { category });
      if (res.status !== 200) { choose.replaceChildren(errorLine(errMsg(res.data))); return; }
      bubble.className = "msg notice"; bubble.replaceChildren(res.data.marker);
      const copy = (withReply) => {
        const text = ["Socrates AI: report of a reply", "Philosopher: " + who.name, "Reason: " + REASONS[category]].concat(withReply ? ["Reply: " + res.data.reply] : []).join("\n");
        const done = (msg) => keep.replaceChildren(el("span", { class: "muted" }, msg));
        const clip = window.navigator && window.navigator.clipboard;
        if (clip && clip.writeText) clip.writeText(text).then(() => done(t("Copied. Send it to {address} as you like; nothing was sent from here.", { address: CONTACT })), () => manual(text));
        else manual(text);
      };
      const manual = (text) => keep.replaceChildren(el("div", { class: "muted" }, t("Copy this yourself:")), el("textarea", { readonly: "", rows: "4", "aria-label": t("Report text") }, text));
      const keep = el("div", { class: "help" }, el("div", { class: "muted" }, t("Keep a copy of the report to send to the developer?")),
        el("div", { class: "row" }, el("button", { class: "chip", type: "button", onclick: () => copy(true) }, t("Copy with the reply")), el("button", { class: "chip", type: "button", onclick: () => copy(false) }, t("Copy without it")), el("button", { class: "link", type: "button", onclick: () => keep.remove() }, t("No, thanks"))));
      choose.replaceWith(keep);
    };
    bubble.after(choose);
  };
  const add = (m) => {
    if (m.role === "philosopher" && m.id) {
      const bubble = el("div", { class: "msg philosopher" }, m.text, " ", el("button", { class: "link small", type: "button", "aria-label": t("Report this reply"), onclick: () => { if (!bubble.nextSibling || !bubble.nextSibling.classList || !bubble.nextSibling.classList.contains("help")) report(m, bubble); } }, t("Report")));
      list.append(bubble);
      return;
    }
    list.append(el("div", { class: "msg " + (m.role === "user" ? "user" : m.role === "marker" ? "notice" : "philosopher") }, m.text));
  };
  msgs.forEach(add);
  const input = el("textarea", { rows: "1", placeholder: t("Say something…"), "aria-label": t("Your message"), maxlength: "6000" });
  const helpBox = el("div", { class: "help", hidden: true });
  const helpBtn = el("button", { class: "link", type: "button" }, t("Help me answer"));
  let offered = [];       // everything suggested so far in this chat, so that asking again gives different ones
  let fromSuggestion = false;
  const canHelp = () => {
    const k = [...list.children].filter((c) => !c.classList.contains("opening"));
    const l = k[k.length - 1];
    return !!l && l.classList.contains("philosopher") && !l.classList.contains("typing");
  };
  // A challenge is a fundamental topic the philosopher poses; the person takes it up or leaves it. `offer` is what the engine says waits.
  let offer = r.data.offer || null;
  const challengeBar = el("div", { class: "help", hidden: true });
  const challengeBtn = el("button", { class: "link", type: "button" }, t("Put a question to me"));
  const canChallenge = () => {
    const k = [...list.children].filter((c) => !c.classList.contains("opening"));
    const l = k[k.length - 1];
    return !l || (l.classList.contains("philosopher") && !l.classList.contains("typing"));
  };
  const turnProblem = (res) => (res.status === 503 ? providerProblem(res.data) : errMsg(res.data));
  const challengeStep = async (path, thinking) => {
    err.textContent = ""; challengeBtn.disabled = true;
    challengeBar.hidden = false; challengeBar.replaceChildren(el("span", { class: "muted" }, thinking));
    const res = await api("POST", "/api/conversations/" + encodeURIComponent(id) + path);
    challengeBtn.disabled = false;
    if (res.status === 200) {
      (res.data.added || []).forEach(add);
      if (res.data.milestone) add({ role: "marker", text: res.data.milestone });
      offer = res.data.offer || null;
    } else {
      err.textContent = turnProblem(res);
      if (res.data.kind === "no_offer") offer = null;
    }
    refreshHelp(); main.scrollTop = main.scrollHeight;
  };
  const leaveChallenge = async () => {
    const res = await api("POST", "/api/conversations/" + encodeURIComponent(id) + "/challenge/decline");
    if (res.status === 204 || res.data.kind === "no_offer") offer = null; else err.textContent = turnProblem(res);
    refreshHelp();
  };
  challengeBtn.addEventListener("click", () => challengeStep("/challenge", t("{name} is thinking of a question…", { name: who.name })));
  const showOffer = () => {
    const pending = offer && offer.state === "pending", ready = offer && offer.state === "ready";
    challengeBtn.hidden = pending || !canChallenge();
    if (pending) {
      challengeBar.hidden = false;
      challengeBar.replaceChildren(el("div", { class: "muted" }, t("Take up {name}’s question?", { name: who.name })),
        el("div", { class: "row" }, el("button", { class: "primary", type: "button", onclick: () => challengeStep("/challenge/accept", t("One moment…")) }, t("Take it")),
          el("button", { class: "link", type: "button", onclick: leaveChallenge }, t("Not now"))));
    } else if (ready && canChallenge()) {
      challengeBar.hidden = false;
      challengeBar.replaceChildren(el("div", { class: "muted" }, t("{name} has a question for you.", { name: who.name })),
        el("div", { class: "row" }, el("button", { class: "link", type: "button", onclick: () => challengeStep("/challenge", t("{name} is thinking of a question…", { name: who.name })) }, t("Hear it"))));
    } else { challengeBar.replaceChildren(); challengeBar.hidden = true; }
  };
  // Deleting a conversation asks, when the philosopher learnt something in it, whether he should forget that too (forgetting is the default).
  const deleteBox = el("div", { class: "help", hidden: true });
  const removeConversation = async (keep) => {
    await api("DELETE", "/api/conversations/" + encodeURIComponent(id) + (keep ? "?keep=memory" : ""));
    unwind(); home();
  };
  const hideDelete = () => { deleteBox.replaceChildren(); deleteBox.hidden = true; };
  const showDelete = async () => {
    const fresh = await api("GET", "/api/conversations/" + encodeURIComponent(id)); // what was learnt may have changed since the chat opened
    const learned = fresh.status === 200 ? (fresh.data.learned || 0) : 0;
    deleteBox.hidden = false;
    deleteBox.replaceChildren(el("div", { class: "muted" }, learned > 0 ? t("Also forget what {name} learned in this conversation?", { name: who.name }) : t("Delete this conversation?")),
      el("div", { class: "row" },
        el("button", { class: "danger", type: "button", onclick: () => removeConversation(false) }, learned > 0 ? t("Delete and forget it") : t("Delete")),
        learned > 0 ? el("button", { class: "link", type: "button", onclick: () => removeConversation(true) }, t("Delete, but let him keep it")) : null,
        el("button", { class: "link", type: "button", onclick: hideDelete }, t("Cancel"))));
  };
  const refreshHelp = () => { helpBtn.hidden = !canHelp(); if (helpBtn.hidden) helpBox.hidden = true; showOffer(); };
  const showSuggestions = async () => {
    helpBtn.disabled = true; helpBox.hidden = false;
    helpBox.replaceChildren(el("span", { class: "muted" }, t("Thinking of some starting points…")));
    const r = await api("POST", "/api/conversations/" + encodeURIComponent(id) + "/suggestions", { exclude: offered });
    helpBtn.disabled = false;
    if (r.status !== 200) {
      helpBox.replaceChildren(el("span", { class: "error" }, r.status === 503 ? providerProblem(r.data, false) : (errMsg(r.data))), el("button", { class: "link", type: "button", onclick: () => { helpBox.hidden = true; } }, t("Close")));
      return;
    }
    const items = r.data.suggestions || [];
    offered = offered.concat(items);
    const chips = items.map((t) => el("button", { class: "chip", type: "button", onclick: () => { input.value = t; fromSuggestion = true; helpBox.hidden = true; input.focus(); } }, t));
    helpBox.replaceChildren(
      el("div", { class: "muted" }, items.length ? t("Pick one to start from, change it as you like, or write your own.") : t("I have no more starting points. Try putting it in your own words.")),
      ...chips,
      el("div", { class: "row" }, items.length ? el("button", { class: "link", type: "button", onclick: showSuggestions }, t("Different ones")) : null, el("button", { class: "link", type: "button", onclick: () => { helpBox.hidden = true; } }, t("Close"))));
  };
  helpBtn.addEventListener("click", showSuggestions);
  input.addEventListener("input", () => { if (!input.value.trim()) fromSuggestion = false; });
  const send = el("button", { class: "primary", type: "submit" }, t("Send"));
  const err = errorLine();
  let busy = false;
  const submit = async (ev) => {
    ev.preventDefault();
    const text = input.value.trim();
    if (!text || busy) return;
    busy = true; send.disabled = true; err.textContent = "";
    const mine = el("div", { class: "msg user" }, text);
    const typing = el("div", { class: "msg philosopher typing" }, t("{name} is thinking…", { name: who.name }));
    list.append(mine, typing); input.value = ""; main.scrollTop = main.scrollHeight;
    const flag = fromSuggestion; fromSuggestion = false; helpBox.hidden = true;
    const res = await api("POST", "/api/conversations/" + encodeURIComponent(id) + "/messages", flag ? { text, suggested: true } : { text });
    typing.remove();
    busy = false; send.disabled = false;
    const d = res.data;
    if (res.status === 200 && d.outcome === "ended") { mine.remove(); return ended(d.text); }
    if (res.status === 200) {
      // A refused message is not kept, so the page does not keep it either; the fixed text stands in its place.
      const kept = (d.added || []).some((m) => m.role === "user");
      if (!kept) mine.remove();
      (d.added || []).forEach((m) => { if (m.role !== "user") add(m); });
      if (d.outcome === "fixed" && !(d.added || []).some((m) => m.role === "marker")) add({ role: "marker", text: d.text });
      if (d.milestone) add({ role: "marker", text: d.milestone });
      offer = d.offer || (d.outcome === "replied" ? null : offer);
    } else {
      mine.remove(); input.value = text; fromSuggestion = flag;
      if (res.status === 503) err.textContent = providerProblem(d);
      else if (d.kind === "age") err.textContent = t(AGE_TEXT);
      else if (d.kind === "no_provider") err.textContent = t("Set up an AI provider in Settings first.");
      else err.textContent = errMsg(d);
    }
    refreshHelp(); main.scrollTop = main.scrollHeight; input.focus();
  };
  input.addEventListener("keydown", (e) => { if (e.key === "Enter" && !e.shiftKey) submit(e); });
  const form = el("form", { class: "composer", onsubmit: submit }, input, send);
  mount(el("header", {}, el("button", { class: "link", onclick: back }, t("← Back")), el("h1", {}, who.name),
    el("button", { class: "link", onclick: () => show(() => memory(who)) }, t("Memory")),
    el("button", { class: "link", onclick: showDelete }, t("Delete"))),
    main, err, helpBox, challengeBar, deleteBox, el("div", { class: "helpbar" }, helpBtn, challengeBtn), form);
  refreshHelp(); main.scrollTop = main.scrollHeight; input.focus();
}

// ---- what a philosopher remembers about the person

async function memory(who) {
  const r = await api("GET", "/api/ledger?philosopherId=" + encodeURIComponent(who.id));
  const entries = r.data.entries || [];
  const body = el("div", { class: "stack" });
  const err = errorLine(r.status === 200 ? "" : (errMsg(r.data)));
  body.append(el("p", { class: "muted" }, t("{name} keeps short notes of what you tell him, with your own words. They are stored encrypted on this device, are used only in your conversations with {name}, and are deleted with the conversation they came from. You can delete any of them.", { name: who.name })), err);
  const remove = async (id) => {
    const d = await api("DELETE", "/api/ledger/" + encodeURIComponent(id));
    if (d.status === 204 || d.status === 404) return memory(who);
    err.textContent = errMsg(d.data);
  };
  const label = { claim: t("Something you said"), term: t("A word you used"), tension: t("Things that did not fit together"), fact: t("Something you told about yourself") };
  if (!entries.length) body.append(el("div", { class: "card" }, el("p", { class: "muted" }, t("{name} has not noted anything about you yet.", { name: who.name }))));
  for (const e of entries) {
    body.append(el("div", { class: "card stack" },
      el("div", { class: "muted" }, label[e.kind] || e.kind, e.suggested ? t(" · from a wording you picked among suggestions") : ""),
      el("div", {}, e.kind === "term" && e.detail ? `${e.text}: ${e.detail}` : e.text),
      e.kind !== "tension" ? el("div", { class: "muted" }, t("Your words: “{quote}”", { quote: e.quote })) : null,
      confirmButton(t("Delete"), "link", t("Sure?"), () => remove(e.id))));
  }
  if (entries.length) body.append(confirmButton(t("Forget everything {name} knows about me", { name: who.name }), "danger", t("Click again to forget it all"), async () => {
    const d = await api("DELETE", "/api/ledger?philosopherId=" + encodeURIComponent(who.id));
    if (d.status === 204) return memory(who);
    err.textContent = errMsg(d.data);
  }));
  mount(...screen(t("What {name} remembers", { name: who.name }), body, el("button", { class: "link", onclick: back }, t("Back"))));
}

function ended(text) {
  mount(...screen("Socrates", el("div", { class: "card stack" }, el("p", {}, text), el("button", { class: "primary", onclick: () => { unwind(); home(); } }, t("OK")))));
}

// ---- settings

async function settings() {
  const err = errorLine();
  const body = el("div", { class: "stack" });
  const p = await api("GET", "/api/settings/llm");
  body.append(el("div", { class: "card stack" }, el("h2", {}, t("Language")), languagePicker(settings)));
  body.append(el("div", { class: "card stack" }, el("h2", {}, t("AI provider")),
    el("p", { class: "muted" }, p.data.provider ? t("{provider}, models {fast} and {reply}", { provider: p.data.provider === "claude" ? "Claude (Anthropic)" : p.data.provider === "openai" ? "OpenAI" : p.data.provider, fast: p.data.fastModel, reply: p.data.replyModel }) : t("Not set up.")),
    el("button", { onclick: () => show(() => providerScreen(false, back)) }, t("Change provider or key"))));
  body.append(el("div", { class: "card stack" }, el("h2", {}, t("Your data")),
    el("p", { class: "muted" }, t("Conversations are stored encrypted on this device only. An export holds your conversations and never your key.")),
    el("div", { class: "row" },
      el("button", { onclick: exportData }, t("Export conversations")),
      el("button", { onclick: () => importInput.click() }, t("Import"))),
    err));
  const importInput = el("input", { type: "file", accept: "application/json", hidden: true, onchange: async (e) => {
    const f = e.target.files[0]; if (!f) return;
    let json; try { json = JSON.parse(await f.text()); } catch (_) { err.textContent = t("That file is not readable."); return; }
    const r = await api("POST", "/api/profile/import", json);
    err.textContent = r.status === 200 ? "" : (errMsg(r.data));
    if (r.status === 200) home();
  } });
  body.append(importInput);
  const mem = await api("GET", "/api/settings/memory");
  const box = el("input", { type: "checkbox", id: "memory-on", style: "width:auto" });
  box.checked = mem.data.enabled !== false;
  box.addEventListener("change", async () => {
    const r = await api("PUT", "/api/settings/memory", { enabled: box.checked });
    if (r.status !== 200) { box.checked = !box.checked; err.textContent = errMsg(r.data); }
  });
  body.append(el("div", { class: "card stack" }, el("h2", {}, t("Memory")),
    el("div", { class: "row" }, box, el("label", { for: "memory-on", style: "margin:0" }, t("The philosophers remember what I tell them"))),
    el("p", { class: "muted" }, t("Each keeps short notes with your own words, only for himself, stored encrypted on this device. Turning this off stops new notes and stops him using the old ones; it does not delete them. You can see and delete them from the Memory button inside a conversation."))));
  const st = await api("GET", "/api/status");
  body.append(el("div", { class: "card stack" }, el("h2", {}, t("About")), logo(),
    el("p", { class: "muted" }, t("The philosophers are artificial characters made from what was written about them. They are not real people, and they are not a source of medical, legal or other professional advice.")),
    el("p", { class: "muted" }, t("If you are thinking of hurting yourself, please call your local emergency number or a crisis line, or tell someone you trust.")),
    el("p", { class: "muted" }, t("For people aged 16 and over. ") + (st.data.version ? t("Version {v}.", { v: st.data.version }) : "")),
    el("p", { class: "muted" }, t("Problems, questions and reports: {address} (Hermes Software).", { address: CONTACT }))));
  body.append(el("div", { class: "card stack" }, el("h2", {}, t("Delete everything")),
    el("p", { class: "muted" }, t("Removes every conversation, your provider key and your settings from this device. It cannot be undone.")),
    confirmButton(t("Delete everything"), "danger", t("Click again to delete everything"), async () => {
      const r = await api("DELETE", "/api/profile", { confirm: "delete" });
      if (r.status === 204) boot(); else err.textContent = errMsg(r.data);
    })));
  mount(...screen(t("Settings"), body, el("button", { class: "link", onclick: back }, t("Done"))));
}

async function exportData() {
  const r = await api("GET", "/api/profile/export");
  if (r.status !== 200) return;
  const text = JSON.stringify(r.data, null, 2);
  const sh = shellInfo();
  if (sh && sh.saveFile) { await sh.saveFile("socrates-export.json", text); return; } // the phone: the system's "save as" picker
  const a = el("a", { href: URL.createObjectURL(new Blob([text], { type: "application/json" })), download: "socrates-export.json" });
  document.body.append(a); a.click(); a.remove();
}

function start(root) { S.root = root || document.getElementById("root"); S.lang = detectedLang(); window.socratesBack = goBack; return boot(); }
if (typeof module !== "undefined") module.exports = { start, el, api, S };
else window.addEventListener("DOMContentLoaded", () => start());
