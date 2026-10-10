package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func syntheticAttachment() *Attachment {
	return &Attachment{Kind: "image", ImageHex: strings.Repeat("a", 32), Query: "synthetic-query"}
}
func attachmentEntry(sc Scope, id string) InboxEntry {
	return InboxEntry{Scope: sc, MessageID: id, ReceivedAt: time.UnixMilli(1), Attachment: syntheticAttachment()}
}
func resolveAttachment(t *testing.T, s *Store, e InboxEntry) {
	t.Helper()
	ctx := context.Background()
	cur, err := s.Cursor(ctx, e.Scope)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RecordPoll(ctx, e.Scope, cur, []byte("synthetic raw"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResolvePoll(ctx, e.Scope, id, cur+"x", []InboxEntry{e}); err != nil {
		t.Fatal(err)
	}
}
func TestAttachmentInboxAtomicRoundTripAndDuplicate(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	sc := Scope{"synthetic-a", "synthetic-u"}
	e := attachmentEntry(sc, "original-id")
	resolveAttachment(t, s, e)
	duplicate := e
	duplicate.Text = "changed"
	duplicate.Attachment = &Attachment{Kind: "unsupported"}
	resolveAttachment(t, s, duplicate)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if reopened != nil {
		t.Cleanup(func() { reopened.Close() })
	}
	if err != nil {
		t.Fatal(err)
	}
	s = reopened
	got, err := s.PendingInbox(ctx, sc, 32)
	if err != nil || len(got) != 1 || got[0].Text != "" || got[0].Attachment == nil || *got[0].Attachment != *e.Attachment || got[0].MessageID != e.MessageID {
		t.Fatalf("durable unchanged descriptor: %#v %v", got, err)
	}
	for _, state := range []string{"processing", "sending", "sent", "failed", "ambiguous"} {
		id := "claimed-" + state
		if _, err = s.db.Exec(`INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES(?,?,?,'',?,1,1)`, sc.Account, sc.User, id, state); err != nil {
			t.Fatal(err)
		}
		resolveAttachment(t, s, attachmentEntry(sc, id))
	}
	got, err = s.PendingInbox(ctx, sc, 32)
	if err != nil || len(got) != 1 {
		t.Fatalf("all-state dedup: %d %v", len(got), err)
	}
	other := Scope{"synthetic-b", "synthetic-u"}
	resolveAttachment(t, s, attachmentEntry(other, "original-id"))
	got, err = s.PendingInbox(ctx, other, 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("scoped identity: %v", err)
	}
}
func fillAttachmentQuota(t *testing.T, s *Store, remaining int) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback() })
	total := (8 << 20) - remaining
	for i := 0; total > 0; i++ {
		n := min(total, 16384)
		if _, err = tx.Exec(`INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('quota','u',?,?,'',1)`, fmt.Sprint(i), strings.Repeat("x", n)); err != nil {
			t.Fatal(err)
		}
		total -= n
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
func TestAttachmentQuotaIncludesMetadata(t *testing.T) {
	for _, over := range []bool{false, true} {
		t.Run(fmt.Sprint(over), func(t *testing.T) {
			s, _ := testStore(t)
			ctx := context.Background()
			sc := Scope{"synthetic", "u"}
			e := attachmentEntry(sc, "last")
			body, err := encodeAttachment(e.Attachment)
			if err != nil {
				t.Fatal(err)
			}
			space := len(body)
			if over {
				space--
			}
			fillAttachmentQuota(t, s, space)
			id, err := s.RecordPoll(ctx, sc, "", []byte("original raw"))
			if err != nil {
				t.Fatal(err)
			}
			err = s.ResolvePoll(ctx, sc, id, "next", []InboxEntry{e})
			if over {
				if !errors.Is(err, ErrCapacity) {
					t.Fatalf("over quota: %v", err)
				}
				f, err := s.PendingPoll(ctx, sc)
				if err != nil || f == nil || string(f.Body) != "original raw" {
					t.Fatalf("raw retention: %v", err)
				}
				cur, err := s.Cursor(ctx, sc)
				if err != nil || cur != "" {
					t.Fatalf("cursor changed %q %v", cur, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				_, size, err := receiveQueueBudget(ctx, s.db)
				if err != nil || size != 8<<20 {
					t.Fatalf("exact combined quota %d %v", size, err)
				}
			}
		})
	}
}
func TestAttachmentAdmissionBeforeMaterialization(t *testing.T) {
	cases := []struct {
		name   string
		body   []byte
		orphan bool
	}{
		{"oversize", []byte(strings.Repeat("x", 16385)), false}, {"orphan", []byte(`{}`), true}, {"invalid", []byte(`{}`), false},
		{"null", []byte(`null`), false}, {"unknown", []byte(`{"kind":"unsupported","extra":"x"}`), false},
	}
	valid, _ := encodeAttachment(&Attachment{Kind: "unsupported"})
	cases = append(cases, struct {
		name   string
		body   []byte
		orphan bool
	}{"duplicate", []byte(strings.Replace(string(valid), `"kind":"unsupported"`, `"kind":"unsupported","kind":"unsupported"`, 1)), false}, struct {
		name   string
		body   []byte
		orphan bool
	}{"alias", []byte(strings.Replace(string(valid), `"kind"`, `"\u006bind"`, 1)), false})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := testStore(t)
			ctx := context.Background()
			if _, err := s.db.Exec(`INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','id','', '',1)`); err != nil {
				t.Fatal(err)
			}
			seq := 1
			if tc.orphan {
				seq = 999
				if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.db.Exec(`INSERT INTO inbox_attachments VALUES(?,?)`, seq, tc.body); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PendingInbox(ctx, Scope{"a", "u"}, 1); !errors.Is(err, ErrInvalid) {
				t.Fatalf("malformed admitted: %v", err)
			}
		})
	}
	// Aggregate admission must precede the malformed JSON decode.
	s, _ := testStore(t)
	fillAttachmentQuota(t, s, 0)
	if _, err := s.db.Exec(`INSERT INTO inbox_attachments VALUES(1,?)`, []byte(`invalid`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PendingInbox(context.Background(), Scope{"quota", "u"}, 1); !errors.Is(err, ErrCapacity) {
		t.Fatalf("aggregate admission before decode: %v", err)
	}
}
func TestAttachmentScopedCompleteAndClear(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	a, b := Scope{"a", "u"}, Scope{"b", "u"}
	resolveAttachment(t, s, attachmentEntry(a, "same"))
	resolveAttachment(t, s, attachmentEntry(b, "same"))
	rows, err := s.PendingInbox(ctx, a, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteInbox(ctx, b, rows[0].Sequence); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign complete %v", err)
	}
	if err = s.CompleteInbox(ctx, a, rows[0].Sequence); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.db.QueryRow(`SELECT count(*) FROM inbox_attachments`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cascade %d %v", n, err)
	}
	resolveAttachment(t, s, attachmentEntry(a, "clear"))
	if _, err = s.db.Exec(`INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u','claim','caption','failed',1,1)`); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearMemory(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM inbox_attachments`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("scoped clear receive preservation %d %v", n, err)
	}
	if err = s.ClearDerived(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, sc := range []Scope{a, b} {
		got, err := s.PendingInbox(ctx, sc, 1)
		if err != nil || len(got) != 1 || got[0].Attachment == nil {
			t.Fatalf("clear changed receive: %v", err)
		}
		if err = s.CompleteInbox(ctx, sc, got[0].Sequence); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM inbox_attachments`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("completion cascade after clear: %d %v", n, err)
	}
	var content string
	if err = s.db.QueryRow(`SELECT content FROM messages WHERE account='a' AND id='claim'`).Scan(&content); err != nil || content != "" {
		t.Fatalf("claim retained %q %v", content, err)
	}
}
func TestAttachmentValidation(t *testing.T) {
	valid := []Attachment{*syntheticAttachment(), {Kind: "unsupported"}, {Kind: "file", MediaKey: "AAAAAAAAAAAAAAAAAAAAAA==", Query: "q", FileName: "../display.txt", Length: "4194304", MD5: strings.Repeat("0", 32)}}
	for _, a := range valid {
		if err := a.Validate(); err != nil {
			t.Fatalf("valid %s: %v", a.Kind, err)
		}
	}
	base := *syntheticAttachment()
	cases := map[string]func(*Attachment){"fallback": func(a *Attachment) { a.MediaKey = "AAAAAAAAAAAAAAAAAAAAAA==" }, "key": func(a *Attachment) { a.ImageHex = "no" }, "plain": func(a *Attachment) { a.ImageHex = "" }, "image-file": func(a *Attachment) { a.Length = "1" }, "url-space": func(a *Attachment) { a.FullURL = " https://example.invalid" }, "control": func(a *Attachment) { a.Query = "q\n" }, "field-bound": func(a *Attachment) { a.Query = strings.Repeat("q", 4097) }, "utf8": func(a *Attachment) { a.Query = string([]byte{255}) }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a := base
			change(&a)
			if !errors.Is(a.Validate(), ErrInvalid) {
				t.Fatal("invalid accepted")
			}
		})
	}
	a := base
	a.FullURL = strings.Repeat("&", 3000)
	if !errors.Is(a.Validate(), ErrCapacity) {
		t.Fatal("encoded escape expansion ignored")
	}
}
func TestAttachmentResolveFaultAndCancellationRetainEvidence(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	id, err := s.RecordPoll(ctx, sc, "", []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER synthetic_attachment_fault BEFORE INSERT ON inbox_attachments BEGIN SELECT RAISE(ABORT,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.ResolvePoll(ctx, sc, id, "next", []InboxEntry{attachmentEntry(sc, "id")}); !errors.Is(err, ErrStorage) {
		t.Fatalf("fault %v", err)
	}
	if _, err = s.db.Exec(`DROP TRIGGER synthetic_attachment_fault`); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = s.ResolvePoll(cancelled, sc, id, "next", []InboxEntry{attachmentEntry(sc, "id")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	f, err := s.PendingPoll(ctx, sc)
	if err != nil || f == nil || string(f.Body) != "original" {
		t.Fatalf("raw lost %v", err)
	}
	rows, err := s.PendingInbox(ctx, sc, 1)
	if err != nil || len(rows) != 0 {
		t.Fatalf("detached work %v", err)
	}
	cur, err := s.Cursor(ctx, sc)
	if err != nil || cur != "" {
		t.Fatalf("cursor %q %v", cur, err)
	}
}

func TestAttachmentCanonicalByteBoundary(t *testing.T) {
	a := *syntheticAttachment()
	body, err := encodeAttachment(&a)
	if err != nil {
		t.Fatal(err)
	}
	a.FullURL = strings.Repeat("x", maxAttachmentBytes-len(body))
	body, err = encodeAttachment(&a)
	if err != nil || len(body) != maxAttachmentBytes {
		t.Fatalf("exact descriptor boundary %d %v", len(body), err)
	}
	a.FullURL += "x"
	if !errors.Is(a.Validate(), ErrCapacity) {
		t.Fatal("descriptor boundary+1 admitted")
	}
}
func TestAttachmentFileKeysAndLengths(t *testing.T) {
	for _, key := range []string{"AAAAAAAAAAAAAAAAAAAAAA==", "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="} {
		a := Attachment{Kind: "file", MediaKey: key, Query: "q"}
		if err := a.Validate(); err != nil {
			t.Fatalf("key rejected %v", err)
		}
	}
	for _, length := range []string{"0", "01", "+1", "-1", "4194305", "1.0", " 1"} {
		a := Attachment{Kind: "file", MediaKey: "AAAAAAAAAAAAAAAAAAAAAA==", Query: "q", Length: length}
		if !errors.Is(a.Validate(), ErrInvalid) {
			t.Fatalf("length accepted %q", length)
		}
	}
	for _, key := range []string{"AAAAAAAAAAAAAAAAAAAAAB==", "AAAAAAAAAAAAAAAAAAAAAA=\n", "AAAAAAAAAAAAAAAAAAAAAA", "$$$$$$$$$$$$$$$$$$$$$$$$", "Z3dnZ3dnZ3dnZ3dnZ3dnZ3dnZ3dnZ3dnZ3dnZ3dnZ3c="} {
		a := Attachment{Kind: "file", MediaKey: key, Query: "q"}
		if !errors.Is(a.Validate(), ErrInvalid) {
			t.Fatal("invalid key accepted")
		}
	}
}

func TestAttachmentBackupDescriptorRoundTrip(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	sc := Scope{"backup-a", "u"}
	e := attachmentEntry(sc, "original")
	e.Text = "genuine caption"
	resolveAttachment(t, s, e)
	dest := path + ".snapshot"
	if err := s.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Open(dest)
	if snapshot != nil {
		t.Cleanup(func() { snapshot.Close() })
	}
	if err != nil {
		t.Fatal(err)
	}
	rows, err := snapshot.PendingInbox(ctx, sc, 1)
	if err != nil || len(rows) != 1 || rows[0].Text != "genuine caption" || rows[0].Attachment == nil || *rows[0].Attachment != *e.Attachment {
		t.Fatalf("descriptor snapshot %#v %v", rows, err)
	}
}
func TestAttachmentInvalidResolveRetainsEvidence(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	id, err := s.RecordPoll(ctx, sc, "", []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	good := attachmentEntry(sc, "good")
	bad := attachmentEntry(sc, "bad")
	bad.Attachment = &Attachment{Kind: "image", ImageHex: "invalid", Query: "q"}
	if err = s.ResolvePoll(ctx, sc, id, "next", []InboxEntry{good, bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid batch admitted %v", err)
	}
	if _, err = s.PendingInbox(ctx, sc, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero limit")
	}
	if _, err = s.PendingInbox(ctx, sc, 33); !errors.Is(err, ErrInvalid) {
		t.Fatal("over limit")
	}
	rows, err := s.PendingInbox(ctx, sc, 1)
	if err != nil || len(rows) != 0 {
		t.Fatal("partial batch committed")
	}
	f, err := s.PendingPoll(ctx, sc)
	if err != nil || f == nil || f.ID != id {
		t.Fatal("raw frame lost")
	}
	cur, err := s.Cursor(ctx, sc)
	if err != nil || cur != "" {
		t.Fatal("cursor advanced")
	}
}

func TestAttachmentSQLShapeAndQueueCountAdmission(t *testing.T) {
	t.Run("text-body", func(t *testing.T) {
		s, _ := testStore(t)
		if _, err := s.db.Exec(`INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','id','','',1);INSERT INTO inbox_attachments VALUES(1,'{}')`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PendingInbox(context.Background(), Scope{"a", "u"}, 1); !errors.Is(err, ErrInvalid) {
			t.Fatalf("text descriptor admitted %v", err)
		}
	})
	t.Run("row-count", func(t *testing.T) {
		s, _ := testStore(t)
		if _, err := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1025) INSERT INTO inbox(account,user,message_id,text,context_token,received_at) SELECT 'a','u',CAST(x AS TEXT),'','',1 FROM n`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PendingInbox(context.Background(), Scope{"a", "u"}, 1); !errors.Is(err, ErrCapacity) {
			t.Fatalf("queue count admitted %v", err)
		}
	})
}
