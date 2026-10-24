# socrates-engine

The engine of Socrates, in Go. It holds the keys, the encrypted store, the safety stage and the philosophers' replies, and serves a shell (the desktop window or the phone app) over HTTP on the loopback address only. Plan and decisions: `../docs/BUILD_PLAN.md`; what is protected and what is not: `../docs/THREAT_MODEL.md`.

```
make check   # gofmt, go vet, go test -race: must pass before every commit
make build   # bin/socrates-engine
make live    # tests against the real providers (needs ANTHROPIC_API_KEY, costs a few tokens)
```

## How a shell starts the engine
1. Run `socrates-engine [--data-dir DIR] [--port N]` with a pipe on its standard input and read its standard output. Standard output carries **one line of JSON and nothing else**: `{"port":51234,"token":"<64 hex>","version":"..."}`. Logs go to standard error.
2. Every request goes to `http://127.0.0.1:<port>` with the header `X-Socrates-Token: <token>`, except `GET /api/status` (the liveness check, no token). A request with an `Origin` header is refused unless the shell registered that exact origin in `SOCRATES_ALLOWED_ORIGINS` (comma separated).
3. Until the database key is available (the person chooses a passphrase or a key file, or unlocks one) only the key gate answers: `GET /api/status` (423 `locked`), `/api/keystore/{status,recheck,unlock,setup}`; everything else answers 423.
4. The engine stops when the shell closes its standard input, on `POST /internal/shutdown` (with the token), or on SIGINT/SIGTERM.
5. The database key comes from the system keychain, a passphrase or a key file (`internal/keystore`); `SOCRATES_DB_KEY` (64 hex characters) overrides it for a shell that holds the key itself (the phone app) and for tests.

## Packages
`internal/secrets` sealing and process hardening · `internal/keystore` where the database key lives · `internal/privacy` log scrubbing · `internal/transport` the door (policy, rate limit, key gate, handshake) · `internal/store` the encrypted store · `internal/llm` Claude and OpenAI · `internal/api` the routes once the database is open · `cmd/socrates-engine` the program.
