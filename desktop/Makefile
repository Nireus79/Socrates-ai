# The desktop window. Needs the Wails v2 tool (go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0) and, on Linux,
# libgtk-3-dev and libwebkit2gtk-4.1-dev. Systems that have only WebKitGTK 4.1 need the webkit2_41 tag.
TAGS ?= webkit2_41
.PHONY: frontend build test
frontend:
	rm -rf frontend/dist && mkdir -p frontend/dist
	cp ../app/index.html ../app/style.css ../app/app.js frontend/dist/
build: frontend
	$(MAKE) -C ../engine build
	CGO_ENABLED=1 go build -tags "$(TAGS),production" -o bin/socrates-desktop .
	cp ../engine/bin/socrates-engine bin/socrates-engine
test:
	go vet ./shell/ && go test -race -count=1 ./shell/
