// The phone shell for the screens (../app/app.js). The engine is a separate process started by the app's foreground service; the native
// plugin SocratesEngine starts it and answers with its port and token. The page waits for that (window.socratesShell.ready) before it
// sends anything. Nothing here reads other apps or pages.
(function () {
  "use strict";
  var plugin = window.Capacitor && window.Capacitor.Plugins && window.Capacitor.Plugins.SocratesEngine;
  var shell = { base: "", token: "" };
  shell.ready = plugin
    ? plugin.start().then(function (r) { shell.base = "http://127.0.0.1:" + r.port; shell.token = r.token; })
    : Promise.reject(new Error("the app's engine plugin is missing"));
  // The engine may have been stopped while the app was away (it stops itself when idle); starting it again gives a new port and token.
  shell.reconnect = function () {
    return plugin
      ? plugin.start().then(function (r) { shell.base = "http://127.0.0.1:" + r.port; shell.token = r.token; })
      : Promise.reject(new Error("the app's engine plugin is missing"));
  };
  // "Export": the system's save-as picker, so the person chooses where the file goes.
  shell.saveFile = function (name, text) { return plugin.saveFile({ name: name, text: text }); };
  // The key help opens the provider's page in the phone's browser (the plugin allows only the two providers' sites over https).
  shell.openLink = function (url) { return plugin.openLink({ url: url }); };
  // The links of the card at the foot of the screens: any plain https address (the plugin checks it again).
  shell.openSponsorLink = function (url) { return plugin.openSponsorLink({ url: url }); };
  // "Report a problem": the text is built on the phone and shown; it goes to the mail app only when the person chooses to send it.
  shell.buildReport = function () { return plugin.buildReport().then(function (r) { return r.text; }); };
  shell.shareReport = function (text) { return plugin.shareReport({ text: text }); };
  window.socratesShell = shell;
})();
