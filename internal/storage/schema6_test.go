package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSchema6FreshReceiptAndReservation(t *testing.T) {
	s, _ := testStore(t)
	var v int
	if e := s.db.QueryRow("PRAGMA user_version").Scan(&v); e != nil || v != 6 {
		t.Fatalf("need schema6: %d %v", v, e)
	}
	var n int
	if e := s.db.QueryRow("SELECT count(*) FROM image_replies").Scan(&n); e != nil || n != 0 {
		t.Fatalf("bounded receipts missing: %d %v", n, e)
	}
	if e := validateManifest(context.Background(), s.db); e != nil {
		t.Fatal(e)
	}
}
func TestSchema6ExactLegacyMigration(t *testing.T) {
	for v := 1; v <= 5; v++ {
		t.Run(fmt.Sprint(v), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "old.db")
			db, e := sql.Open("sqlite", path)
			if e != nil {
				t.Fatal(e)
			}
			ddl := []string{schema, schema2, schema3, schema4, schema5}
			for _, query := range ddl[:v] {
				if _, e = db.Exec(query); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = db.Exec("INSERT INTO messages VALUES('a','u','legacy','question','answer','sent',1,1)"); e != nil {
				t.Fatal(e)
			}
			if e = validateManifest(context.Background(), db); e != nil {
				t.Fatal("legacy manifest", e)
			}
			if e = db.Close(); e != nil {
				t.Fatal(e)
			}
			if e = ValidateBackup(context.Background(), path); e != nil {
				t.Fatal("legacy backup", e)
			}
			s, e := Open(path)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			var version int
			s.db.QueryRow("PRAGMA user_version").Scan(&version)
			if version != 6 {
				t.Fatal(version)
			}
			if e = validateManifest(context.Background(), s.db); e != nil {
				t.Fatal(e)
			}
			h, e := s.History(context.Background(), Scope{"a", "u"}, 1)
			if e != nil || len(h) != 2 || h[1].Content != "answer" {
				t.Fatal("legacy text", e)
			}
		})
	}
}
func TestSchema6AdmissionRejectsCorruption(t *testing.T) {
	bad := []string{
		"PRAGMA user_version=7", "CREATE TRIGGER image_foreign AFTER UPDATE ON image_replies BEGIN SELECT 1; END",
		"UPDATE image_replies SET sha256=upper(sha256)", "UPDATE image_replies SET text_client_id='alias'", "UPDATE image_replies SET account='other'", "UPDATE image_replies SET asset_id=1.5", "UPDATE image_replies SET text_acked_at='x'", "UPDATE image_replies SET created_at=0", "UPDATE image_replies SET updated_at=created_at-1", "UPDATE image_replies SET image_phase='acked',image_acked_at=created_at", "UPDATE image_replies SET stop_code='refused'", "UPDATE image_replies SET text_phase='sending'", "UPDATE image_replies SET id=CAST(zeroblob(513) AS TEXT)", "UPDATE image_replies SET sha256=CAST(X'FF' AS TEXT)", "UPDATE image_replies SET preparation='bogus'",
	}
	for i, query := range bad {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, _, _, _ := imageFixture(t)
			if i == 2 {
				query = "UPDATE image_replies SET sha256='ABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCD'"
			}
			if _, e := s.db.Exec("PRAGMA ignore_check_constraints=ON; PRAGMA foreign_keys=OFF;" + query); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(t.TempDir(), "bad.db")
			if e := s.Backup(context.Background(), path); e != nil {
				t.Fatal(e)
			}
			if e := ValidateBackup(context.Background(), path); e == nil {
				t.Fatal("admitted backup", query)
			}
			if opened, e := Open(path); e == nil {
				opened.Close()
				t.Fatal("admitted Open", query)
			}
			for _, suffix := range []string{"-wal", "-shm"} {
				if _, e := os.Lstat(path + suffix); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("immutable refusal side effect", suffix, e)
				}
			}
		})
	}
}
func TestSchema6FixedBudgetBoundary(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	a, _, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 1))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ClaimMessage(ctx, sc, "selected", ""); e != nil {
		t.Fatal(e)
	}
	// One finite SQL bulk fixture fills the existing budget to precisely one
	// receipt reservation below the cap without rerunning 10,000-row quotas.
	left := int64(64<<20) - 16384 - imageReplyReservation
	full := left / 32768
	rem := left % 32768
	if _, e = s.db.Exec(`WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?) INSERT INTO messages SELECT 'a','u','bulk-'||x,?,?,'sent',1,1 FROM n`, full, strings.Repeat("c", 16384), strings.Repeat("r", 16384)); e != nil {
		t.Fatal(e)
	}
	if rem > 0 {
		if _, e = s.db.Exec("INSERT INTO messages VALUES('a','u','tail',?,?,'sent',1,1)", strings.Repeat("c", int(min(rem, 16384))), strings.Repeat("r", int(max(rem-16384, 0)))); e != nil {
			t.Fatal(e)
		}
	}
	x, y := imageClientIDs(sc, "selected")
	if e = s.PrepareImageReply(ctx, sc, "selected", a.ID, a.SHA256, x, y); e != nil {
		t.Fatal("exact reservation", e)
	}
	_, bytes, e := messageBudget(ctx, s.db)
	if e != nil || bytes != 64<<20 {
		t.Fatal("budget", bytes, e)
	}
	if _, e = s.ClaimMessage(ctx, sc, "another", ""); !errors.Is(e, ErrCapacity) {
		t.Fatal("uncharged receipt", e)
	}
	if e = s.EnterImagePreparation(ctx, sc, "selected"); e != nil {
		t.Fatal(e)
	}
	if e = s.StopImageReply(ctx, sc, "selected", "preparation", "unavailable"); e != nil {
		t.Fatal("late phase charge", e)
	}
	if _, _, e = messageBudget(ctx, s.db); e != nil {
		t.Fatal(e)
	}
}
func TestSchema6ReceiptInsertionOverBudgetRollsBack(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	a, _, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 1))
	if e != nil {
		t.Fatal(e)
	}
	s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{Enabled: true})
	s.ClaimMessage(ctx, sc, "id", "")
	left := int64(64<<20) - 16384 - imageReplyReservation + 1
	full := left / 32768
	rem := left % 32768
	if _, e = s.db.Exec(`WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?) INSERT INTO messages SELECT 'a','u','bulk-'||x,?,?,'sent',1,1 FROM n`, full, strings.Repeat("c", 16384), strings.Repeat("r", 16384)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec("INSERT INTO messages VALUES('a','u','tail',?,?,'sent',1,1)", strings.Repeat("c", int(min(rem, 16384))), strings.Repeat("r", int(max(rem-16384, 0)))); e != nil {
		t.Fatal(e)
	}
	x, y := imageClientIDs(sc, "id")
	if e = s.PrepareImageReply(ctx, sc, "id", a.ID, a.SHA256, x, y); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	var count int
	s.db.QueryRow("SELECT count(*) FROM image_replies").Scan(&count)
	if count != 0 {
		t.Fatal("partial insert")
	}
}
func TestSchema6EffectiveWALRejectsParentContradiction(t *testing.T) {
	s, sc, _, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if _, e := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); e != nil {
		t.Fatal(e)
	}
	var path string
	s.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path)
	if _, e := s.db.Exec("UPDATE image_replies SET text_phase='sending',text_acked_at=0"); e != nil {
		t.Fatal(e)
	}
	if opened, e := Open(path); e == nil {
		opened.Close()
		t.Fatal("ignored invalid WAL receipt")
	}
	if e := ctx.Err(); e != nil {
		t.Fatal(e)
	}
}
func TestSchema6HistoricalDeletedAssetReopen(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if e := s.MarkLibraryDeleting(ctx, sc, a.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.FinishLibraryDelete(ctx, sc, a.ID); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "history.db")
	if e := s.Backup(ctx, path); e != nil {
		t.Fatal(e)
	}
	r, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = r.BeginImageReplyImage(ctx, sc, id); e == nil {
		t.Fatal("missing asset authorized")
	}
	receipt, e := r.ImageReplyReceipt(ctx, sc, id)
	if e != nil || receipt.TextPhase != "acked" {
		t.Fatal(e)
	}
	if e = r.StopImageReply(ctx, sc, id, "image", "unavailable"); e != nil {
		t.Fatal(e)
	}
	if e = r.ConfirmImageReplyImage(ctx, sc, id, time.Now().UnixMilli()); e == nil {
		t.Fatal("invented ACK")
	}
}
func TestSchema6CompanionBudgetAndAdmission(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	e := attachmentEntry(sc, "attachment")
	e.Text = "caption"
	resolveAttachment(t, s, e)
	body, err := encodeAttachment(e.Attachment)
	if err != nil {
		t.Fatal(err)
	}
	count, size, err := receiveQueueBudget(ctx, s.db)
	if err != nil || count != 1 || size != len(e.Text)+len(body) {
		t.Fatal("schema6 companion not charged", count, size, err)
	}
	if _, err = s.db.Exec("UPDATE inbox_attachments SET body=X'7B7D'"); err != nil {
		t.Fatal(err)
	}
	if err = validateAttachments(ctx, s.db); err == nil {
		t.Fatal("schema6 malformed companion admitted")
	}
	if _, err = s.db.Exec("UPDATE inbox_attachments SET body=zeroblob(8388608)"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = receiveQueueBudget(ctx, s.db); !errors.Is(err, ErrCapacity) {
		t.Fatal("schema6 companion quota skipped", err)
	}
}
