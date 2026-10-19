package storage

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type imageReplyAPI interface {
	PrepareImageReply(context.Context, Scope, string, int64, string, string, string) error
	EnterImagePreparation(context.Context, Scope, string) error
	FinishImagePreparation(context.Context, Scope, string) error
	BeginImageReplyText(context.Context, Scope, string) error
	ConfirmImageReplyText(context.Context, Scope, string, string) error
	BeginImageReplyImage(context.Context, Scope, string) error
	ConfirmImageReplyImage(context.Context, Scope, string, int64) error
	StopImageReply(context.Context, Scope, string, string, string) error
}

func imageTestIDs(sc Scope, id string) (string, string) { return imageClientIDs(sc, id) }
func imageFixture(t *testing.T) (*Store, Scope, LibraryAsset, string) {
	t.Helper()
	s, _ := testStore(t)
	sc := Scope{"a", "u"}
	ctx := context.Background()
	a, _, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 1))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ClaimMessage(ctx, sc, "id", "synthetic caption"); e != nil {
		t.Fatal(e)
	}
	x, y := imageTestIDs(sc, "id")
	if e = s.PrepareImageReply(ctx, sc, "id", a.ID, a.SHA256, x, y); e != nil {
		t.Fatal(e)
	}
	return s, sc, a, "id"
}
func imageTextAck(t *testing.T, s *Store, sc Scope, id string) {
	t.Helper()
	ctx := context.Background()
	for _, fn := range []func(context.Context, Scope, string) error{s.EnterImagePreparation, s.FinishImagePreparation, s.BeginImageReplyText} {
		if e := fn(ctx, sc, id); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.ConfirmImageReplyText(ctx, sc, id, "delivered text"); e != nil {
		t.Fatal(e)
	}
}
func TestImageReplyTransactionAndIdempotentACK(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	if e := s.SetMessageState(ctx, sc, id, "sending"); !errors.Is(e, ErrNotFound) {
		t.Fatalf("generic mutation accepted: %v", e)
	}
	if e := s.BeginImageReplyImage(ctx, sc, id); e == nil {
		t.Fatal("premature image")
	}
	imageTextAck(t, s, sc, id)
	if e := s.ConfirmImageReplyText(ctx, sc, id, "delivered text"); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmImageReplyText(ctx, sc, id, "different"); e == nil {
		t.Fatal("rewrote ACK")
	}
	if e := s.CompleteMessage(ctx, sc, id, "different"); e == nil {
		t.Fatal("legacy completion selected")
	}
	if e := s.BeginImageReplyImage(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	at := time.Now().UnixMilli() + 5
	if e := s.ConfirmImageReplyImage(ctx, sc, id, at); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmImageReplyImage(ctx, sc, id, at); e != nil {
		t.Fatal(e)
	}
	got, e := s.LibraryAsset(ctx, sc, a.ID)
	if e != nil || got.Uses != 1 || got.LastUsedMillis != at {
		t.Fatalf("usage %v %v", got, e)
	}
	if e = s.MarkLibraryDeleting(ctx, sc, a.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishLibraryDelete(ctx, sc, a.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.ConfirmImageReplyImage(ctx, sc, id, at); e != nil {
		t.Fatal("historical ACK", e)
	}
	if e = validateSchema6IfPresent(ctx, s.db); e != nil {
		t.Fatal(e)
	}
	if e = s.StopImageReply(ctx, sc, id, "image", "uncertain"); e == nil {
		t.Fatal("ACK regressed")
	}
	h, e := s.History(ctx, sc, 1)
	if e != nil || len(h) != 2 || h[1].Content != "delivered text" {
		t.Fatal("history", h, e)
	}
}
func TestImageReplyUsageRollback(t *testing.T) {
	for _, bad := range []string{"overflow", "digest", "disabled", "missing", "scope"} {
		t.Run(bad, func(t *testing.T) {
			s, sc, a, id := imageFixture(t)
			ctx := context.Background()
			imageTextAck(t, s, sc, id)
			if e := s.BeginImageReplyImage(ctx, sc, id); e != nil {
				t.Fatal(e)
			}
			switch bad {
			case "overflow":
				s.db.Exec("UPDATE library_assets SET uses=?", int64(math.MaxInt64))
			case "digest":
				s.db.Exec("UPDATE library_assets SET sha256=?", libraryFixture(sc, 2).SHA256)
			case "disabled":
				s.db.Exec("UPDATE library_assets SET enabled=0")
			case "missing":
				s.db.Exec("DELETE FROM library_assets")
			case "scope":
				s.db.Exec("UPDATE library_assets SET user='other'")
			}
			if e := s.ConfirmImageReplyImage(ctx, sc, id, time.Now().UnixMilli()+10); e == nil {
				t.Fatal("invalid asset ACK")
			}
			r, e := s.ImageReplyReceipt(ctx, sc, id)
			if e != nil || r.ImagePhase != "sending" || r.ImageAcknowledgedAtMillis != 0 {
				t.Fatal("partial receipt", r, e)
			}
			var uses int64
			if bad != "missing" {
				if e = s.db.QueryRow("SELECT uses FROM library_assets WHERE id=?", a.ID).Scan(&uses); e != nil {
					t.Fatal(e)
				}
				want := int64(0)
				if bad == "overflow" {
					want = math.MaxInt64
				}
				if uses != want {
					t.Fatal("partial usage", uses)
				}
			}
			h, e := s.History(ctx, sc, 1)
			if e != nil || len(h) != 2 {
				t.Fatal("text lost", e)
			}
		})
	}
}
func TestImageReplyMonotoneStops(t *testing.T) {
	for _, stage := range []string{"planned", "external", "ready", "textsending", "textacked", "imagesending"} {
		t.Run(stage, func(t *testing.T) {
			s, sc, _, id := imageFixture(t)
			ctx := context.Background()
			phase := "preparation"
			if stage != "planned" {
				if e := s.EnterImagePreparation(ctx, sc, id); e != nil {
					t.Fatal(e)
				}
			}
			if stage != "planned" && stage != "external" {
				if e := s.FinishImagePreparation(ctx, sc, id); e != nil {
					t.Fatal(e)
				}
				phase = "text"
			}
			if stage == "textsending" || stage == "textacked" || stage == "imagesending" {
				if e := s.BeginImageReplyText(ctx, sc, id); e != nil {
					t.Fatal(e)
				}
			}
			if stage == "textacked" || stage == "imagesending" {
				if e := s.ConfirmImageReplyText(ctx, sc, id, "delivered text"); e != nil {
					t.Fatal(e)
				}
				phase = "image"
			}
			if stage == "imagesending" {
				if e := s.BeginImageReplyImage(ctx, sc, id); e != nil {
					t.Fatal(e)
				}
			}
			if e := s.StopImageReply(ctx, sc, id, phase, "canceled"); e != nil {
				t.Fatal(e)
			}
			r, e := s.ImageReplyReceipt(ctx, sc, id)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.StopImageReply(ctx, sc, id, phase, "canceled"); e != nil {
				t.Fatal(e)
			}
			if e = s.StopImageReply(ctx, sc, id, phase, "refused"); e == nil {
				t.Fatal("terminal overwritten")
			}
			again, _ := s.ImageReplyReceipt(ctx, sc, id)
			if again != r {
				t.Fatal("terminal changed")
			}
			if e = validateSchema6IfPresent(ctx, s.db); e != nil {
				t.Fatal(e)
			}
			if stage == "textacked" || stage == "imagesending" {
				h, e := s.History(ctx, sc, 1)
				if e != nil || len(h) != 2 {
					t.Fatal("ACK erased", e)
				}
			}
		})
	}
}
func TestImageReplyClearAndBackupNoReplay(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if e := s.ClearDerived(ctx, sc); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ImageReplyReceipt(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot.db")
	if e := s.Backup(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	if e := ValidateBackup(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	restored, e := Open(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if claimed, e := restored.ClaimMessage(ctx, sc, id, "new"); e != nil || claimed {
		t.Fatal("replay", claimed, e)
	}
	r, e := restored.ImageReplyReceipt(ctx, sc, id)
	if e != nil || r.ImagePhase != "planned" || r.TextPhase != "acked" {
		t.Fatal("receipt changed", r, e)
	}
	asset, e := restored.LibraryAsset(ctx, sc, a.ID)
	if e != nil || asset.Uses != 0 {
		t.Fatal("inferred image usage", e)
	}
	if e = s.ClearMemory(ctx, sc); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImageReplyReceipt(ctx, sc, id); !errors.Is(e, ErrNotFound) {
		t.Fatal("receipt retained", e)
	}
	if claimed, e := s.ClaimMessage(ctx, sc, id, "new"); e != nil || claimed {
		t.Fatal("claim erased", e)
	}
	h, e := s.History(ctx, sc, 1)
	if e != nil || len(h) != 0 {
		t.Fatal("history not cleared", e)
	}
}
func TestImageReplyCanceledTransactionKeepsText(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	imageTextAck(t, s, sc, id)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := s.BeginImageReplyImage(ctx, sc, id); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	asset, e := s.LibraryAsset(context.Background(), sc, a.ID)
	if e != nil || asset.Uses != 0 {
		t.Fatal(e)
	}
	h, e := s.History(context.Background(), sc, 1)
	if e != nil || len(h) != 2 {
		t.Fatal("text lost", e)
	}
}
func TestImageReplyPreparationRejectsAliasesAndAssets(t *testing.T) {
	for _, bad := range []string{"digest", "textAlias", "imageAlias", "scope", "disabled", "gif", "deleting", "parent", "unclaimed"} {
		t.Run(bad, func(t *testing.T) {
			s, _ := testStore(t)
			ctx := context.Background()
			sc := Scope{"a", "u"}
			a, _, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 10))
			if e != nil {
				t.Fatal(e)
			}
			if e = s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{Enabled: true}); e != nil {
				t.Fatal(e)
			}
			if _, e = s.ClaimMessage(ctx, sc, "id", "caption"); e != nil {
				t.Fatal(e)
			}
			digest := a.SHA256
			x, y := imageClientIDs(sc, "id")
			id := "id"
			switch bad {
			case "digest":
				digest = libraryFixture(sc, 11).SHA256
			case "textAlias":
				x = "miskoai-alias"
			case "imageAlias":
				y = x
			case "scope":
				sc.User = "other"
			case "disabled":
				s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{})
			case "gif":
				s.db.Exec("UPDATE library_assets SET format='gif'")
			case "deleting":
				s.MarkLibraryDeleting(ctx, sc, a.ID)
			case "parent":
				s.SetMessageState(ctx, sc, id, "failed")
			case "unclaimed":
				id = "missing"
			}
			if e = s.PrepareImageReply(ctx, sc, id, a.ID, digest, x, y); e == nil {
				t.Fatal("invalid prerequisite accepted")
			}
			var n int
			if e = s.db.QueryRow("SELECT count(*) FROM image_replies").Scan(&n); e != nil || n != 0 {
				t.Fatal("partial plan", n, e)
			}
		})
	}
}
func TestImageReplyExactIDsAndScopedDuplicate(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	x, y := imageClientIDs(sc, id)
	if len(x) != 72 || len(y) != 72 || x == y {
		t.Fatal("IDs", x, y)
	}
	if e := s.PrepareImageReply(ctx, sc, id, a.ID, a.SHA256, x, y); !errors.Is(e, ErrStale) {
		t.Fatal("duplicate plan", e)
	}
	if _, e := s.ImageReplyReceipt(ctx, Scope{"a", "other"}, id); !errors.Is(e, ErrNotFound) {
		t.Fatal("scope leak", e)
	}
	if e := s.EnterImagePreparation(ctx, Scope{"other", "u"}, id); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if e := s.FinishImagePreparation(ctx, sc, id); e == nil {
		t.Fatal("skipped external")
	}
	if e := s.BeginImageReplyText(ctx, sc, id); e == nil {
		t.Fatal("skipped ready")
	}
	if e := s.StopImageReply(ctx, sc, id, "image", "canceled"); e == nil {
		t.Fatal("invented text ACK")
	}
}
func TestImageReplyReusedAssetCannotAuthorizeUsage(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if e := s.BeginImageReplyImage(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("UPDATE library_assets SET sha256=? WHERE id=?", libraryFixture(sc, 2).SHA256, a.ID); e != nil {
		t.Fatal(e)
	}
	if e := validateSchema6IfPresent(ctx, s.db); e != nil {
		t.Fatal("historical digest not admitted", e)
	}
	if e := s.ConfirmImageReplyImage(ctx, sc, id, time.Now().UnixMilli()+1); e == nil {
		t.Fatal("replacement asset use")
	}
	var n int
	s.db.QueryRow("SELECT uses FROM library_assets WHERE id=?", a.ID).Scan(&n)
	if n != 0 {
		t.Fatal("replacement usage", n)
	}
}
func TestImageReplyTextACKTransactionRollback(t *testing.T) {
	s, sc, _, id := imageFixture(t)
	ctx := context.Background()
	if e := s.EnterImagePreparation(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	if e := s.FinishImagePreparation(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	if e := s.BeginImageReplyText(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("CREATE TRIGGER synthetic_ack_failure BEFORE UPDATE ON image_replies BEGIN SELECT RAISE(ABORT,'synthetic'); END"); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmImageReplyText(ctx, sc, id, "delivered text"); e == nil {
		t.Fatal("trigger must fail ACK")
	}
	var state, reply string
	if e := s.db.QueryRow("SELECT state,reply FROM messages WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id).Scan(&state, &reply); e != nil || state != "sending" || reply != "" {
		t.Fatal("partial parent ACK", state, reply, e)
	}
	if _, e := s.db.Exec("DROP TRIGGER synthetic_ack_failure"); e != nil {
		t.Fatal(e)
	}
	r, e := s.ImageReplyReceipt(ctx, sc, id)
	if e != nil || r.TextPhase != "sending" || r.TextAcknowledgedAtMillis != 0 {
		t.Fatal("partial sidecar ACK", r, e)
	}
}
func TestImageReplyMonotonicUsageTimestamp(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if e := s.BeginImageReplyImage(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	at := time.Now().UnixMilli() + 1
	later := at + 100
	if _, e := s.db.Exec("UPDATE library_assets SET last_used=?", later); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmImageReplyImage(ctx, sc, id, at); e != nil {
		t.Fatal(e)
	}
	asset, e := s.LibraryAsset(ctx, sc, a.ID)
	if e != nil || asset.LastUsedMillis != later || asset.Uses != 1 {
		t.Fatal(asset, e)
	}
}
func TestImageReplyClearMemoryPreservesReceiveCatalogCounters(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if _, e := s.db.Exec("INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES(?,?, 'incoming','receive caption','synthetic token',1)", sc.Account, sc.User); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("INSERT INTO channel_cursors VALUES(?,?,'synthetic cursor')", sc.Account, sc.User); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("INSERT INTO poll_frames(account,user,cursor,body,state) VALUES(?,?,'synthetic',X'7B7D','pending')", sc.Account, sc.User); e != nil {
		t.Fatal(e)
	}
	if e := s.ClearMemory(ctx, sc); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"inbox", "channel_cursors", "poll_frames", "library_assets", "messages"} {
		var n int
		if e := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != 1 {
			t.Fatal("erased durable metadata", table, n, e)
		}
	}
	var counter int64
	if e := s.db.QueryRow("SELECT seq FROM sqlite_sequence WHERE name='inbox'").Scan(&counter); e != nil || counter != 1 {
		t.Fatal("sequence lost", counter, e)
	}
	asset, e := s.LibraryAsset(ctx, sc, a.ID)
	if e != nil || asset.Uses != 0 {
		t.Fatal(asset, e)
	}
	if e = validateSchema6IfPresent(ctx, s.db); e != nil {
		t.Fatal(e)
	}
}
func TestImageReplyImageACKTransactionRollback(t *testing.T) {
	s, sc, a, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if e := s.BeginImageReplyImage(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("CREATE TRIGGER synthetic_image_ack_failure BEFORE UPDATE ON image_replies BEGIN SELECT RAISE(ABORT,'synthetic'); END"); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmImageReplyImage(ctx, sc, id, time.Now().UnixMilli()+2); e == nil {
		t.Fatal("trigger must fail ACK")
	}
	asset, e := s.LibraryAsset(ctx, sc, a.ID)
	if e != nil || asset.Uses != 0 || asset.LastUsedMillis != 0 {
		t.Fatal("partial usage", asset, e)
	}
	if _, e = s.db.Exec("DROP TRIGGER synthetic_image_ack_failure"); e != nil {
		t.Fatal(e)
	}
	r, e := s.ImageReplyReceipt(ctx, sc, id)
	if e != nil || r.ImagePhase != "sending" || r.ImageAcknowledgedAtMillis != 0 {
		t.Fatal("partial receipt", r, e)
	}
}

// Oversized identities must be refused before JSON marshaling/hash work or
// opening a transaction. Inputs are prepared outside allocation measurement.
func TestImageReplyInvalidIdentityRejectedBeforeAllocation(t *testing.T) {
	digest := strings.Repeat("a", 64)
	payload := strings.Repeat("<\"\\", 1366) // 4098 escaped synthetic bytes, finite.
	cases := []struct {
		name  string
		scope Scope
		id    string
	}{
		{"account", Scope{payload, "u"}, "id"},
		{"user", Scope{"a", payload}, "id"},
		{"message-id", Scope{"a", "u"}, payload},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Store{} // Invalid input must not touch/open a database.
			var got error
			allocations := testing.AllocsPerRun(5, func() {
				got = s.PrepareImageReply(context.Background(), tc.scope, tc.id, 1, digest, "synthetic-text", "synthetic-image")
			})
			if !errors.Is(got, ErrInvalid) {
				t.Fatalf("invalid identity error: %v", got)
			}
			if allocations != 0 {
				t.Fatalf("invalid identity allocated %.0f times before refusal; want zero", allocations)
			}
		})
	}
}
