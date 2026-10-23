# Threat model (M0 draft)

Status: written on 2026-10-23 with the first engine packages (secrets, keystore, privacy, transport, store). It says what is protected, against whom, how, and what is not protected. It grows with the engine; the safety stage (what the philosophers refuse and how a crisis is met) is a separate document, written with that stage.

## 1. What is protected
1. **What the user writes and what the philosophers answer**, and the summaries and claims made from them.
2. **The user's AI-provider key** (and any other credential). The key belongs to the user (PLAN principle 8).
3. **The user's control of their data:** deleting the profile leaves nothing; an export contains only the user's own data.

## 2. Against whom
| Adversary | In scope? |
|---|---|
| Another user account on the same machine; anyone who gets a copy of the data folder or of a backup | **Yes** |
| A web page in the user's browser trying to reach the engine on localhost (including DNS rebinding) | **Yes** |
| Another program of the same user probing the engine's port | **Yes** (needs the per-run token) |
| A thief with the powered-off machine | Partly: the content is encrypted, the key sits in the OS keychain or behind a passphrase; full-disk encryption is the user's and the OS's job |
| Malware running as the same user, with the keychain unlocked | **No.** It can read the keychain, the process memory of a non-hardened platform and the screen. The engine raises the bar (a non-dumpable process on Linux) and does not claim more |
| A malicious or compromised AI provider | **No.** Whatever is sent to the provider is visible to it; the user chose it. Only the text needed for a reply is sent, and nothing from the profile that the reply does not need |
| The developers | The engine has no server, no account, no telemetry, no analytics; nothing is sent anywhere except to the provider the user chose |

## 3. How, by layer
**Keys** (`keystore`, `secrets`). The database key is 32 random bytes. It lives in the OS keychain (Secret Service, Keychain, Credential Manager) or, where there is none, the person chooses once and visibly: a passphrase that seals the key (scrypt, N = 2^17, AES-256-GCM, a passphrase of at least 12 characters, the cost stored and authenticated in the sealed value) or, explicitly less safe, a key file (mode 0600). Nothing is downgraded silently, and a key is never replaced while a database exists (that would lock the data away). Wrong passphrase guesses are slowed down outside the lock. The key files are fsynced and renamed atomically.
**The store** (`store`). A SQLite file in a private folder (0700, files 0600; the engine runs with a umask of 077 and the driver gives its side files the database's mode), written by `github.com/ncruces/go-sqlite3`, which is SQLite translated to plain Go (no C library, no raw system calls, no JIT). Every user-written value and every credential is encrypted before it reaches the file, field by field, with AES-256-GCM under a key derived (HKDF-SHA256) from the database key, a fresh random nonce each, and bound to its table, column and row, so a ciphertext cannot be moved or altered undetected (`ErrCorrupt`). A wrong key is refused without touching the file; a file from a newer version is refused untouched. Deleted data is overwritten (secure delete) and a wipe compacts the file.
**The door** (`transport`). Loopback only, on a port the system chooses; Host must be the loopback address and the engine's own port; a request with an Origin header is refused unless the shell registered that exact origin; every request except the liveness check carries the per-run token (constant-time compare), which is generated at start, printed once to the shell and never written to disk; request bodies, headers and read times are limited; every answer is `no-store` and `nosniff`; panics are logged by place and kind, never by value; the engine stops when its parent closes its input. There is no development mode that relaxes any of this.
**Logs** (`privacy`). First rule: the code does not log content. Second line: a scrubber masks registered strings (user text, claims, replies) in every log line. `SOCRATES_LOG_CONTENT=1` turns masking off for debugging and the engine ignores it in production.

## 4. What stays readable, and what is not protected (known, accepted for now)
- **Metadata in the database file:** identifiers, creation and update times, the philosopher a conversation is with, a message's role, and the length of every value. Someone who copies the file learns when and how much the user talked and with whom, not what. Full-database encryption (SQLCipher) would hide this; it was not chosen because the available driver is unmaintained and its forks are single-maintainer C code (see the commit message of the store). It can be reconsidered.
- **Rollback:** an attacker with write access to the file can replace a whole row's ciphertext with an older ciphertext of the same row, or delete rows; individual values cannot be forged or moved, but a replayed old value is valid. Not defended.
- **Windows and macOS process hardening:** the non-dumpable process exists on Linux only. macOS (`PT_DENY_ATTACH`) and Windows (a restricted token) are to be done with the platform work (BUILD_PLAN M5).
- **Passphrase mode:** the sealed key sits in a file; a thief can guess offline. Length (12 or more) and the scrypt cost are the only protection. The keychain mode is better and is the default whenever there is a keychain.
- **The environment key:** `SOCRATES_DB_KEY` supplies the key from the environment, for a shell that holds the key itself (the phone app) and for development. Anything that can set the engine's environment can set it; it is never offered to a person.
- **Same-user malware, the screen, the clipboard:** see section 2.
- **The user's own key in the provider's hands:** the engine sends it only to the provider endpoint the user chose; whether a provider logs requests is the provider's policy.

## 5. Checks that gate every merge (so far)
Each protection above has a test that fails when the protection is removed; this was verified by breaking each one on purpose (origin, host, token, panic logging in the door; encryption, row binding, secure delete, file modes, transaction rollback and sequence numbers in the store). Still to write: the test that fails if user text reaches a log, once the conversation exists (BUILD_PLAN M0), and the independent review of the whole path (M4).
