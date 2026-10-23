"use strict";
const test = require("node:test");
const assert = require("node:assert");
const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const root = path.join(__dirname, "..");

test("the phone page may reach the engine on the loopback address and nothing else, and loads the shim first", () => {
  execFileSync(path.join(root, "scripts/build-www.sh"), { cwd: root });
  const html = fs.readFileSync(path.join(root, "www/index.html"), "utf8");
  const csp = /Content-Security-Policy" content="([^"]+)"/.exec(html)[1];
  assert.match(csp, /default-src 'self'/);
  assert.match(csp, /connect-src 'self' http:\/\/127\.0\.0\.1:\*;/);
  assert.ok(!/https?:\/\/(?!127\.0\.0\.1)/.test(html), "no other address in the page");
  assert.ok(html.indexOf("shim.js") > 0 && html.indexOf("shim.js") < html.indexOf("app.js"), "the shim must run before the screens");
  for (const f of ["app.js", "style.css", "shim.js"]) assert.ok(fs.existsSync(path.join(root, "www", f)), f);
});

test("the shim waits for the native plugin, hands the page its address and token, and fails plainly when the plugin is missing", async () => {
  const code = fs.readFileSync(path.join(root, "shim.js"), "utf8");
  const run = (Capacitor) => { const w = { Capacitor }; new Function("window", code)(w); return w.socratesShell; };
  const ok = run({ Plugins: { SocratesEngine: { start: async () => ({ port: 4321, token: "tok" }), saveFile: async (a) => a } } });
  await ok.ready;
  assert.equal(ok.base, "http://127.0.0.1:4321");
  assert.equal(ok.token, "tok");
  assert.deepEqual(await ok.saveFile("n.json", "{}"), { name: "n.json", text: "{}" });
  // reconnect: after the engine was stopped and started again, the address and token are the new ones
  let n = 0;
  const again = run({ Plugins: { SocratesEngine: { start: async () => ({ port: 5000 + ++n, token: "tok" + n }), saveFile: async () => ({}) } } });
  await again.ready;
  assert.equal(again.base, "http://127.0.0.1:5001");
  await again.reconnect();
  assert.equal(again.base, "http://127.0.0.1:5002");
  assert.equal(again.token, "tok2");
  const missing = run(undefined);
  await assert.rejects(missing.reconnect(), /plugin is missing/);
  await assert.rejects(missing.ready, /plugin is missing/);
});

test("the Android project keeps nothing in backups and allows cleartext only to the engine", () => {
  const main = path.join(root, "android/app/src/main");
  const manifest = fs.readFileSync(path.join(main, "AndroidManifest.xml"), "utf8");
  assert.match(manifest, /android:allowBackup="false"/);
  assert.match(manifest, /dataExtractionRules/);
  assert.match(manifest, /networkSecurityConfig/);
  const net = fs.readFileSync(path.join(main, "res/xml/network_security_config.xml"), "utf8");
  assert.match(net, /<domain includeSubdomains="false">127\.0\.0\.1<\/domain>/);
  assert.equal((net.match(/<domain /g) || []).length, 1, "exactly one cleartext domain");
  assert.ok(!/<base-config[^>]*cleartextTrafficPermitted="true"/.test(net));
  assert.ok(!/ACTION_SEND|android.intent.action.SEND/.test(manifest), "no share target yet");
});

test("the Android activity asks the page to go back and only leaves when the page says it is at its first screen", () => {
  const java = fs.readFileSync(path.join(root, "android/app/src/main/java/io/github/nireus79/socrates/MainActivity.java"), "utf8");
  assert.match(java, /OnBackPressedCallback/);
  assert.match(java, /evaluateJavascript\([^;]*socratesBack/s);
  assert.match(java, /!"true"\.equals\(answer\)\) leave\(\)/);
  assert.match(java, /setEnabled\(false\)/, "at the first screen the key must fall through to the system");
  assert.ok(!/canGoBack|goBack\(\)/.test(java), "the WebView's own history is not used: it ignores steps made without a touch");
});
