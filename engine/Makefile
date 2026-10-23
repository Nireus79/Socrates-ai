# The gate every change must pass (BUILD_PLAN section 5).
.PHONY: check build test live cross
check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet ./...
	go test -race -count=1 ./...
build:
	go build -ldflags "-X main.version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/socrates-engine ./cmd/socrates-engine
test:
	go test -race -count=1 ./...
# Live checks against the real providers (they cost a few tokens): ANTHROPIC_API_KEY=... make live
live:
	go test -tags live -run Live -v -count=1 ./internal/llm/
# The engine must at least compile for every platform we ship to (it has no cgo). Compiling is not running: Windows and macOS are untested at run time.
cross:
	for t in windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/arm64 android/arm64; do \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cmd/socrates-engine || exit 1; done
