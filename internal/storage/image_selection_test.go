package storage

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func selectionSnapshot(t *testing.T, s *Store, sc Scope) ([]LibraryAsset, int64) {
	t.Helper()
	a, at, err := s.ImageSelectionSnapshot(context.Background(), sc)
	if err != nil {
		t.Fatal(err)
	}
	if s.db.Stats().InUse != 0 {
		t.Fatal("snapshot retained a connection")
	}
	return a, at
}

func TestImageSelectionSnapshotCountsAndCopies(t *testing.T) {
	for _, n := range []int{0, 1, 32, 128} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s, _ := testStore(t)
			sc := Scope{"a", "u"}
			ctx := context.Background()
			for i := 1; i <= n; i++ {
				a := libraryFixture(sc, i)
				if i == 1 {
					a.Format, a.Frames = "gif", 2
				}
				got, _, err := s.AddLibraryAsset(ctx, sc, a)
				if err != nil {
					t.Fatal(err)
				}
				if i == 2 {
					if err = s.MarkLibraryDeleting(ctx, sc, got.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			a, at := selectionSnapshot(t, s, sc)
			if len(a) != n || at != 0 {
				t.Fatalf("count=%d timestamp=%d", len(a), at)
			}
			for i, asset := range a {
				if asset.Scope != sc || asset.ID <= 0 || i > 0 && a[i-1].ID >= asset.ID {
					t.Fatal("scope/order", a)
				}
			}
			if n > 0 {
				if a[0].Format != "gif" || a[0].Enabled {
					t.Fatal("disabled GIF omitted or changed")
				}
				a[0].Tags[0] = "caller mutation"
				b, _ := selectionSnapshot(t, s, sc)
				if b[0].Tags[0] != "开心" || n > 1 && a[1].Tags[0] != "开心" {
					t.Fatal("tag alias")
				}
				if n > 1 && b[1].State != "deleting" {
					t.Fatal("tombstone omitted")
				}
			}
			if _, err := s.LibraryAssets(ctx, sc, 0, 128); !errors.Is(err, ErrInvalid) {
				t.Fatal("public page contract changed", err)
			}
		})
	}
}

func TestImageSelectionSnapshotConfirmedACKAndScope(t *testing.T) {
	s, sc, asset, id := imageFixture(t)
	ctx := context.Background()
	before, at := selectionSnapshot(t, s, sc)
	if len(before) != 1 || at != 0 {
		t.Fatal(before, at)
	}
	imageTextAck(t, s, sc, id)
	if err := s.BeginImageReplyImage(ctx, sc, id); err != nil {
		t.Fatal(err)
	}
	_, at = selectionSnapshot(t, s, sc)
	if at != 0 {
		t.Fatal("sending counted as ACK")
	}
	stamp := time.Now().UnixMilli() + 5
	if err := s.ConfirmImageReplyImage(ctx, sc, id, stamp); err != nil {
		t.Fatal(err)
	}
	a, at := selectionSnapshot(t, s, sc)
	if at != stamp || len(a) != 1 || a[0].Uses != 1 || a[0].LastUsedMillis != stamp {
		t.Fatal("confirmed transaction not visible", a, at)
	}
	for _, other := range []Scope{{"b", "u"}, {"a", "v"}} {
		otherAsset, _, err := s.AddLibraryAsset(ctx, other, libraryFixture(other, 2))
		if err != nil {
			t.Fatal(err)
		}
		if err = s.UpdateLibraryAsset(ctx, other, otherAsset.ID, LibraryMetadata{Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.ClaimMessage(ctx, other, "other", "synthetic"); err != nil {
			t.Fatal(err)
		}
		x, y := imageTestIDs(other, "other")
		if err = s.PrepareImageReply(ctx, other, "other", otherAsset.ID, otherAsset.SHA256, x, y); err != nil {
			t.Fatal(err)
		}
		b, cooldown := selectionSnapshot(t, s, other)
		if len(b) != 1 || b[0].Scope != other || cooldown != 0 {
			t.Fatal("cross-scope leak", b, cooldown)
		}
		imageTextAck(t, s, other, "other")
		if err = s.BeginImageReplyImage(ctx, other, "other"); err != nil {
			t.Fatal(err)
		}
		if err = s.ConfirmImageReplyImage(ctx, other, "other", stamp+1000); err != nil {
			t.Fatal(err)
		}
		_, ownCooldown := selectionSnapshot(t, s, sc)
		if ownCooldown != stamp {
			t.Fatal("other-scope ACK leaked", ownCooldown)
		}
	}
	if err := s.MarkLibraryDeleting(ctx, sc, asset.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishLibraryDelete(ctx, sc, asset.ID); err != nil {
		t.Fatal(err)
	}
	a, at = selectionSnapshot(t, s, sc)
	if len(a) != 0 || at != stamp {
		t.Fatal("historical ACK lost after asset deletion", a, at)
	}
	if err := s.ClearMemory(ctx, sc); err != nil {
		t.Fatal(err)
	}
	_, at = selectionSnapshot(t, s, sc)
	if at != 0 {
		t.Fatal("cleared receipt retained")
	}
}

func TestImageSelectionSnapshotStoppedAndUnknown(t *testing.T) {
	for _, sending := range []bool{false, true} {
		t.Run(fmt.Sprint(sending), func(t *testing.T) {
			s, sc, _, id := imageFixture(t)
			ctx := context.Background()
			imageTextAck(t, s, sc, id)
			if sending {
				if err := s.BeginImageReplyImage(ctx, sc, id); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.StopImageReply(ctx, sc, id, ImageStopImage, ImageStopUncertain); err != nil {
				t.Fatal(err)
			}
			_, at := selectionSnapshot(t, s, sc)
			if at != 0 {
				t.Fatal("stopped/unknown counted", at)
			}
		})
	}
}

func TestImageSelectionSnapshotRejectsBeforeDB(t *testing.T) {
	s := &Store{}
	sc := Scope{"a", "u"}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx  context.Context
		sc   Scope
		want error
	}{{nil, sc, ErrInvalid}, {context.Background(), Scope{}, ErrInvalid}, {canceled, sc, context.Canceled}} {
		a, at, err := s.ImageSelectionSnapshot(tc.ctx, tc.sc)
		if a != nil || at != 0 || !errors.Is(err, tc.want) {
			t.Fatalf("unsafe refusal: %v %d %v", a, at, err)
		}
	}
}

func TestImageSelectionSnapshotCorruption(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE library_assets SET tags='["duplicate","duplicate"]'`,
		`UPDATE library_assets SET tags='null'`,
		`UPDATE library_assets SET description=CAST(x'ff' AS TEXT)`,
		`UPDATE library_assets SET description=printf('%01025d',0)`,
		`UPDATE library_assets SET id=0`,
		`UPDATE image_replies SET image_phase='acked',image_acked_at=0`,
		`UPDATE image_replies SET text_acked_at=-1`,
		`UPDATE image_replies SET updated_at=253402300800000`,
		`UPDATE image_replies SET text_client_id='incorrect'`,
		`UPDATE messages SET state='sent',reply='wrong parent phase'`,
		`UPDATE image_replies SET stop_phase='image',stop_code='uncertain'`,
		`UPDATE image_replies SET sha256=printf('%065d',0)`,
	} {
		t.Run(mutation, func(t *testing.T) {
			s, sc, _, _ := imageFixture(t)
			ctx := context.Background()
			if _, err := s.db.ExecContext(ctx, mutation); err != nil {
				t.Fatal(err)
			}
			a, at, err := s.ImageSelectionSnapshot(ctx, sc)
			if err == nil || a != nil || at != 0 || s.db.Stats().InUse != 0 {
				t.Fatalf("corruption escaped/refusal retained resources: %v %d %v", a, at, err)
			}
		})
	}
}

func TestImageSelectionSnapshotOverflow(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	// Bounded raw synthetic fixture deliberately bypasses the normal 128 cap.
	for i := 1; i <= 129; i++ {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) VALUES('a','u',?,'png',100,2,3,1,'[]','','','ready',0,0,0)`, fmt.Sprintf("%064x", i)); err != nil {
			t.Fatal(err)
		}
	}
	a, at, err := s.ImageSelectionSnapshot(ctx, sc)
	if !errors.Is(err, ErrCapacity) || a != nil || at != 0 || s.db.Stats().InUse != 0 {
		t.Fatal("overflow silently truncated", len(a), at, err)
	}
}

func TestImageSelectionSnapshotConsistentConcurrentMutation(t *testing.T) {
	s, sc, _, id := imageFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	imageTextAck(t, s, sc, id)
	if err := s.BeginImageReplyImage(ctx, sc, id); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UnixMilli() + 5
	if err := s.ConfirmImageReplyImage(ctx, sc, id, base); err != nil {
		t.Fatal(err)
	}
	ready, advance, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	t.Cleanup(func() { cancel(); <-done }) // Join registered before any concurrent assertions.
	go func() {
		defer close(ready)
		select {
		case <-ctx.Done():
			done <- nil
			return
		case <-advance:
		}
		for i := int64(1); i <= 24; i++ {
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				done <- err
				return
			}
			_, err = tx.ExecContext(ctx, `UPDATE library_assets SET last_used=? WHERE account=? AND user=?`, base+i, sc.Account, sc.User)
			if err == nil {
				_, err = tx.ExecContext(ctx, `UPDATE image_replies SET image_acked_at=?,updated_at=? WHERE account=? AND user=?`, base+i, base+i, sc.Account, sc.User)
			}
			if err == nil {
				err = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	close(advance)
	for i := 0; i < 24; i++ {
		a, at, err := s.ImageSelectionSnapshot(ctx, sc)
		if err != nil || len(a) != 1 || a[0].LastUsedMillis != at {
			t.Fatalf("mixed transaction view: %v %d %v", a, at, err)
		}
	}
	<-ready
	// Inspect the actual worker result without consuming cleanup's join token.
	err := <-done
	done <- err
	if err != nil {
		t.Fatal(err)
	}
	a, at := selectionSnapshot(t, s, sc)
	if at != base+24 || a[0].LastUsedMillis != at {
		t.Fatal("writer did not finish", a, at)
	}
}

func TestImageSelectionSnapshotNoMutation(t *testing.T) {
	s, sc, _, id := imageFixture(t)
	ctx := context.Background()
	before, err := s.ImageReplyReceipt(ctx, sc, id)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := selectionSnapshot(t, s, sc)
	b, _ := selectionSnapshot(t, s, sc)
	after, err := s.ImageReplyReceipt(ctx, sc, id)
	if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(a, b) || strings.Contains(fmt.Sprint(a), "synthetic caption") {
		t.Fatal("snapshot mutated state or returned receipt payload", err)
	}
}

func TestImageSelectionSnapshotMaximumAndByteCapacity(t *testing.T) {
	s, sc, asset, id := imageFixture(t)
	ctx := context.Background()
	imageTextAck(t, s, sc, id)
	if err := s.BeginImageReplyImage(ctx, sc, id); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UnixMilli() + 5
	if err := s.ConfirmImageReplyImage(ctx, sc, id, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimMessage(ctx, sc, "second", "synthetic"); err != nil {
		t.Fatal(err)
	}
	x, y := imageTestIDs(sc, "second")
	if err := s.PrepareImageReply(ctx, sc, "second", asset.ID, asset.SHA256, x, y); err != nil {
		t.Fatal(err)
	}
	imageTextAck(t, s, sc, "second")
	if err := s.BeginImageReplyImage(ctx, sc, "second"); err != nil {
		t.Fatal(err)
	}
	stamp += 1000
	if err := s.ConfirmImageReplyImage(ctx, sc, "second", stamp); err != nil {
		t.Fatal(err)
	}
	_, at := selectionSnapshot(t, s, sc)
	if at != stamp {
		t.Fatal("did not take maximum", at)
	}
	for i := 2; i <= 17; i++ {
		if _, _, err := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE library_assets SET bytes=4194304`); err != nil {
		t.Fatal(err)
	}
	a, at, err := s.ImageSelectionSnapshot(ctx, sc)
	if !errors.Is(err, ErrCapacity) || a != nil || at != 0 {
		t.Fatal("global bytes bypassed", len(a), at, err)
	}
}

func TestImageSelectionSnapshotWaitCancellation(t *testing.T) {
	s, _ := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan error, 1)
	t.Cleanup(func() { cancel(); <-done })
	go func() {
		a, at, err := s.ImageSelectionSnapshot(ctx, Scope{"a", "u"})
		if a != nil || at != 0 || !errors.Is(err, context.Canceled) {
			done <- fmt.Errorf("unsafe cancellation: %v %d %v", a, at, err)
			return
		}
		done <- nil
	}()
	deadline := time.Now().Add(5 * time.Second)
	for s.db.Stats().WaitCount == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	waited := s.db.Stats().WaitCount > 0
	cancel()
	err = <-done
	done <- err
	if !waited || err != nil {
		t.Fatal("did not cancel queued transaction", waited, err)
	}
}
