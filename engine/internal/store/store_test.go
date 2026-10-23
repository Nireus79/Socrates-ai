package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

func open(t *testing.T) (*Store, string, []byte) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(dir, "socrates.db")
	key := testKey(7)
	s, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path, key
}

func mustConv(t *testing.T, o *Ops, philosopher string) Conversation {
	t.Helper()
	c, err := o.CreateConversation(ctx, philosopher)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheFolderAndTheFilesAreCreatedPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not apply on Windows")
	}
	s, path, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	s.AppendMessage(ctx, c.ID, "user", "hello") // makes the log and the shared-memory file
	for _, p := range []string{filepath.Dir(path), path, path + "-wal", path + "-shm"} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if st.IsDir() {
			want = 0o700
		}
		if st.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s has mode %v, want %v: other users could read it", p, st.Mode().Perm(), want)
		}
	}
}

func TestReopeningNeedsTheSameKeyAndAWrongKeyChangesNothing(t *testing.T) {
	s, path, key := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	s.AppendMessage(ctx, c.ID, "user", "remember this")
	s.Close()
	wrong := testKey(9)
	if _, err := Open(path, wrong); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("a wrong key: %v", err)
	}
	if _, err := Open(path, key[:16]); err == nil {
		t.Fatal("a short key was accepted")
	}
	s2, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	msgs, err := s2.Messages(ctx, c.ID)
	if err != nil || len(msgs) != 1 || msgs[0].Content != "remember this" {
		t.Fatalf("after the refused attempts: %v %v", msgs, err)
	}
}

func TestADatabaseFromANewerVersionIsRefusedAndLeftAlone(t *testing.T) {
	s, path, key := open(t)
	s.Close()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	raw.Exec("PRAGMA user_version = 99")
	raw.Close()
	if _, err := Open(path, key); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("newer schema: %v", err)
	}
	raw, _ = sql.Open("sqlite", "file:"+path)
	defer raw.Close()
	var v int
	raw.QueryRow("PRAGMA user_version").Scan(&v)
	if v != 99 {
		t.Fatalf("the refused file was changed: user_version = %d", v)
	}
}

func TestConversationsAndTheirTitles(t *testing.T) {
	s, _, _ := open(t)
	base := time.Unix(1_700_000_000, 0)
	tick := 0
	s.now = func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Minute) }
	a := mustConv(t, &s.Ops, "socrates")
	b := mustConv(t, &s.Ops, "diogenes")
	if err := s.SetTitle(ctx, a.ID, "On courage"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Conversation(ctx, a.ID)
	if got.Title != "On courage" || got.PhilosopherID != "socrates" {
		t.Fatalf("%+v", got)
	}
	s.AppendMessage(ctx, a.ID, "user", "later message") // makes a the most recent
	list, _ := s.Conversations(ctx)
	if len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("the most recent comes first: %+v", list)
	}
	if _, err := s.Conversation(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown conversation: %v", err)
	}
	if err := s.SetTitle(ctx, "nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("title of an unknown conversation: %v", err)
	}
	if _, err := s.CreateConversation(ctx, ""); err == nil {
		t.Fatal("a conversation without a philosopher was created")
	}
}

func TestMessagesGetSequenceNumbersAndComeBackInOrder(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	for i := 1; i <= 5; i++ {
		role := "user"
		if i%2 == 0 {
			role = "philosopher"
		}
		m, err := s.AppendMessage(ctx, c.ID, role, fmt.Sprintf("message %d", i))
		if err != nil || m.Seq != i {
			t.Fatalf("message %d: %+v %v", i, m, err)
		}
	}
	all, _ := s.Messages(ctx, c.ID)
	recent, _ := s.RecentMessages(ctx, c.ID, 2)
	if len(all) != 5 || all[0].Content != "message 1" || all[4].Content != "message 5" {
		t.Fatalf("all: %+v", all)
	}
	if len(recent) != 2 || recent[0].Seq != 4 || recent[1].Seq != 5 {
		t.Fatalf("recent: %+v", recent)
	}
	if _, err := s.AppendMessage(ctx, c.ID, "narrator", "x"); err == nil {
		t.Fatal("an unknown role was accepted")
	}
	if _, err := s.AppendMessage(ctx, "no-such-conversation", "user", "x"); err == nil {
		t.Fatal("a message for an unknown conversation was stored")
	}
	if n, _ := s.Messages(ctx, "no-such-conversation"); len(n) != 0 {
		t.Fatal("a message appeared under an unknown conversation")
	}
}

func TestConcurrentWritersNeverTakeTheSameSequenceNumber(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if _, err := s.AppendMessage(ctx, c.ID, "user", "x"); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	all, _ := s.Messages(ctx, c.ID)
	if len(all) != 200 {
		t.Fatalf("%d messages", len(all))
	}
	for i, m := range all {
		if m.Seq != i+1 {
			t.Fatalf("message %d has seq %d", i, m.Seq)
		}
	}
}

func TestATurnIsOneTransactionAndAFailedTurnKeepsNothing(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	failure := errors.New("the model failed")
	err := s.Update(ctx, func(o *Ops) error {
		if _, err := o.AppendMessage(ctx, c.ID, "user", "a question"); err != nil {
			return err
		}
		o.SetSummary(ctx, c.ID, "a summary", 1)
		o.SetPendingCheckin(ctx, c.ID, true)
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Update returned %v", err)
	}
	msgs, _ := s.Messages(ctx, c.ID)
	_, _, hasSummary, _ := s.Summary(ctx, c.ID)
	pending, _ := s.PendingCheckin(ctx, c.ID)
	if len(msgs) != 0 || hasSummary || pending {
		t.Fatalf("a failed turn left something: %d messages, summary %v, check-in %v", len(msgs), hasSummary, pending)
	}
	err = s.Update(ctx, func(o *Ops) error {
		o.AppendMessage(ctx, c.ID, "user", "a question")
		o.AppendMessage(ctx, c.ID, "philosopher", "an answer")
		return o.SetSummary(ctx, c.ID, "a summary", 2)
	})
	msgs, _ = s.Messages(ctx, c.ID)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("a good turn: %v %d", err, len(msgs))
	}
}

func TestSummariesSettingsAndTheSafetyState(t *testing.T) {
	s, _, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	if _, _, ok, _ := s.Summary(ctx, c.ID); ok {
		t.Fatal("a summary before any was written")
	}
	s.SetSummary(ctx, c.ID, "first", 2)
	s.SetSummary(ctx, c.ID, "second", 4)
	if text, through, ok, err := s.Summary(ctx, c.ID); err != nil || !ok || text != "second" || through != 4 {
		t.Fatalf("summary: %q %d %v %v", text, through, ok, err)
	}
	if _, ok, _ := s.Setting(ctx, "provider"); ok {
		t.Fatal("a setting before any was written")
	}
	s.SetSetting(ctx, "provider", "anthropic")
	s.SetSetting(ctx, "provider", "openai")
	if v, ok, err := s.Setting(ctx, "provider"); err != nil || !ok || v != "openai" {
		t.Fatalf("setting: %q %v %v", v, ok, err)
	}
	s.DeleteSetting(ctx, "provider")
	if _, ok, _ := s.Setting(ctx, "provider"); ok {
		t.Fatal("a deleted setting is still there")
	}
	if p, _ := s.PendingCheckin(ctx, c.ID); p {
		t.Fatal("a check-in is pending by default")
	}
	s.SetPendingCheckin(ctx, c.ID, true)
	if p, _ := s.PendingCheckin(ctx, c.ID); !p {
		t.Fatal("the check-in was not recorded")
	}
	s.SetPendingCheckin(ctx, c.ID, false)
	if p, _ := s.PendingCheckin(ctx, c.ID); p {
		t.Fatal("the check-in was not cleared")
	}
}

func TestDeletingAConversationDeletesEverythingAboutIt(t *testing.T) {
	s, _, _ := open(t)
	a, b := mustConv(t, &s.Ops, "socrates"), mustConv(t, &s.Ops, "plato")
	for _, c := range []Conversation{a, b} {
		s.AppendMessage(ctx, c.ID, "user", "hi")
		s.SetSummary(ctx, c.ID, "s", 1)
		s.SetPendingCheckin(ctx, c.ID, true)
	}
	if err := s.DeleteConversation(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"messages", "summaries", "safety_state"} {
		var n int
		s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE conversation_id = ?", a.ID).Scan(&n)
		if n != 0 {
			t.Fatalf("%s still holds %d rows of the deleted conversation", table, n)
		}
		s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE conversation_id = ?", b.ID).Scan(&n)
		if n != 1 {
			t.Fatalf("%s lost the other conversation's row", table)
		}
	}
	if err := s.DeleteConversation(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting twice: %v", err)
	}
}

// What the user writes must never be readable in the files on disk, not in the database, not in the log, not in the shared memory.
func TestNothingTheUserWritesIsReadableInTheFilesOnDisk(t *testing.T) {
	s, path, _ := open(t)
	markers := []string{"MARKER-MESSAGE-Ωmega-91", "MARKER-TITLE-Ωmega-92", "MARKER-SUMMARY-Ωmega-93", "MARKER-SETTING-Ωmega-94", "MARKER-KEY-sk-ant-95"}
	c := mustConv(t, &s.Ops, "socrates")
	s.AppendMessage(ctx, c.ID, "user", "please keep "+markers[0]+" private")
	s.SetTitle(ctx, c.ID, markers[1])
	s.SetSummary(ctx, c.ID, "the user said "+markers[2], 1)
	s.SetSetting(ctx, "provider.anthropic.key", markers[4])
	s.SetSetting(ctx, "note", markers[3])
	scan := func(when string) {
		for _, f := range []string{path, path + "-wal", path + "-shm"} {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			for _, m := range markers {
				if bytes.Contains(data, []byte(m)) {
					t.Fatalf("%s: %q is readable in %s", when, m, filepath.Base(f))
				}
			}
		}
	}
	scan("with the log still in use")
	s.Close()
	scan("after closing")
	// The structure that stays readable is only what the threat model says: identifiers, times, roles.
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("socrates")) {
		t.Log("the philosopher id is not visible either")
	}
}

func TestACiphertextMovedOrChangedInTheFileIsNeverReturnedAsText(t *testing.T) {
	s, path, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	m1, _ := s.AppendMessage(ctx, c.ID, "user", "first message")
	m2, _ := s.AppendMessage(ctx, c.ID, "user", "second message")
	d := mustConv(t, &s.Ops, "plato")
	s.SetTitle(ctx, c.ID, "title of c")
	s.SetTitle(ctx, d.ID, "title of d")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	// message 1's ciphertext copied over message 2
	if _, err := raw.Exec("UPDATE messages SET content = (SELECT content FROM messages WHERE id = ?) WHERE id = ?", m1.ID, m2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Messages(ctx, c.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("a moved message ciphertext was accepted: %v", err)
	}
	// a title moved to another conversation
	raw.Exec("UPDATE conversations SET title = (SELECT title FROM conversations WHERE id = ?) WHERE id = ?", c.ID, d.ID)
	if _, err := s.Conversation(ctx, d.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("a moved title was accepted: %v", err)
	}
	if _, err := s.Conversations(ctx); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("the list showed a moved title: %v", err)
	}
	// a changed byte
	raw.Exec("UPDATE messages SET content = X'0100' || substr(content, 3) WHERE id = ?", m1.ID)
	if _, err := s.RecentMessages(ctx, c.ID, 5); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("a changed ciphertext was accepted: %v", err)
	}
}

func TestADeletedValueDoesNotStayInTheFile(t *testing.T) {
	s, path, _ := open(t)
	c := mustConv(t, &s.Ops, "socrates")
	m, _ := s.AppendMessage(ctx, c.ID, "user", strings.Repeat("a long message that will be deleted ", 40))
	var blob []byte
	s.db.QueryRow("SELECT content FROM messages WHERE id = ?", m.ID).Scan(&blob)
	if len(blob) < 100 {
		t.Fatal("unexpected blob")
	}
	if err := s.Wipe(ctx); err != nil {
		t.Fatal(err)
	}
	if msgs, _ := s.Messages(ctx, c.ID); len(msgs) != 0 {
		t.Fatal("the wipe left messages")
	}
	if convs, _ := s.Conversations(ctx); len(convs) != 0 {
		t.Fatal("the wipe left conversations")
	}
	s.Close()
	for _, f := range []string{path, path + "-wal"} {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if bytes.Contains(data, blob[:64]) {
			t.Fatalf("the deleted ciphertext is still in %s", filepath.Base(f))
		}
	}
}
