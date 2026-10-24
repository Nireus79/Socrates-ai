// Package store is the engine's memory: conversations, messages, summaries, settings and the safety state, in one SQLite file.
//
// The file is a plain SQLite database (a pure-Go driver, no C code) whose content is encrypted field by field (see cipher.go). It lives
// in a private folder (mode 0700, file 0600), uses a write-ahead log, enforces foreign keys, overwrites deleted data
// (secure_delete) and syncs to disk on every commit. A turn is written in one transaction, so a turn that fails keeps nothing.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver" // SQLite translated to plain Go: no C library, no raw system calls (Android blocks some), no JIT
)

const keyCheckValue = "socrates-key-check-v1"

var (
	ErrNotFound = errors.New("not found")
	// ErrNewerSchema is returned when the file was written by a newer version of Socrates: it is left untouched.
	ErrNewerSchema = errors.New("this data was made by a newer version of Socrates")
)

// migrations are applied in order; PRAGMA user_version says how many have been applied. Never edit one that has shipped: add a new one.
var migrations = []string{
	`CREATE TABLE meta (
		key   TEXT PRIMARY KEY,
		value BLOB NOT NULL
	);
	CREATE TABLE conversations (
		id            TEXT PRIMARY KEY,
		philosopher_id TEXT NOT NULL,
		title         BLOB,
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	);
	CREATE INDEX conversations_updated ON conversations(updated_at DESC);
	CREATE TABLE messages (
		id              TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		seq             INTEGER NOT NULL,
		role            TEXT NOT NULL CHECK (role IN ('user', 'philosopher', 'marker')),
		content         BLOB NOT NULL,
		created_at      INTEGER NOT NULL,
		UNIQUE (conversation_id, seq)
	);
	CREATE TABLE summaries (
		conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
		text            BLOB NOT NULL,
		through_seq     INTEGER NOT NULL,
		updated_at      INTEGER NOT NULL
	);
	CREATE TABLE settings (
		key        TEXT PRIMARY KEY,
		value      BLOB NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE TABLE safety_state (
		conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
		pending_checkin INTEGER NOT NULL DEFAULT 0 CHECK (pending_checkin IN (0, 1)),
		reflective_asked INTEGER NOT NULL DEFAULT 0 CHECK (reflective_asked IN (0, 1)),
		updated_at      INTEGER NOT NULL
	);`,
	// 2: a user message may be marked as chosen from suggestions the philosopher offered (it is not yet the user's own conviction)
	`ALTER TABLE messages ADD COLUMN suggested INTEGER NOT NULL DEFAULT 0 CHECK (suggested IN (0, 1));`,
	// 3: the ledger, what a philosopher remembers of the person (claims, terms, tensions), each with the person's own words as evidence.
	// It belongs to one philosopher and is deleted with the conversation it was learnt in.
	`CREATE TABLE ledger (
		id              TEXT PRIMARY KEY,
		philosopher_id  TEXT NOT NULL,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		kind            TEXT NOT NULL CHECK (kind IN ('claim', 'term', 'tension')),
		text            BLOB NOT NULL,
		detail          BLOB,
		quote           BLOB NOT NULL,
		suggested       INTEGER NOT NULL DEFAULT 0 CHECK (suggested IN (0, 1)),
		status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retracted', 'resolved')),
		a_id            TEXT,
		b_id            TEXT,
		created_at      INTEGER NOT NULL,
		updated_at      INTEGER NOT NULL
	);
	CREATE INDEX ledger_by_philosopher ON ledger(philosopher_id, created_at);`,
	// 4: inquiries and challenges. An inquiry is the question under examination in a conversation (it follows the person as the question
	// deepens); the ledger entries it rests on are linked to it, and each entry says whether it has been examined (0 no, 1 in the person's
	// own words, 2 from a wording chosen among suggestions). A finished inquiry is a challenge: the challenge row holds no text, so it
	// stays when the conversation is deleted and counts toward unlocking.
	`ALTER TABLE ledger ADD COLUMN examined INTEGER NOT NULL DEFAULT 0 CHECK (examined IN (0, 1, 2));
	UPDATE ledger SET examined = 1 WHERE kind = 'term' AND length(detail) > 0;
	CREATE TABLE inquiry (
		id              TEXT PRIMARY KEY,
		philosopher_id  TEXT NOT NULL,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		parent_id       TEXT,
		question        BLOB NOT NULL,
		quote           BLOB NOT NULL,
		banked          INTEGER NOT NULL DEFAULT 0,
		completed       INTEGER NOT NULL DEFAULT 0 CHECK (completed IN (0, 1)),
		announced       INTEGER NOT NULL DEFAULT 0 CHECK (announced IN (0, 1)),
		created_at      INTEGER NOT NULL,
		updated_at      INTEGER NOT NULL
	);
	CREATE INDEX inquiry_by_conversation ON inquiry(conversation_id, created_at);
	CREATE TABLE inquiry_entry (
		inquiry_id TEXT NOT NULL REFERENCES inquiry(id) ON DELETE CASCADE,
		entry_id   TEXT NOT NULL REFERENCES ledger(id) ON DELETE CASCADE,
		PRIMARY KEY (inquiry_id, entry_id)
	);
	CREATE TABLE challenge (
		id             TEXT PRIMARY KEY,
		philosopher_id TEXT NOT NULL,
		inquiry_id     TEXT NOT NULL UNIQUE,
		created_at     INTEGER NOT NULL
	);
	CREATE INDEX challenge_by_philosopher ON challenge(philosopher_id);`,
	// 5: facts the person tells about themselves (a new ledger kind, so the table is rebuilt: its check cannot be altered; the links to
	// inquiries are saved and put back because dropping the old table would cascade them away), challenges the philosopher poses
	// (offers: the topic is sealed, the pitch is an ordinary message) and the mark of an inquiry that began as an accepted challenge.
	`CREATE TABLE ie_save AS SELECT inquiry_id, entry_id FROM inquiry_entry;
	CREATE TABLE ledger_new (
		id              TEXT PRIMARY KEY,
		philosopher_id  TEXT NOT NULL,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		kind            TEXT NOT NULL CHECK (kind IN ('claim', 'term', 'tension', 'fact')),
		text            BLOB NOT NULL,
		detail          BLOB,
		quote           BLOB NOT NULL,
		suggested       INTEGER NOT NULL DEFAULT 0 CHECK (suggested IN (0, 1)),
		status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retracted', 'resolved')),
		a_id            TEXT,
		b_id            TEXT,
		created_at      INTEGER NOT NULL,
		updated_at      INTEGER NOT NULL,
		examined        INTEGER NOT NULL DEFAULT 0 CHECK (examined IN (0, 1, 2))
	);
	INSERT INTO ledger_new(id, philosopher_id, conversation_id, kind, text, detail, quote, suggested, status, a_id, b_id, created_at, updated_at, examined)
		SELECT id, philosopher_id, conversation_id, kind, text, detail, quote, suggested, status, a_id, b_id, created_at, updated_at, examined FROM ledger;
	DROP TABLE ledger;
	ALTER TABLE ledger_new RENAME TO ledger;
	CREATE INDEX ledger_by_philosopher ON ledger(philosopher_id, created_at);
	INSERT INTO inquiry_entry(inquiry_id, entry_id) SELECT inquiry_id, entry_id FROM ie_save;
	DROP TABLE ie_save;
	ALTER TABLE inquiry ADD COLUMN challenge INTEGER NOT NULL DEFAULT 0 CHECK (challenge IN (0, 1));
	ALTER TABLE conversations ADD COLUMN challenge_hint INTEGER NOT NULL DEFAULT 0 CHECK (challenge_hint IN (0, 1));
	CREATE TABLE offer (
		id              TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		philosopher_id  TEXT NOT NULL,
		topic           BLOB NOT NULL,
		state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'accepted', 'declined')),
		seq             INTEGER NOT NULL DEFAULT 0,
		created_at      INTEGER NOT NULL
	);
	CREATE INDEX offer_by_conversation ON offer(conversation_id, created_at);`,
	// 6: what a philosopher learned may outlive the conversation it was learnt in, when the person chooses so at the moment of deleting it:
	// the ledger's link to its conversation becomes optional (cleared, not cascaded). The table is rebuilt once more, saving the links
	// to inquiries as before. The ordinary deletion of a conversation still removes its entries (see DeleteConversation).
	`CREATE TABLE ie_save AS SELECT inquiry_id, entry_id FROM inquiry_entry;
	CREATE TABLE ledger_new (
		id              TEXT PRIMARY KEY,
		philosopher_id  TEXT NOT NULL,
		conversation_id TEXT REFERENCES conversations(id) ON DELETE SET NULL,
		kind            TEXT NOT NULL CHECK (kind IN ('claim', 'term', 'tension', 'fact')),
		text            BLOB NOT NULL,
		detail          BLOB,
		quote           BLOB NOT NULL,
		suggested       INTEGER NOT NULL DEFAULT 0 CHECK (suggested IN (0, 1)),
		status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retracted', 'resolved')),
		a_id            TEXT,
		b_id            TEXT,
		created_at      INTEGER NOT NULL,
		updated_at      INTEGER NOT NULL,
		examined        INTEGER NOT NULL DEFAULT 0 CHECK (examined IN (0, 1, 2))
	);
	INSERT INTO ledger_new(id, philosopher_id, conversation_id, kind, text, detail, quote, suggested, status, a_id, b_id, created_at, updated_at, examined)
		SELECT id, philosopher_id, conversation_id, kind, text, detail, quote, suggested, status, a_id, b_id, created_at, updated_at, examined FROM ledger;
	DROP TABLE ledger;
	ALTER TABLE ledger_new RENAME TO ledger;
	CREATE INDEX ledger_by_philosopher ON ledger(philosopher_id, created_at);
	INSERT INTO inquiry_entry(inquiry_id, entry_id) SELECT inquiry_id, entry_id FROM ie_save;
	DROP TABLE ie_save;`,
	// 7: what the person has bought, been given or earned by a pass, as product ids from a source (a store, a licence, an ad); a grant may expire.
	`CREATE TABLE grants (
		product_id TEXT NOT NULL,
		source     TEXT NOT NULL,
		expires_at INTEGER,
		PRIMARY KEY (product_id, source)
	);`,
	// 8: an entry a deepening inquiry took over from earlier work is marked, so that work already counted is not counted again.
	`ALTER TABLE inquiry_entry ADD COLUMN inherited INTEGER NOT NULL DEFAULT 0;`,
	// 9: how many of the person's turns an inquiry has been examined over (a question is not thought through in two messages).
	`ALTER TABLE inquiry ADD COLUMN turns INTEGER NOT NULL DEFAULT 0;`,
}

// dbtx is what both *sql.DB and *sql.Tx offer.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Ops are the operations on the store. On a Store each one is its own transaction; inside Update they share one.
type Ops struct {
	q   dbtx
	c   *fieldCipher
	now func() time.Time
}

// Store is an open database.
type Store struct {
	Ops
	db   *sql.DB
	path string
}

// Open opens (and, when new, creates) the database at path with the 32-byte database key from the keystore. A wrong key gives
// ErrWrongKey and changes nothing; a file from a newer version gives ErrNewerSchema and changes nothing.
func Open(path string, key []byte) (*Store, error) {
	c, err := newFieldCipher(key)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	os.Chmod(dir, 0o700)
	// Create the database, and the shared-memory file that goes beside it, private before SQLite does (the driver gives the shared-memory
	// file the default mode; the write-ahead log follows the database's mode through the "modeof" option below).
	for _, p := range []string{path, path + "-shm"} {
		f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, err
		}
		f.Close()
		os.Chmod(p, 0o600)
	}
	q := url.Values{}
	for _, p := range []string{"journal_mode(WAL)", "foreign_keys(1)", "busy_timeout(5000)", "secure_delete(1)", "synchronous(FULL)", "trusted_schema(0)"} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	q.Set("modeof", filepath.ToSlash(path)) // the side files (write-ahead log, shared memory) get the database file's private mode
	db, err := sql.Open("sqlite3", (&url.URL{Scheme: "file", Opaque: filepath.ToSlash(path), RawQuery: q.Encode()}).String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db, path: path, Ops: Ops{q: db, c: c, now: time.Now}}
	if err := s.prepare(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) prepare(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("cannot read the schema version: %w", err)
	}
	if version > len(migrations) {
		return ErrNewerSchema
	}
	if version > 0 {
		if err := s.checkKey(ctx); err != nil {
			return err
		}
	}
	for v := version; v < len(migrations); v++ {
		err := s.Update(ctx, func(o *Ops) error {
			if _, err := o.q.ExecContext(ctx, migrations[v]); err != nil {
				return err
			}
			if v == 0 {
				blob, err := o.c.seal("meta", "value", "key_check", []byte(keyCheckValue))
				if err != nil {
					return err
				}
				if _, err := o.q.ExecContext(ctx, "INSERT INTO meta(key, value) VALUES ('key_check', ?)", blob); err != nil {
					return err
				}
			}
			_, err := o.q.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1))
			return err
		})
		if err != nil {
			return fmt.Errorf("cannot prepare the database (step %d): %w", v+1, err)
		}
	}
	return nil
}

func (s *Store) checkKey(ctx context.Context) error {
	var blob []byte
	if err := s.db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = 'key_check'").Scan(&blob); err != nil {
		return ErrWrongKey
	}
	plain, err := s.c.open("meta", "value", "key_check", blob)
	if err != nil || string(plain) != keyCheckValue {
		return ErrWrongKey
	}
	return nil
}

// Path is the database file.
func (s *Store) Path() string { return s.path }

// Close closes the database after folding the log into it.
func (s *Store) Close() error {
	s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return s.db.Close()
}

// Update runs fn in one transaction: it commits when fn returns nil and rolls everything back otherwise.
func (s *Store) Update(ctx context.Context, fn func(*Ops) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Ops{q: tx, c: s.c, now: s.now}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Wipe removes everything the store holds: every row is deleted, the write-ahead log is folded in and the file is compacted, so the
// freed space does not keep old data. The caller deletes the file and the key as well when a profile is deleted.
func (s *Store) Wipe(ctx context.Context) error {
	err := s.Update(ctx, func(o *Ops) error {
		for _, t := range []string{"grants", "offer", "challenge", "inquiry_entry", "inquiry", "ledger", "safety_state", "summaries", "messages", "conversations", "settings"} {
			if _, err := o.q.ExecContext(ctx, "DELETE FROM "+t); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "VACUUM")
	return err
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (o *Ops) stamp() int64 { return o.now().Unix() }

// ---- conversations

type Conversation struct {
	ID            string
	PhilosopherID string
	Title         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (o *Ops) CreateConversation(ctx context.Context, philosopherID string) (Conversation, error) {
	if philosopherID == "" {
		return Conversation{}, errors.New("a conversation needs a philosopher")
	}
	id, now := newID(), o.stamp()
	if _, err := o.q.ExecContext(ctx, "INSERT INTO conversations(id, philosopher_id, created_at, updated_at) VALUES (?, ?, ?, ?)", id, philosopherID, now, now); err != nil {
		return Conversation{}, err
	}
	return Conversation{ID: id, PhilosopherID: philosopherID, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (o *Ops) scanConversation(sc interface{ Scan(...any) error }) (Conversation, error) {
	var c Conversation
	var title []byte
	var created, updated int64
	if err := sc.Scan(&c.ID, &c.PhilosopherID, &title, &created, &updated); err != nil {
		return Conversation{}, err
	}
	c.CreatedAt, c.UpdatedAt = time.Unix(created, 0), time.Unix(updated, 0)
	if title != nil {
		plain, err := o.c.open("conversations", "title", c.ID, title)
		if err != nil {
			return Conversation{}, fmt.Errorf("conversation %s: %w", c.ID, err)
		}
		c.Title = string(plain)
	}
	return c, nil
}

func (o *Ops) Conversation(ctx context.Context, id string) (Conversation, error) {
	c, err := o.scanConversation(o.q.QueryRowContext(ctx, "SELECT id, philosopher_id, title, created_at, updated_at FROM conversations WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return c, err
}

// Conversations lists them, the most recently active first.
func (o *Ops) Conversations(ctx context.Context) ([]Conversation, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT id, philosopher_id, title, created_at, updated_at FROM conversations ORDER BY updated_at DESC, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		c, err := o.scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (o *Ops) SetTitle(ctx context.Context, id, title string) error {
	blob, err := o.c.seal("conversations", "title", id, []byte(title))
	if err != nil {
		return err
	}
	res, err := o.q.ExecContext(ctx, "UPDATE conversations SET title = ? WHERE id = ?", blob, id)
	return affected(res, err)
}

// DeleteConversation removes a conversation with its messages, summary, safety state and everything the philosophers learnt in it.
// Call it inside Store.Update: the entries, the tensions that rest on them and the conversation go together or not at all.
func (o *Ops) DeleteConversation(ctx context.Context, id string) error {
	// A tension may join a claim of this conversation to one of another; it describes the forgotten claim, so it goes too.
	if _, err := o.q.ExecContext(ctx, "DELETE FROM ledger WHERE kind = 'tension' AND (a_id IN (SELECT id FROM ledger WHERE conversation_id = ?) OR b_id IN (SELECT id FROM ledger WHERE conversation_id = ?))", id, id); err != nil {
		return err
	}
	if _, err := o.q.ExecContext(ctx, "DELETE FROM ledger WHERE conversation_id = ?", id); err != nil {
		return err
	}
	res, err := o.q.ExecContext(ctx, "DELETE FROM conversations WHERE id = ?", id)
	return affected(res, err)
}

// DeleteConversationKeepingMemory removes a conversation but leaves what was learnt in it: the entries stay with their philosopher, no
// longer tied to any conversation, until the person deletes them.
func (o *Ops) DeleteConversationKeepingMemory(ctx context.Context, id string) error {
	res, err := o.q.ExecContext(ctx, "DELETE FROM conversations WHERE id = ?", id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- messages

type Message struct {
	ID             string
	ConversationID string
	Seq            int
	Role           string // "user", "philosopher", or "marker" (a refusal, a crisis step or an age end: only the kind is kept, never the words)
	Content        string
	CreatedAt      time.Time
	Suggested      bool // a user message chosen from the philosopher's suggestions
}

// AppendMessage adds a message at the end of the conversation.
func (o *Ops) AppendMessage(ctx context.Context, conversationID, role, content string) (Message, error) {
	return o.appendMessage(ctx, conversationID, role, content, false)
}

// AppendSuggested adds a user message that was chosen from suggestions.
func (o *Ops) AppendSuggested(ctx context.Context, conversationID, content string) (Message, error) {
	return o.appendMessage(ctx, conversationID, "user", content, true)
}

func (o *Ops) appendMessage(ctx context.Context, conversationID, role, content string, suggested bool) (Message, error) {
	if role != "user" && role != "philosopher" && role != "marker" {
		return Message{}, fmt.Errorf("unknown role %q", role)
	}
	id, now := newID(), o.stamp()
	blob, err := o.c.seal("messages", "content", id, []byte(content))
	if err != nil {
		return Message{}, err
	}
	// The sequence number is chosen inside the INSERT, so two writers can never take the same one.
	var seq int
	flag := 0
	if suggested {
		flag = 1
	}
	err = o.q.QueryRowContext(ctx, `INSERT INTO messages(id, conversation_id, seq, role, content, created_at, suggested)
		VALUES (?, ?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE conversation_id = ?), ?, ?, ?, ?) RETURNING seq`,
		id, conversationID, conversationID, role, blob, now, flag).Scan(&seq)
	if err != nil {
		return Message{}, err
	}
	if res, err := o.q.ExecContext(ctx, "UPDATE conversations SET updated_at = ? WHERE id = ?", now, conversationID); affected(res, err) != nil {
		return Message{}, affected(res, err)
	}
	return Message{ID: id, ConversationID: conversationID, Seq: seq, Role: role, Content: content, CreatedAt: time.Unix(now, 0), Suggested: suggested}, nil
}

func (o *Ops) scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var blob []byte
		var created, flag int64
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.Role, &blob, &created, &flag); err != nil {
			return nil, err
		}
		plain, err := o.c.open("messages", "content", m.ID, blob)
		if err != nil {
			return nil, fmt.Errorf("message %s: %w", m.ID, err)
		}
		m.Content, m.CreatedAt, m.Suggested = string(plain), time.Unix(created, 0), flag == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

// Messages returns the whole conversation in order.
func (o *Ops) Messages(ctx context.Context, conversationID string) ([]Message, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT id, conversation_id, seq, role, content, created_at, suggested FROM messages WHERE conversation_id = ? ORDER BY seq", conversationID)
	if err != nil {
		return nil, err
	}
	return o.scanMessages(rows)
}

// RecentMessages returns the last n messages, in order.
func (o *Ops) RecentMessages(ctx context.Context, conversationID string, n int) ([]Message, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT id, conversation_id, seq, role, content, created_at, suggested FROM (SELECT * FROM messages WHERE conversation_id = ? ORDER BY seq DESC LIMIT ?) ORDER BY seq", conversationID, n)
	if err != nil {
		return nil, err
	}
	return o.scanMessages(rows)
}

// MessagesAfter returns the messages with a sequence number above afterSeq, in order (what a summary has not yet covered).
func (o *Ops) MessagesAfter(ctx context.Context, conversationID string, afterSeq int) ([]Message, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT id, conversation_id, seq, role, content, created_at, suggested FROM messages WHERE conversation_id = ? AND seq > ? ORDER BY seq", conversationID, afterSeq)
	if err != nil {
		return nil, err
	}
	return o.scanMessages(rows)
}

// FirstMessages returns the first n messages of a conversation, in order.
func (o *Ops) FirstMessages(ctx context.Context, conversationID string, n int) ([]Message, error) {
	rows, err := o.q.QueryContext(ctx, "SELECT id, conversation_id, seq, role, content, created_at, suggested FROM messages WHERE conversation_id = ? ORDER BY seq LIMIT ?", conversationID, n)
	if err != nil {
		return nil, err
	}
	return o.scanMessages(rows)
}

// ---- summaries

// SetSummary keeps the running summary of a conversation, written after message throughSeq.
func (o *Ops) SetSummary(ctx context.Context, conversationID, text string, throughSeq int) error {
	blob, err := o.c.seal("summaries", "text", conversationID, []byte(text))
	if err != nil {
		return err
	}
	_, err = o.q.ExecContext(ctx, `INSERT INTO summaries(conversation_id, text, through_seq, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET text = excluded.text, through_seq = excluded.through_seq, updated_at = excluded.updated_at`,
		conversationID, blob, throughSeq, o.stamp())
	return err
}

// Summary returns the summary and the message it runs up to; ok is false when there is none yet.
func (o *Ops) Summary(ctx context.Context, conversationID string) (text string, throughSeq int, ok bool, err error) {
	var blob []byte
	err = o.q.QueryRowContext(ctx, "SELECT text, through_seq FROM summaries WHERE conversation_id = ?", conversationID).Scan(&blob, &throughSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	plain, err := o.c.open("summaries", "text", conversationID, blob)
	if err != nil {
		return "", 0, false, fmt.Errorf("summary of %s: %w", conversationID, err)
	}
	return string(plain), throughSeq, true, nil
}

// ---- settings (provider choice, model roles, credentials: all encrypted)

func (o *Ops) SetSetting(ctx context.Context, key, value string) error {
	blob, err := o.c.seal("settings", "value", key, []byte(value))
	if err != nil {
		return err
	}
	_, err = o.q.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, blob, o.stamp())
	return err
}

func (o *Ops) Setting(ctx context.Context, key string) (value string, ok bool, err error) {
	var blob []byte
	err = o.q.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plain, err := o.c.open("settings", "value", key, blob)
	if err != nil {
		return "", false, fmt.Errorf("setting %s: %w", key, err)
	}
	return string(plain), true, nil
}

func (o *Ops) DeleteSetting(ctx context.Context, key string) error {
	_, err := o.q.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", key)
	return err
}

// ---- safety state

// SafetyState is what the safety stage remembers about a conversation.
type SafetyState struct {
	PendingCheckin          bool // the next turn must open with the fixed check-in (after a crisis refusal)
	ReflectiveQuestionAsked bool // the one reflective question before refusing an illegal request has been used
}

// SetSafetyState records the state of a conversation.
func (o *Ops) SetSafetyState(ctx context.Context, conversationID string, st SafetyState) error {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err := o.q.ExecContext(ctx, `INSERT INTO safety_state(conversation_id, pending_checkin, reflective_asked, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET pending_checkin = excluded.pending_checkin, reflective_asked = excluded.reflective_asked, updated_at = excluded.updated_at`,
		conversationID, b(st.PendingCheckin), b(st.ReflectiveQuestionAsked), o.stamp())
	return err
}

// SafetyState returns the state of a conversation; a conversation with none recorded has the zero state.
func (o *Ops) SafetyState(ctx context.Context, conversationID string) (SafetyState, error) {
	var p, r int
	err := o.q.QueryRowContext(ctx, "SELECT pending_checkin, reflective_asked FROM safety_state WHERE conversation_id = ?", conversationID).Scan(&p, &r)
	if errors.Is(err, sql.ErrNoRows) {
		return SafetyState{}, nil
	}
	return SafetyState{PendingCheckin: p == 1, ReflectiveQuestionAsked: r == 1}, err
}

// ClearSummary forgets the running summary of a conversation (it is written again from what remains when it is next due).
func (o *Ops) ClearSummary(ctx context.Context, conversationID string) error {
	_, err := o.q.ExecContext(ctx, "DELETE FROM summaries WHERE conversation_id = ?", conversationID)
	return err
}

// ErrNotAReply is returned when something other than a philosopher's reply is reported.
var ErrNotAReply = errors.New("only a philosopher's reply can be reported")

// ReportMessage takes a philosopher's reply out of a conversation: it becomes a marker of the kind "reported" and its words are gone from the
// store. The words are returned once, so that the person can keep a copy of the report if they choose.
func (o *Ops) ReportMessage(ctx context.Context, conversationID, messageID string) (string, error) {
	msgs, err := o.Messages(ctx, conversationID)
	if err != nil {
		return "", err
	}
	for _, m := range msgs {
		if m.ID != messageID {
			continue
		}
		if m.Role != "philosopher" {
			return "", ErrNotAReply
		}
		blob, err := o.c.seal("messages", "content", m.ID, []byte("reported"))
		if err != nil {
			return "", err
		}
		res, err := o.q.ExecContext(ctx, "UPDATE messages SET role = 'marker', content = ? WHERE id = ? AND conversation_id = ?", blob, m.ID, conversationID)
		return m.Content, affected(res, err)
	}
	return "", ErrNotFound
}
