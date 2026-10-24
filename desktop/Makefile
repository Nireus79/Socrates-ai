# The desktop window. Needs the Wails v2 tool (go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0) and, on Linux,
# libgtk-3-dev and libwebkit2gtk-4.1-dev. Systems that have only WebKitGTK 4.1 need the webkit2_41 tag.
TAGS ?= webkit2_41
.PHONY: frontend build test windows
frontend:
	rm -rf frontend/dist && mkdir -p frontend/dist
	cp ../app/index.html ../app/style.css ../app/app.js ../app/i18n.js ../app/icon.png ../app/mark.png frontend/dist/
build: frontend
	$(MAKE) -C ../engine build
	CGO_ENABLED=1 go build -tags "$(TAGS),production" -o bin/socrates-desktop .
	cp ../engine/bin/socrates-engine bin/socrates-engine
test:
	go vet ./shell/ && go test -race -count=1 ./shell/
# The Windows window and engine (the icon and version go in through go-winres: go install github.com/tc-hib/go-winres@v0.3.3), cross-compiled without a C compiler (the window needs the WebView2 runtime on the machine, which Windows 11
# has). Compiling is not running: nothing here has been run on Windows.
windows: frontend
	go-winres simply --arch amd64 --icon icon.png --manifest gui --product-name Socrates --file-description "Socrates: ask a philosopher" --copyright "Hermes Software, MIT" --out rsrc
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags production -ldflags "-H windowsgui" -o bin/socrates-desktop.exe .
	cd ../engine && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o ../desktop/bin/socrates-engine.exe ./cmd/socrates-engine
