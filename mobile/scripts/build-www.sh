#!/bin/bash
# Assembles www/: the screens of ../app, the phone shim, and a content security policy that also lets the page reach the engine on the
# loopback address (and nothing else).
set -euo pipefail
cd "$(dirname "$0")/.."
rm -rf www && mkdir -p www
cp ../app/style.css ../app/app.js ../app/icon.png ../app/mark.png shim.js www/
python3 - <<'PY'
import re
html = open("../app/index.html", encoding="utf-8").read()
csp = "connect-src 'self' http://127.0.0.1:*;"
assert "connect-src 'self';" in html, "the app's content security policy changed"
html = html.replace("connect-src 'self';", csp, 1)
html = html.replace('<script src="app.js"></script>', '<script src="shim.js"></script>\n<script src="app.js"></script>', 1)
assert "shim.js" in html
open("www/index.html", "w", encoding="utf-8").write(html)
PY
