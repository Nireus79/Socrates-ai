# The gate every change must pass (BUILD_PLAN section 5).
.PHONY: check build test live
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
