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

// ---- icons: Moly's set, one stroke style, drawn inline so that they follow the text colour (no emoji, no image files)
const SVG_NS = "http://www.w3.org/2000/svg";
const ICONS = {
  plus: [["path", "M12 5v14"], ["path", "M5 12h14"]],
  back: [["path", "M19 12H5"], ["path", "m11 18-6-6 6-6"]],
  logout: [["path", "M9 21H5V3h4"], ["path", "M16 17l5-5-5-5"], ["path", "M21 12H9"]],
  insight: [["path", "M9 18h6"], ["path", "M10 21h4"], ["path", "M12 3a6 6 0 0 0-3.6 10.8c.6.5 1 1.2 1 2V16h5.2v-.2c0-.8.4-1.5 1-2A6 6 0 0 0 12 3Z"]],
  progress: [["path", "M5 20V11"], ["path", "M12 20V4"], ["path", "M19 20v-6"]],
  settings: [["circle", { cx: 12, cy: 12, r: 3 }], ["path", "M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3h0a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9v0a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z"]],
  send: [["path", "M12 19V5"], ["path", "m5 12 7-7 7 7"]],
  stop: [["rect", { x: 6.5, y: 6.5, width: 11, height: 11, rx: 2 }]],
  spinner: [["path", "M12 3a9 9 0 1 0 9 9"]],
  copy: [["rect", { x: 8.5, y: 8.5, width: 11, height: 11, rx: 2 }], ["path", "M15.5 8.5V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v7.5a2 2 0 0 0 2 2h2.5"]],
  trash: [["path", "M4 7h16"], ["path", "M9 7V4.5h6V7"], ["path", "M6.5 7 7.4 20h9.2L17.5 7"]],
  chevron: [["path", "m9 5 7 7-7 7"]],
};
// Copies a reply to the clipboard (where the page may use it); the tooltip says so for a moment.
function copyButton(text) {
  const b = iconBtn("copy", t("Copy this reply"), async () => {
    try { await navigator.clipboard.writeText(text); b.title = t("Copied."); setTimeout(() => { b.title = t("Copy this reply"); }, 1500); } catch (_) { /* no clipboard here */ }
  }, { class: "link icon-btn copy-btn" });
  return b;
}
function icon(name, cls) {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("class", "icon" + (cls ? " " + cls : ""));
  for (const [k, v] of Object.entries({ viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", "stroke-width": "1.7", "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true", focusable: "false" })) svg.setAttribute(k, v);
  for (const [tag, spec] of ICONS[name] || []) {
    const n = document.createElementNS(SVG_NS, tag);
    if (typeof spec === "string") n.setAttribute("d", spec); else for (const [k, v] of Object.entries(spec)) n.setAttribute(k, v);
    svg.append(n);
  }
  return svg;
}
// A button that shows an icon and, for screen readers and the tooltip, says what it does in words.
function iconBtn(name, label, onclick, extra) {
  return el("button", Object.assign({ class: "link icon-btn", type: "button", title: label, onclick }, extra || {}), icon(name), el("span", { class: "sr" }, label));
}
// The back button at the left of the top bar of a screen opened from another one.
const backButton = (label, go) => el("button", { class: "link btn-sm", type: "button", "data-back": "1", onclick: go || back }, icon("back"), label);

// A shell that cannot serve /api itself (the phone app: the engine is a separate process on the loopback address) sets window.socratesShell
// = { ready: Promise, base, token, reconnect?, saveFile? } before this script runs. The desktop window does not: it serves /api on the page's own origin.
function shellInfo() { return typeof window !== "undefined" && window.socratesShell ? window.socratesShell : null; }

async function api(method, path, body, signal) {
  const sh = shellInfo();
  if (sh) {
    try { await sh.ready; } catch (_) { return { status: 0, data: { error: t("The engine does not answer.") } }; }
  }
  const send = () => {
    const headers = body ? { "Content-Type": "application/json" } : {};
    let url = path;
    if (sh) { url = sh.base + path; headers["X-Socrates-Token"] = sh.token; }
    return fetch(url, { method, headers, signal, body: body ? JSON.stringify(body) : undefined });
  };
  let res;
  try {
    res = await send();
  } catch (_) {
    if (signal && signal.aborted) return { status: 0, data: { aborted: true } }; // the person pressed Stop
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

const S = { philosophers: [], root: null, current: null, lang: "en", profiles: false, profile: null };
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
// ---- the card at the foot of every screen
//
// A small card with a message from the project or from a sponsor, as Moly's: labelled "Sponsored" (the project's own too), static (text and
// links only), one click hides it until the app is opened again, and it has its own area below everything, never between a philosopher's
// replies. The messages built in here show until the engine brings the project's data file (site/cards.json, fetched by the engine at most
// once a day with nothing about the person), which lets them change without a release. The links open in the system's browser.
const CARD_KEY = "socrates_card_hidden";
const BUILTIN_CARDS = [{
  id: "about-socrates", kind: "socrates", active: true, builtin: true, marks: ["hermes"],
  title: "Socrates AI is made by Hermes Software",
  text: "A free beta, made by one person. If you would like it to go on, please sponsor.",
  links: [{ text: "Sponsor", url: "https://github.com/sponsors/Nireus79", primary: true }, { text: "GitHub", url: "https://github.com/Nireus79/Socrates-ai" }, { text: "Hermes Software", url: "https://hermesoftware.wordpress.com/" }],
}];
const cardHidden = () => { if (S.cardGone) return true; try { return window.sessionStorage.getItem(CARD_KEY) === "1"; } catch (_) { return false; } };
const hideCard = () => { S.cardGone = true; try { window.sessionStorage.setItem(CARD_KEY, "1"); } catch (_) { /* hidden until the next screen is drawn */ } };
const safeHttps = (u) => { try { const p = new URL(u); return p.protocol === "https:" && !p.username && !p.port; } catch (_) { return false; } };
// One of the active entries, chosen by the day only, so that the card changes slowly and depends on nothing about the person.
function pickCard(entries, now) {
  const active = (entries || []).filter((e) => e && e.active);
  return active.length ? active[Math.floor((now || Date.now()) / 86400000) % active.length] : null;
}
async function openCardLink(url, err) {
  if (!safeHttps(url)) return;
  const shell = shellInfo();
  const bound = typeof window !== "undefined" && window.go && window.go.main && window.go.main.App;
  try {
    if (shell && shell.openSponsorLink) await shell.openSponsorLink(url);
    else if (bound && bound.OpenSponsorLink) await bound.OpenSponsorLink(url);
    else window.open(url, "_blank", "noopener,noreferrer");
  } catch (_) {
    err.textContent = t("The browser could not be opened. Go to {url} yourself.", { url });
  }
}
function cardNode() {
  if (cardHidden()) return null;
  const e = pickCard(S.cardFeed && S.cardFeed.length ? S.cardFeed : BUILTIN_CARDS);
  if (!e) return null;
  const say = (x) => (e.builtin ? t(x) : x); // the built-in words are translated; a message from the file is shown as written
  const err = errorLine();
  const links = (e.links || []).filter((l) => safeHttps(l.url));
  const node = el("aside", { class: "card-area", "aria-label": t("Sponsored") },
    el("div", { class: "card-head" }, el("span", { class: "card-label" }, t("Sponsored")),
      el("button", { class: "link", type: "button", "aria-label": t("Hide this card"), title: t("Hide this card until you open Socrates again"), onclick: () => { hideCard(); node.remove(); } }, t("Hide"))),
    (e.marks || []).includes("hermes") ? el("span", { class: "hermes", role: "img", "aria-label": "Hermes Software" }) : null,
    el("strong", { class: "card-title" }, say(e.title)), el("p", { class: "card-text" }, say(e.text)),
    links.length ? el("div", { class: "row" }, ...links.map((l) => el("button", { type: "button", class: l.primary ? "primary small-btn" : "small-btn", onclick: () => openCardLink(l.url, err) }, say(l.text)))) : null, err);
  return node;
}
// The project's messages, once per session (the engine fetches the file at most once a day); nothing is asked while the card is hidden.
async function refreshCards() {
  if (cardHidden() || S.cardFeed || S.cardsBusy) return;
  S.cardsBusy = true;
  try {
    const r = await api("GET", "/api/cards");
    if (r.status === 200) {
      S.cardFeed = (r.data.entries || []).filter((e) => e && e.active);
      const old = S.root.querySelector(".card-area");
      const fresh = cardNode();
      if (old && fresh) old.replaceWith(fresh);
    }
  } finally { S.cardsBusy = false; }
}

function mount(...nodes) {
  S.root.replaceChildren(...nodes, ...(S.noCard ? [] : [cardNode()].filter(Boolean)));
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

// The mark, small, at the left of every header (a decoration: it has no name for a screen reader).
const headerLogo = () => el("img", { class: "hlogo", src: "mark.png", alt: "" });
function screen(title, body, actions) {
  const left = actions && actions.getAttribute && actions.getAttribute("data-back") ? actions : null; // a back button comes first, as in Moly
  return [el("header", {}, left, headerLogo(), el("h1", {}, title), left ? null : actions || null), el("main", {}, body)];
}
function errMsg(d) { return t((d && d.error) || "That did not work."); }
function errorLine(text) { return el("p", { class: "error", role: "alert" }, text || ""); }

// ---- theme

// "auto" follows the device; "light" and "dark" are the person's own choice. The choice is kept by the engine with the profile, and a copy
// is kept in the page so that the screens before the login (the profiles, the passphrase) already have it.
const THEMES = [["auto", "Automatic"], ["light", "Light"], ["dark", "Dark"]];
function applyTheme(v) {
  const root = document.documentElement;
  if (v === "light" || v === "dark") root.dataset.theme = v; else delete root.dataset.theme;
  try { localStorage.setItem("theme", v === "light" || v === "dark" ? v : "auto"); } catch (_) { /* no storage: the choice lasts until the page closes */ }
}
// The desktop window asks the system whether it is dark or light and tells the page (the web view does not always follow it). The answer goes
// in data-device, which only "Automatic" listens to: the person's own choice (data-theme) always wins. Without the window, or when the
// system gives no answer, the browser's own setting decides, as before.
async function followDevice() {
  const bound = typeof window !== "undefined" && window.go && window.go.main && window.go.main.App;
  if (!bound || !bound.SystemTheme) return;
  try {
    const v = await bound.SystemTheme();
    if (v === "dark" || v === "light") document.documentElement.dataset.device = v; else delete document.documentElement.dataset.device;
  } catch (_) { /* the browser's own setting decides */ }
}
function rememberedTheme() { try { return localStorage.getItem("theme") || "auto"; } catch (_) { return "auto"; } }
async function loadTheme() {
  const r = await api("GET", "/api/settings/theme");
  if (r.status === 200) applyTheme(r.data.theme);
}
function themePicker() {
  const now = document.documentElement.dataset.theme || "auto";
  const group = el("div", { class: "segmented", role: "radiogroup", "aria-label": t("Theme") });
  for (const [c, n] of THEMES) {
    const b = el("button", { type: "button", role: "radio", "aria-checked": String(c === now), class: c === now ? "active" : "" }, t(n));
    b.addEventListener("click", async () => {
      applyTheme(c);
      for (const x of group.children) { x.classList.toggle("active", x === b); x.setAttribute("aria-checked", String(x === b)); }
      await api("PUT", "/api/settings/theme", { theme: c });
    });
    group.append(b);
  }
  return group;
}

// ---- profiles (the desktop app: several people on one computer)

// The desktop window answers /shell/profiles; the phone app and a plain browser do not, and then there are no profiles.
async function desktopProfiles() {
  if (shellInfo()) return null;
  try {
    const r = await fetch("/shell/profiles");
    return r.status === 200 ? await r.json() : null;
  } catch (_) { return null; }
}
async function shellPost(path, body) {
  try {
    const r = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body || {}) });
    return { status: r.status, data: await r.json() };
  } catch (_) { return { status: 0, data: { error: "The engine does not answer." } }; }
}
async function logOut() {
  await shellPost("/shell/close");
  S.profile = null; S.opened = null; S.philosophers = [];
  boot();
}
// Waits until the engine of the open profile answers about its key (it needs a moment after it starts).
async function keyStatus() {
  for (let i = 0; i < 60; i++) {
    const r = await api("GET", "/api/keystore/status");
    if (r.status === 200) return r.data || {};
    if ((await api("GET", "/api/status")).status === 200) return { state: "ready" }; // the key was found by itself (the keychain)
    await new Promise((ok) => setTimeout(ok, 250));
  }
  return null;
}
const MIN_PASSPHRASE = 12;

// A profile without a passphrase keeps its key the way the old data did: in the system keychain where there is one, otherwise in a file
// next to its data. The data here is not sensitive and several people may share the computer; a person who wants more protects the profile.
async function setupOpenProfile(st) {
  if (!st || st.state !== "needs_setup") return { status: 200 };
  const opts = st.options || [];
  return api("POST", "/api/keystore/setup", opts.includes("keychain") ? { mode: "keychain" } : { mode: "file", confirmInsecure: true });
}

// The first screen of the desktop app: log in to a profile on this computer, or register a new one. There is no server; a profile is a
// folder with its own passphrase, and nothing here can be reset, which the register tab says.
function authScreen(info, tab) {
  const profiles = info.profiles || [];
  tab = tab || (profiles.length ? "login" : "register");
  const err = errorLine();
  const busy = (b, on) => { b.disabled = on; };
  const passField = (label, auto) => {
    const input = el("input", { type: "password", autocomplete: auto, "aria-label": label });
    return { label: el("label", {}, label), input };
  };
  const showBox = (inputs) => {
    const box = el("input", { type: "checkbox", id: "show-pass", class: "check" });
    box.addEventListener("change", () => { for (const i of inputs) i.type = box.checked ? "text" : "password"; });
    return el("div", { class: "row" }, box, el("label", { for: "show-pass", class: "inline" }, t("Show the passphrase")));
  };
  const tabs = el("div", { class: "tabs", role: "tablist" },
    ...[["login", t("Log in")], ["register", t("Register")]].map(([k, label]) =>
      el("button", { type: "button", role: "tab", class: "tab", "aria-selected": String(k === tab), onclick: () => authScreen(info, k) }, label)));
  let form;
  if (tab === "login") {
    const sel = el("select", { "aria-label": t("Profile") }, ...profiles.map((p) => el("option", { value: p.id }, p.legacy ? t("Existing data") : p.name)));
    const pass = passField(t("Passphrase"), "current-password");
    const note = el("p", { class: "muted" });
    const isLegacy = () => { const p = profiles.find((x) => x.id === sel.value) || {}; return p.legacy || p.open; };
    const sync = () => {
      const legacy = isLegacy();
      pass.label.hidden = pass.input.hidden = legacy; // the data from before profiles, and a profile made without a passphrase, open without one
      note.textContent = legacy ? t("Made before profiles existed. It opens the way it always did.") : "";
    };
    sel.addEventListener("change", sync);
    const go = el("button", { class: "primary", type: "submit" }, t("Log in"));
    form = el("form", { class: "stack", onsubmit: async (e) => {
      e.preventDefault();
      err.textContent = "";
      const p = profiles.find((x) => x.id === sel.value);
      if (!p) return;
      if (!p.legacy && !p.open && !pass.input.value) { err.textContent = t("Enter your passphrase."); return; }
      busy(go, true);
      // A profile stays open after a wrong passphrase, so the engine's slowing down of wrong guesses is not undone by starting it again.
      if (S.opened !== p.id) {
        const o = await shellPost("/shell/open", { id: p.id });
        if (o.status !== 200) { err.textContent = errMsg(o.data); busy(go, false); return; }
        S.opened = p.id;
      }
      S.profile = p;
      const st = await keyStatus();
      if (!st) { err.textContent = t("The engine did not start."); busy(go, false); return; }
      if (p.open) {
        const r = await setupOpenProfile(st);
        if (r.status !== 200) { err.textContent = errMsg(r.data); busy(go, false); return; }
      }
      if (st.state === "locked") {
        const u = await api("POST", "/api/keystore/unlock", { passphrase: pass.input.value });
        if (u.status !== 200) { err.textContent = errMsg(u.data); busy(go, false); pass.input.value = ""; pass.input.focus(); return; }
      }
      S.opened = null;
      boot(); // ready (the old data), or still to be set up: the engine's own gate screens take over
    } },
      el("label", {}, t("Profile")), sel, pass.label, pass.input, note, err, showBox([pass.input]), go);
    sync();
  } else {
    const name = el("input", { type: "text", maxlength: "40", autocomplete: "username", "aria-label": t("Name") });
    const one = passField(t("Passphrase"), "new-password"), two = passField(t("Repeat the passphrase"), "new-password");
    const protect = el("input", { type: "checkbox", id: "protect", class: "check" });
    const fields = [one.label, one.input, two.label, two.input];
    const explain = el("p", { class: "muted" });
    const syncProtect = () => {
      for (const f of fields) f.hidden = !protect.checked;
      explain.textContent = protect.checked
        ? t("Your conversations are encrypted with a passphrase only you know. If you forget it, they cannot be opened: there is no way to get it back.")
        : t("No passphrase: the profile opens with one click. Anyone who uses this computer account can open it, so choose a passphrase if that matters to you.");
    };
    protect.addEventListener("change", syncProtect);
    const go = el("button", { class: "primary", type: "submit" }, t("Register"));
    form = el("form", { class: "stack", onsubmit: async (e) => {
      e.preventDefault();
      err.textContent = "";
      if (!name.value.trim()) { err.textContent = t("Give the profile a name."); return; }
      if (protect.checked && one.input.value.length < MIN_PASSPHRASE) { err.textContent = t("A passphrase needs at least 12 characters."); return; }
      if (protect.checked && one.input.value !== two.input.value) { err.textContent = t("The two passphrases are not the same."); return; }
      busy(go, true);
      const c = await shellPost("/shell/profiles", { name: name.value, open: !protect.checked });
      if (c.status !== 200) { err.textContent = errMsg(c.data); busy(go, false); return; }
      const o = await shellPost("/shell/open", { id: c.data.id });
      if (o.status !== 200) { err.textContent = errMsg(o.data); busy(go, false); return; }
      S.profile = c.data;
      const st = await keyStatus();
      if (!st) { err.textContent = t("The engine did not start."); busy(go, false); return; }
      const r = protect.checked ? await api("POST", "/api/keystore/setup", { mode: "passphrase", passphrase: one.input.value }) : await setupOpenProfile(st);
      if (r.status !== 200) { err.textContent = errMsg(r.data); busy(go, false); return; }
      boot(); // the age confirmation and the provider follow, as for anyone new
    } },
      el("label", {}, t("Name")), name,
      el("div", { class: "row" }, protect, el("label", { for: "protect", class: "inline" }, t("Protect this profile with a passphrase"))),
      one.label, one.input, two.label, two.input, explain,
      err, showBox([one.input, two.input]), go);
    syncProtect();
  }
  const body = el("div", { class: "auth" }, tabs, el("div", { class: "card stack" }, form));
  // Removing a profile is rare and final, so it sits out of the way, below the form.
  const removable = profiles.filter((p) => !p.legacy);
  if (removable.length) {
    const rm = el("details", { class: "manage" }, el("summary", {}, t("Manage profiles")));
    for (const p of removable) rm.append(el("div", { class: "row" }, el("span", { class: "grow" }, p.name),
      confirmButton(t("Remove"), "link", t("Click again to remove everything of {name}", { name: p.name }), async () => {
        const r = await shellPost("/shell/remove", { id: p.id });
        if (r.status === 200) boot(); else err.textContent = errMsg(r.data);
      })));
    body.append(rm);
  }
  S.noCard = true; // the card is not shown on the login page
  mount(...screen("Socrates", body));
  S.noCard = false;
  const first = S.root.querySelector("form input:not([hidden]), form select");
  if (first) first.focus({ preventScroll: true });
}

// ---- start: the engine, the key, the age, the provider

async function boot() {
  unwind();
  const pr = await desktopProfiles();
  if (pr) {
    S.profiles = true;
    if (!pr.active) return authScreen(pr);
    S.profile = S.profile || pr.profiles.find((p) => p.id === pr.active) || null;
  }
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
    const who = S.profile && !S.profile.legacy ? S.profile.name : "";
    return mount(...screen(who ? t("Welcome back, {name}", { name: who }) : t("Unlock Socrates"), el("div", { class: "card stack" },
      el("p", {}, t("Your conversations are encrypted. Enter your passphrase to open them.")), pass, err,
      el("button", { class: "primary", onclick: async () => finish(await api("POST", "/api/keystore/unlock", { passphrase: pass.value })) }, t("Unlock")),
      S.profiles ? el("button", { class: "link", onclick: logOut }, t("Choose another profile")) : null)));
  }
  if (st.state === "needs_setup") {
    const pass = el("input", { type: "password", autocomplete: "new-password", "aria-label": t("New passphrase") });
    const opts = st.options || [];
    if (opts.length === 1 && opts[0] === "passphrase") {
      // A profile: the passphrase is the only protection, so it is asked twice (a slip would lock the person out of their own data).
      const again = el("input", { type: "password", autocomplete: "new-password", "aria-label": t("Repeat the passphrase") });
      const make = async () => {
        if (pass.value !== again.value) { err.textContent = t("The two passphrases are not the same."); return; }
        finish(await api("POST", "/api/keystore/setup", { mode: "passphrase", passphrase: pass.value }));
      };
      return mount(...screen(S.profile && !S.profile.legacy ? t("Choose a passphrase, {name}", { name: S.profile.name }) : t("Welcome"), el("div", { class: "card stack" },
        el("p", {}, t("Your conversations are encrypted with a passphrase only you know. If you forget it, they cannot be opened: there is no way to get it back.")),
        el("label", {}, t("A passphrase (at least 12 characters)")), pass, el("label", {}, t("Repeat the passphrase")), again, err,
        el("button", { class: "primary", onclick: make }, t("Create profile")),
        S.profiles ? el("button", { class: "link", onclick: logOut }, t("Choose another profile")) : null)));
    }
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
  await loadTheme();
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
    el("p", {}, t("Socrates is an app for talking with artificial philosophers, made from what was written about them. They ask hard questions and do not give verdicts.")),
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

async function providerScreen(first, back) {
  const err = errorLine();
  const cur = await api("GET", "/api/settings/llm");
  const localAllowed = !!(cur.data && cur.data.localAllowed);
  const options = [el("option", { value: "claude" }, "Claude (Anthropic)"), el("option", { value: "openai" }, "OpenAI")];
  // A model server on this computer is looked for by itself, and the models it holds are offered in a list: nothing to type.
  const lookFor = async (endpoint) => { const r = await api("GET", "/api/settings/llm/local-models" + (endpoint ? "?endpoint=" + encodeURIComponent(endpoint) : "")); return r.status === 200 ? r.data : { models: [], endpoint: "" }; };
  const found = localAllowed ? await lookFor("") : { models: [] };
  if (localAllowed) options.push(el("option", { value: "ollama" }, found.models.length ? t("A model on this computer ({n} found)", { n: found.models.length }) : t("A model on this computer")));
  const provider = el("select", { "aria-label": t("Provider") }, ...options);
  const key = el("input", { type: "password", autocomplete: "off", placeholder: t("Paste your API key"), "aria-label": t("API key") });
  const keyBox = el("div", {}, el("label", {}, t("Your API key")), key, el("p", { class: "muted" }, t("The key stays on this device, encrypted. It is tested once, then used only to talk to your provider.")));
  const endpoint = el("input", { type: "text", value: (cur.data && cur.data.ollamaEndpoint) || "http://127.0.0.1:11434", "aria-label": t("Address of the model server") });
  const modelSel = el("select", { "aria-label": t("Model") });
  const noModel = el("p", { class: "muted" }, t("No model was found on this computer. Start your model server (for example Ollama) and press Look again."));
  const fillModels = (d) => {
    modelSel.replaceChildren(...d.models.map((m) => el("option", { value: m }, m)));
    if (d.current && d.models.includes(d.current)) modelSel.value = d.current;
    modelSel.hidden = !d.models.length; noModel.hidden = d.models.length > 0;
  };
  fillModels(found);
  const again = el("button", { class: "link", type: "button", onclick: async () => fillModels(await lookFor(endpoint.value)) }, t("Look again"));
  const epBox = el("div", { hidden: true }, el("label", {}, t("Model")), modelSel, noModel, again, el("label", {}, t("Address of the model server")), endpoint,
    el("p", { class: "muted" }, t("The server of a model that runs on this computer, for example Ollama. Nothing leaves the computer if the address is this computer.")),
    el("p", { class: "notice", role: "note" }, t("How well the philosophers talk, and how reliably the safety checks work, depend on what the model can do. A small model understands Greek and instructions less well, may answer badly, and may judge a risky message wrongly. Choose the strongest model you can run.")));
  const help = el("div", {}, keyHelp(provider.value, err));
  const helpCard = el("div", { class: "card" }, help);
  provider.addEventListener("change", () => {
    const local = provider.value === "ollama";
    keyBox.hidden = local; epBox.hidden = !local; helpCard.hidden = local;
    if (!local) help.replaceChildren(keyHelp(provider.value, err));
  });
  const save = el("button", { class: "primary" }, t("Save and test"));
  save.addEventListener("click", async () => {
    err.textContent = ""; save.disabled = true; save.textContent = t("Testing…");
    const body = { provider: provider.value };
    if (provider.value === "ollama") { body.ollamaEndpoint = endpoint.value; if (modelSel.value) body.localModel = modelSel.value; } else body.apiKey = key.value;
    const r = await api("PUT", "/api/settings/llm", body);
    save.disabled = false; save.textContent = t("Save and test");
    if (r.status === 200) { key.value = ""; return boot(); }
    err.textContent = errMsg(r.data);
  });
  mount(...screen(first ? t("Connect an AI provider") : t("AI provider"), el("div", {}, el("div", { class: "card stack" },
    el("p", {}, t("Socrates uses your own account with an AI provider. You pay the provider directly; Socrates has no server and sees nothing.")),
    el("label", {}, t("Provider")), provider, keyBox, epBox, err, save), helpCard), back ? el("button", { class: "link", onclick: back }, t("Back")) : null));
}

// ---- home: the philosophers and the conversations

async function home() {
  refreshCards(); // the card's messages from the project's file, once per session and only while the card is shown
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
  mount(...screen("Socrates", body, el("span", { class: "header-actions" }, S.profiles ? iconBtn("logout", t("Log out"), logOut) : null, iconBtn("progress", t("Progress"), () => show(progressScreen)), iconBtn("settings", t("Settings"), () => show(settings)))));
}

// ---- progress: how far the person has come with every philosopher, unlocked or not

function listNames(names) { return names.join(", "); }

async function progressScreen() {
  const r = await api("GET", "/api/philosophers");
  if (r.status === 200) S.philosophers = r.data.philosophers || [];
  const body = el("div", { class: "stack" });
  body.append(el("p", { class: "muted" }, t("A question counts when you have thought it through to the end with a philosopher, in your own words. Meeting each later philosopher takes 5 of them with every philosopher before him.")));
  for (const p of S.philosophers) {
    const n = p.challenges || 0, need = p.required || 5;
    const card = el("div", { class: "card stack" }, el("h2", {}, p.name));
    card.append(el("div", { class: "muted" }, t("Questions thought through: {n}", { n })));
    if (p.leads && p.leads.length) {
      const names = listNames(p.leads), got = Math.min(n, need);
      card.append(el("label", { for: "prog-" + p.id }, t("Toward meeting {names}: {done} of {need}", { names, done: got, need })),
        el("progress", { id: "prog-" + p.id, value: String(got), max: String(need) }));
    }
    if (p.inProgress > 0) {
      card.append(el("label", { for: "open-" + p.id }, t("A question in progress")),
        el("progress", { id: "open-" + p.id, value: String(p.inProgress), max: "1000" }));
    }
    body.append(card);
  }
  mount(...screen(t("Progress"), body, backButton(t("Back"))));
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
      const bubble = el("div", { class: "msg philosopher" }, m.text, " ", copyButton(m.text), el("button", { class: "link small", type: "button", "aria-label": t("Report this reply"), onclick: () => { if (!bubble.nextSibling || !bubble.nextSibling.classList || !bubble.nextSibling.classList.contains("help")) report(m, bubble); } }, t("Report")));
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
  // The person sees how far they have come with this philosopher without leaving the chat: the questions thought through to the end and
  // how far the question now under way is. No numbers beyond the count; the bar is read again after every turn.
  const progressLine = el("div", { class: "progressline" });
  const refreshProgress = async () => {
    const res = await api("GET", "/api/philosophers");
    if (res.status !== 200) return;
    S.philosophers = res.data.philosophers || [];
    const me = S.philosophers.find((p) => p.id === conv.philosopherId);
    if (!me) { progressLine.replaceChildren(); return; }
    progressLine.replaceChildren(el("span", { class: "muted" }, t("Questions thought through: {n}", { n: me.challenges || 0 })),
      me.inProgress > 0 ? el("progress", { value: String(me.inProgress), max: "1000", "aria-label": t("A question in progress") }) : null);
  };
  const refreshHelp = () => { helpBtn.hidden = !canHelp(); if (helpBtn.hidden) helpBox.hidden = true; showOffer(); refreshProgress(); };
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
  const send = el("button", { class: "primary send", type: "submit", title: t("Send") });
  const sendIcon = (waiting) => send.replaceChildren(icon(waiting ? "spinner" : "send", waiting ? "spin" : ""), el("span", { class: "sr" }, t("Send")));
  sendIcon(false);
  let ctl = null; // lets Stop cancel the request: the engine then keeps nothing of the turn
  const stop = iconBtn("stop", t("Stop"), () => { if (ctl) ctl.abort(); }, { hidden: true });
  const err = errorLine();
  let busy = false;
  const submit = async (ev) => {
    ev.preventDefault();
    const text = input.value.trim();
    if (!text || busy) return;
    busy = true; send.disabled = true; err.textContent = ""; stop.hidden = false; sendIcon(true); ctl = new AbortController();
    const mine = el("div", { class: "msg user" }, text);
    const typing = el("div", { class: "msg philosopher typing" }, el("i"), el("i"), el("i"), el("span", { class: "sr" }, t("{name} is thinking…", { name: who.name })));
    list.append(mine, typing); input.value = ""; main.scrollTop = main.scrollHeight;
    const flag = fromSuggestion; fromSuggestion = false; helpBox.hidden = true;
    const res = await api("POST", "/api/conversations/" + encodeURIComponent(id) + "/messages", flag ? { text, suggested: true } : { text }, ctl.signal);
    typing.remove();
    busy = false; send.disabled = false; stop.hidden = true; sendIcon(false); ctl = null;
    const d = res.data;
    if (d.aborted) { mine.remove(); input.value = text; fromSuggestion = flag; refreshHelp(); input.focus(); return; } // stopped: nothing was kept, the words are back in the box
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
  const form = el("form", { class: "composer", onsubmit: submit }, input, stop, send);
  mount(el("header", {}, iconBtn("back", t("Back"), back), headerLogo(), el("h1", {}, who.name),
    el("span", { class: "header-actions" }, iconBtn("insight", t("Memory"), () => show(() => memory(who))), iconBtn("trash", t("Delete"), showDelete))),
    progressLine, el("div", { class: "chatwrap" }, el("div", { class: "watermark", "aria-hidden": "true" }), main), err, helpBox, challengeBar, deleteBox, el("div", { class: "helpbar" }, helpBtn, challengeBtn), form);
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
  mount(...screen(t("What {name} remembers", { name: who.name }), body, backButton(t("Back"))));
}

function ended(text) {
  mount(...screen("Socrates", el("div", { class: "card stack" }, el("p", {}, text), el("button", { class: "primary", onclick: () => { unwind(); home(); } }, t("OK")))));
}

// ---- the manual and About (both opened from Settings, "About and help", as in Moly)

function philName(id, fallback) { const p = (S.philosophers || []).find((x) => x.id === id); return p ? p.name : fallback; }

const REPO = "https://github.com/Nireus79/Socrates-ai";
const SITE = "https://nireus79.github.io/Socrates-ai/";

// The user manual: a section opens and closes; everything is plain text.
function manualScreen() {
  const body = el("div", { class: "stack" });
  const sec = (title, open, ...nodes) => body.append(el("details", { class: "manual-section", open: open ? "" : null },
    el("summary", {}, title), el("div", { class: "manual-body stack" }, ...nodes)));
  const P = (text) => el("p", {}, text);
  const list = (...items) => el("ul", {}, ...items.map((i) => el("li", {}, i)));
  body.append(el("p", { class: "muted" }, t("How to use Socrates, from the first start to the last conversation. Open a section to read it.")));
  sec(t("What Socrates does"), true,
    P(t("Socrates is an app for talking with four artificial Greek philosophers: Socrates, Diogenes, Plato and Aristotle. Each is made from what was written about him and after him, and imagined alive in today's world. They are not real people, and they are not a source of medical, legal or other professional advice.")),
    P(t("They are not here to hand you answers. They ask, they answer in their own manner, and they say so when they do not know. The thinking is yours.")));
  sec(t("Getting started"), false,
    el("ol", {}, ...[
      t("Choose Claude or OpenAI: Socrates works with your own account at one of them."),
      t("No key yet? On the provider screen, the How to get a key card has buttons that open the provider's pages, with the steps beside them. A chat subscription is not an API key."),
      t("Paste the key and press Save. Socrates tries the key with the provider before it keeps it."),
    ].map((i) => el("li", {}, i))),
    S.profiles ? P(t("On a computer you first make a profile: a name and, if you wish, a passphrase. A forgotten passphrase cannot be recovered, because nothing is stored anywhere else.")) : null);
  sec(t("Talking with a philosopher"), false,
    P(t("Choose a philosopher and write whatever is on your mind: a question, an opinion, something you have always wondered. You can write in English or in Greek, and he answers in the language you write in.")),
    P(t("His replies are short and usually end with one question. Answer it in your own words, even roughly: your own words are what the conversation is built from.")),
    P(t("Nothing is marked right or wrong. When you have thought a question all the way through with him, he tells you in a plain sentence.")));
  sec(t("Help me answer"), false,
    P(t("Stuck? Press Help me answer. You get a few starting points from different angles. Pick one, change it, or ignore them all. Words you took from the list count for less when progress is measured, because they began as someone else's.")));
  sec(t("Put a question to me"), false,
    P(t("Press Put a question to me when you want him to put a question of his own to you, chosen from who he is and what he has learnt about you. He may also offer one when a conversation reaches a natural pause.")));
  sec(t("Meeting the others"), false,
    P(t("Socrates is open from the start. To meet each of the others, you think 5 questions through with every philosopher before him, in the order Socrates, Diogenes, Plato, Aristotle. In this free beta all four are already open; the Progress screen still keeps the count.")),
    P(t("A question counts when you have worked on it over several messages, said what your key words mean, and tested at least one of your own views, in your own words. Long messages alone do not count, and neither does agreeing with everything.")));
  sec(t("What they remember"), false,
    P(t("Each philosopher keeps short notes about what you said: your claims, the words you defined, where two of your views pull apart, and small facts about your life. They are for him alone, stored encrypted on this device, and he is told to refer to them only when they matter.")),
    P(t("Open the Memory button inside a conversation to read or delete them. Settings has a switch to turn memory off; with it off nothing new is noted and no question can count as thought through.")));
  const phil = (id, name, ...paras) => [el("h3", {}, philName(id, name)), ...paras.map(P)];
  sec(t("The four philosophers"), false,
    ...phil("socrates", "Socrates",
      t("He asks far more than he tells. He takes your words seriously, follows them to where they lead, and asks whether you still hold them there. He seldom says you are wrong; you tend to find it yourself. A conversation often ends not with an answer but with a better question."),
      t("What helps: say what you mean by your main words, give him a real case from your life, and admit when you are not sure. If you stay stuck he may give his own view, as one view among others.")),
    ...phil("diogenes", "Diogenes",
      t("Short and blunt, and the joke turns on himself first. He answers more than he asks, often with a plain picture instead of an argument, and he goes after poses and conventions, not after you."),
      t("What helps: be honest about the gap between what you say and what you do. Insults get a witty answer and no escalation, and shock for its own sake gets you nowhere. If you are really low, he drops the edge.")),
    ...phil("plato", "Plato",
      t("He builds upward: from one case toward what all such cases share. He likes a picture or a story to think with, and he offers a view and asks you to test it."),
      t("What helps: say what you see in his picture, in your own words; start from a case before you make a general claim; and push back, since he expects it. Asked about the harsh parts of his own writing, he says plainly what he wrote and asks you what you make of it.")),
    ...phil("aristotle", "Aristotle",
      t("He sorts. He starts from what people usually say, finds where those sayings pull apart, and asks which kind of thing you mean and in what sense. His examples come from nature and from trades."),
      t("What helps: expect precision, take one distinction at a time, and bring an example and a counter-example. Asked about slavery or women, he says honestly what he held and why, then asks a question about today.")),
    el("h3", {}, t("What all four share")),
    P(t("They know what came after them, so they can speak of one another and disagree. They are honest that they are artificial. When something they believed would be unpopular today, they say it plainly, set it in the frame of their own time, and then ask you a question about now; they do not settle it for you.")));
  sec(t("A few tips"), false, list(
    t("Write in sentences. One word gives him little to work with."),
    t("Say what you mean by the words that carry your view. Most disagreements start there."),
    t("Disagree with him. Changing your mind is welcome and counts as thinking."),
    t("I do not know is a good answer. Say it, and then try Help me answer."),
    t("Ask the same question of two philosophers and compare."),
    t("They can misremember a detail. Check what matters, and ask him where something comes from: he will say what he knows and admit what he does not.")));
  sec(t("Your data and privacy"), false, list(
    t("A safety check runs on every message and every reply, above every philosopher. If a reply is wrong or harmful, press Report next to it."),
    t("The app uses your own key from Claude or OpenAI, which you pay the provider for; Settings explains how to get one. Your key and your conversations stay encrypted on this device. The only thing the app fetches is the sponsor card, once a day, and it sends nothing about you."),
    t("Export conversations saves your conversations in one file, never your key. Import puts such a file back. Delete everything removes every conversation, your key and your settings from this device, and it cannot be undone."),
    t("Settings has a Language choice and a Theme: Automatic, Light or Dark. The small card at the bottom is a message from the project or a sponsor and is always labelled Sponsored; Hide removes it until you open the app again.")));
  const faq = [
    [t("The engine does not answer."), t("Close Socrates completely and open it again. Your message stays in the box.")],
    [t(PROVIDER_PROBLEMS.auth), t("Copy the key again from the provider's website and paste it in Settings, under AI provider. A key from Claude starts with sk-ant, one from OpenAI with sk-.")],
    [t(PROVIDER_PROBLEMS.rate_limit), t("You have used up the provider's limit or your credit. Wait, or add credit at the provider, then send the message again.")],
    [t(PROVIDER_PROBLEMS.overloaded), t("Wait a moment and send the message again. It is back in the box.")],
    [t("Is anything running after I close the app?"), t("No. Closing the app stops its engine. If you only switch to another app, the engine stops by itself after about three minutes.")],
    [t("Socrates stops while the phone is idle"), t("Some phones close apps to save battery. In the phone's battery settings, allow Socrates to run in the background.")],
    [t("Where is the log?"), t("On a computer, the engine keeps its log in the .socrates folder of your user folder, in the file socrates.log. It holds no message text. On the phone, Settings, Report a problem shows the engine's error lines for you to read and send.")],
    [t("Something else, or it keeps happening"), t("Problems, questions and reports: {address} (Hermes Software).", { address: CONTACT }) + " " + t("Please leave out your conversations and any key.")],
  ];
  sec(t("Something is wrong"), false, el("dl", { class: "faq" }, ...faq.flatMap(([q, a]) => [el("dt", {}, q), el("dd", {}, a)])));
  mount(...screen(t("User manual"), body, backButton(t("Settings"))));
}

// About: what it is, who makes it, how to reach us, and under what terms.
async function aboutScreen() {
  const err = errorLine();
  const st = await api("GET", "/api/status");
  const body = el("div", { class: "stack" });
  const card = (title, ...nodes) => body.append(el("div", { class: "card stack" }, el("h2", {}, title), ...nodes));
  const P = (text, cls) => el("p", cls ? { class: cls } : {}, text);
  const out = (label, url) => el("div", { class: "row about-link" }, el("button", { class: "small-btn", onclick: () => openCardLink(url, err) }, label), el("span", { class: "muted" }, url.replace("https://", "")));
  body.append(el("div", { class: "card stack center" }, logo(), el("h2", {}, "Socrates"), st.data.version ? P(t("Version {v}.", { v: st.data.version }), "muted") : null));
  card(t("What Socrates is"),
    P(t("Socrates is an app for talking with four artificial Greek philosophers: Socrates, Diogenes, Plato and Aristotle. Each is made from what was written about him and after him, and imagined alive in today's world. They are not real people, and they are not a source of medical, legal or other professional advice.")),
    P(t("It keeps your conversations in an encrypted store on this device and talks to the AI model you choose: your own Claude or OpenAI key, or on a computer a model of your own. There is no Socrates server, no sign-up, no analytics and no tracking.")),
    P(t("How well the philosophers talk, and how reliably the safety checks work, depend on what the model behind them can do."), "muted"),
    P(t("If you are thinking of hurting yourself, please call your local emergency number or a crisis line, or tell someone you trust."), "muted"),
    P(t("For people aged 16 and over."), "muted"));
  card(t("Who makes it"), P(t("Socrates is made by Hermes Software, one person, in the same family as the writing coach Moly.")));
  card(t("Contact"),
    P(t("Email") + ": " + CONTACT),
    out(t("Questions and problems"), REPO + "/issues"),
    out(t("Website"), SITE),
    out(t("Source code"), REPO),
    out(t("Developer"), "https://hermesoftware.wordpress.com/"),
    out(t("Privacy policy"), SITE + "privacy.html"),
    P(t("When you write for help, please do not include your conversations or any key."), "muted"), err);
  card(t("Support the project"),
    P(t("Socrates is free. If it helps you, please sponsor its development, or tell a friend.")),
    el("div", { class: "row" }, el("button", { class: "primary small-btn", onclick: () => openCardLink("https://github.com/sponsors/Nireus79", err) }, t("Sponsor")), el("button", { class: "small-btn", onclick: () => openCardLink(REPO, err) }, "GitHub")));
  card(t("Licence"), P(t("Socrates is free and open source under the MIT licence: you may use, copy and change it as long as the licence notice stays with it. It comes without warranty.")));
  mount(...screen(t("About Socrates"), body, backButton(t("Settings"))));
}

// "Report a problem" (the phone app, as in Moly): the report is built on the phone and shown in full; it leaves the phone only when the
// person presses Send and their mail app opens with it. The person may add a note, for example a reply they want to report.
function reportCard() {
  const sh = shellInfo();
  if (!sh || !sh.buildReport || !sh.shareReport) return null;
  const box = el("div", { class: "stack" });
  const msg = errorLine();
  const prepare = async () => {
    msg.textContent = "";
    let text;
    try { text = await sh.buildReport(); } catch (_) { msg.textContent = t("Could not prepare the report."); return; }
    const note = el("textarea", { class: "report-text", rows: "4", placeholder: t("Your note: describe the problem, or paste the reply you want to report (optional)"), "aria-label": t("Your note") });
    const body = el("textarea", { class: "report-text", rows: "10", readonly: "", "aria-label": t("The report") }, text);
    const send = async () => {
      try { await sh.shareReport(note.value.trim() ? note.value.trim() + "\n\n---\n" + text : text); } catch (_) { msg.textContent = t("Could not open a mail app."); }
    };
    box.replaceChildren(note, body, el("div", { class: "row" },
      el("button", { class: "primary", onclick: send }, t("Send report")),
      el("button", { onclick: () => box.replaceChildren(el("button", { onclick: prepare }, t("Prepare report"))) }, t("Close"))));
  };
  box.append(el("button", { onclick: prepare }, t("Prepare report")));
  return el("div", { class: "card stack" }, el("h2", {}, t("Report a problem or a reply")),
    el("p", { class: "muted" }, t("Prepares a short note about how Socrates is doing: your phone model and Android version, whether its engine is running, and error lines from the engine. You can add your own note, for example a reply you want to report: paste it there. Read everything first. It goes nowhere unless you press Send and your mail app opens with it; you press Send there.")),
    box, msg);
}

// ---- settings

async function settings() {
  const err = errorLine();
  const body = el("div", { class: "stack" });
  const cards = {};
  const p = await api("GET", "/api/settings/llm");
  cards.language = el("div", { class: "card stack" }, el("h2", {}, t("Language")), languagePicker(settings));
  cards.theme = el("div", { class: "card stack" }, el("h2", {}, t("Appearance")), el("p", { class: "muted" }, t("Socrates follows your device by default.")), themePicker());
  if (S.profiles) cards.profile = (el("div", { class: "card stack" }, el("h2", {}, t("Profile")),
    el("p", { class: "muted" }, S.profile && !S.profile.legacy ? t("Signed in as {name}.", { name: S.profile.name }) : t("Signed in to the existing data.")),
    el("button", { onclick: logOut }, t("Log out"))));
  cards.provider = (el("div", { class: "card stack" }, el("h2", {}, t("AI provider")),
    el("p", { class: "muted" }, p.data.provider ? t("{provider}, models {fast} and {reply}", { provider: p.data.provider === "claude" ? "Claude (Anthropic)" : p.data.provider === "openai" ? "OpenAI" : p.data.provider === "ollama" ? t("A model on this computer") : p.data.provider, fast: p.data.fastModel, reply: p.data.replyModel }) : t("Not set up.")),
    el("button", { onclick: () => show(() => providerScreen(false, back)) }, t("Change provider or key"))));
  cards.data = (el("div", { class: "card stack" }, el("h2", {}, t("Your data")),
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
  cards.import = importInput;
  const mem = await api("GET", "/api/settings/memory");
  const box = el("input", { type: "checkbox", id: "memory-on", class: "check" });
  box.checked = mem.data.enabled !== false;
  box.addEventListener("change", async () => {
    const r = await api("PUT", "/api/settings/memory", { enabled: box.checked });
    if (r.status !== 200) { box.checked = !box.checked; err.textContent = errMsg(r.data); }
  });
  cards.memory = (el("div", { class: "card stack" }, el("h2", {}, t("Memory")),
    el("div", { class: "row" }, box, el("label", { for: "memory-on", class: "inline" }, t("The philosophers remember what I tell them"))),
    el("p", { class: "muted" }, t("Each keeps short notes with your own words, only for himself, stored encrypted on this device. Turning this off stops new notes and stops him using the old ones; it does not delete them. You can see and delete them from the Memory button inside a conversation."))));
  const st = await api("GET", "/api/status");
  const row = (title, sub, go) => el("button", { class: "linkrow", onclick: () => show(go) }, el("span", {}, el("strong", {}, title), el("small", {}, sub)), icon("chevron"));
  cards.report = reportCard();
  cards.help = (el("div", { class: "card stack" }, el("h2", {}, t("About and help")),
    row(t("About Socrates"), t("What it is, who makes it, how to reach us") + (st.data.version ? " · " + t("Version {v}.", { v: st.data.version }) : ""), aboutScreen),
    row(t("User manual"), t("How to use Socrates, step by step, and what to do when something is wrong"), manualScreen)));
  cards.delete = (el("div", { class: "card stack" }, el("h2", {}, t("Delete everything")),
    el("p", { class: "muted" }, t("Removes every conversation, your provider key and your settings from this device. It cannot be undone.")),
    confirmButton(t("Delete everything"), "danger", t("Click again to delete everything"), async () => {
      const r = await api("DELETE", "/api/profile", { confirm: "delete" });
      if (r.status === 204) boot(); else err.textContent = errMsg(r.data);
    })));
  if (S.profiles) {
    // The computer app: its version and where to look for a newer one (nothing is looked up by itself), and how to remove it.
    cards.app = el("div", { class: "card stack" }, el("h2", {}, t("Socrates app")),
      el("p", { class: "muted" }, (st.data.version ? t("Version {v}.", { v: st.data.version }) + " " : "") + t("Socrates does not look for updates by itself and sends nothing about you. To see whether a newer version exists, open the releases page.")),
      el("div", { class: "row" }, el("button", { onclick: () => openCardLink(REPO + "/releases", err) }, t("Open the releases page"))));
    cards.remove = el("div", { class: "card stack" }, el("h2", {}, t("Remove Socrates")),
      el("ol", { class: "muted" }, ...[t("Close Socrates and log out of every profile."), t("Delete the folder you unpacked Socrates into."),
        t("Your conversations and profiles stay in the .socrates folder of your user folder. Export what you want to keep first, then delete that folder too.")].map((x) => el("li", {}, x))));
  }
  for (const k of ["provider", "profile", "app", "theme", "language", "memory", "data", "import", "report", "remove", "delete", "help"]) if (cards[k]) body.append(cards[k]);
  mount(...screen(t("Settings"), body, backButton(t("Back"))));
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

function start(root) { S.root = root || document.getElementById("root"); S.lang = detectedLang(); applyTheme(rememberedTheme()); followDevice(); window.addEventListener("focus", followDevice); window.socratesBack = goBack; return boot(); }
if (typeof module !== "undefined") module.exports = { start, el, api, S };
else window.addEventListener("DOMContentLoaded", () => start());
