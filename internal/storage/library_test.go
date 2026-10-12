package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func libraryFixture(sc Scope, n int) LibraryAsset {
	return LibraryAsset{Scope: sc, SHA256: fmt.Sprintf("%064x", n), Format: "png", Bytes: 100, Width: 2, Height: 3, Frames: 1, State: "ready", Tags: []string{"开心"}, Description: "synthetic"}
}

func TestLibraryMetadataBoundariesAndDisabledEdits(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	a := libraryFixture(sc, 1)
	a.Format, a.Width, a.Height, a.Frames = "gif", 8192, 1, 32
	a.Description, a.Emotion = strings.Repeat("x", 1024), strings.Repeat("e", 64)
	a.Tags = make([]string, 16)
	for i := range a.Tags {
		a.Tags[i] = fmt.Sprintf("%032d", i)
	}
	got, added, e := s.AddLibraryAsset(ctx, sc, a)
	if e != nil || !added || !reflect.DeepEqual(got.Tags, a.Tags) {
		t.Fatal("boundary insert", e)
	}
	a.Tags[0] = "changed caller slice"
	stored, e := s.LibraryAsset(ctx, sc, got.ID)
	if e != nil || stored.Tags[0] == a.Tags[0] {
		t.Fatal("caller aliases persisted tags", e)
	}
	for _, meta := range []LibraryMetadata{
		{Tags: []string{strings.Repeat("x", 33)}}, {Emotion: strings.Repeat("e", 65)},
		{Description: "x\x00"}, {Description: strings.Repeat("界", 342)},
	} {
		if e = s.UpdateLibraryAsset(ctx, sc, got.ID, meta); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid edit", e)
		}
	}
	escaped := make([]string, 16)
	for i := range escaped {
		escaped[i] = fmt.Sprintf("%02d", i) + strings.Repeat("<", 30)
	}
	if e = s.UpdateLibraryAsset(ctx, sc, got.ID, LibraryMetadata{Tags: escaped}); !errors.Is(e, ErrInvalid) {
		t.Fatal("encoded tag JSON budget", e)
	}
	if e = s.UpdateLibraryAsset(ctx, sc, got.ID, LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	if e = s.RecordLibraryUse(ctx, sc, got.ID, 253402300799999); e != nil {
		t.Fatal("max stamp", e)
	}
	if e = s.UpdateLibraryAsset(ctx, sc, got.ID, LibraryMetadata{}); e != nil {
		t.Fatal("disable", e)
	}
	stored, e = s.LibraryAsset(ctx, sc, got.ID)
	if e != nil || stored.Enabled || stored.Uses != 1 || stored.LastUsedMillis != 253402300799999 {
		t.Fatal("disable lost usage", e)
	}
	if e = s.RecordLibraryUse(ctx, sc, got.ID, 1); !errors.Is(e, ErrInvalid) {
		t.Fatal("disabled usage", e)
	}
	for _, id := range []int64{0, -1} {
		if _, e = s.LibraryAsset(ctx, sc, id); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid id", e)
		}
	}
}

func TestLibraryConcurrentDuplicateAndMutationRollback(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	type outcome struct {
		a     LibraryAsset
		added bool
		e     error
	}
	results := make(chan outcome, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, added, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 1))
			results <- outcome{a, added, e}
		}()
	}
	wg.Wait()
	close(results)
	var id int64
	inserted := 0
	for r := range results {
		if r.e != nil {
			t.Fatal(r.e)
		}
		if id == 0 {
			id = r.a.ID
		}
		if r.a.ID != id {
			t.Fatal("duplicate race IDs")
		}
		if r.added {
			inserted++
		}
	}
	if inserted != 1 {
		t.Fatalf("concurrent inserted %d", inserted)
	}
	before, e := s.LibraryAsset(ctx, sc, id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`CREATE TRIGGER library_update_fail AFTER UPDATE ON library_assets BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateLibraryAsset(ctx, sc, id, LibraryMetadata{Description: "new", Enabled: true}); !errors.Is(e, ErrStorage) {
		t.Fatal("update abort", e)
	}
	after, e := s.LibraryAsset(ctx, sc, id)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("mutation rollback", e)
	}
	if _, e = s.db.Exec("DROP TRIGGER library_update_fail"); e != nil {
		t.Fatal(e)
	}
	if e = s.MarkLibraryDeleting(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`CREATE TRIGGER library_delete_fail AFTER DELETE ON library_assets BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishLibraryDelete(ctx, sc, id); !errors.Is(e, ErrStorage) {
		t.Fatal("delete abort", e)
	}
	after, e = s.LibraryAsset(ctx, sc, id)
	if e != nil || after.State != "deleting" || after.Enabled {
		t.Fatal("delete rollback", e)
	}
}
func libraryAdd(t *testing.T, s *Store, sc Scope, n int) LibraryAsset {
	t.Helper()
	a, added, e := s.AddLibraryAsset(context.Background(), sc, libraryFixture(sc, n))
	if e != nil || !added || a.ID <= 0 {
		t.Fatalf("insert: added=%v id=%d err=%v", added, a.ID, e)
	}
	return a
}
func TestLibraryScopedLifecycle(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"account", "user"}
	foreign := Scope{"other", "user"}
	a := libraryAdd(t, s, sc, 1)
	if a.Enabled || a.Uses != 0 || a.LastUsedMillis != 0 {
		t.Fatal("new asset enabled or used")
	}
	if _, e := s.LibraryAsset(ctx, foreign, a.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("foreign read %v", e)
	}
	for _, e := range []error{s.UpdateLibraryAsset(ctx, foreign, a.ID, LibraryMetadata{}), s.MarkLibraryDeleting(ctx, foreign, a.ID), s.FinishLibraryDelete(ctx, foreign, a.ID), s.RecordLibraryUse(ctx, foreign, a.ID, 1)} {
		if !errors.Is(e, ErrNotFound) {
			t.Fatalf("foreign mutation %v", e)
		}
	}
	if e := s.RecordLibraryUse(ctx, sc, a.ID, 1); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	meta := LibraryMetadata{Tags: []string{"new"}, Emotion: "calm", Description: "changed", Enabled: true}
	if e := s.UpdateLibraryAsset(ctx, sc, a.ID, meta); e != nil {
		t.Fatal(e)
	}
	if e := s.RecordLibraryUse(ctx, sc, a.ID, 123); e != nil {
		t.Fatal(e)
	}
	got, e := s.LibraryAsset(ctx, sc, a.ID)
	if e != nil || got.Uses != 1 || got.LastUsedMillis != 123 || got.SHA256 != a.SHA256 || got.Bytes != a.Bytes {
		t.Fatalf("usage/immutable: %+v %v", got, e)
	}
	if e := s.FinishLibraryDelete(ctx, sc, a.ID); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if e := s.MarkLibraryDeleting(ctx, sc, a.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.MarkLibraryDeleting(ctx, sc, a.ID); e != nil {
		t.Fatal("idempotent tombstone", e)
	}
	got, e = s.LibraryAsset(ctx, sc, a.ID)
	if e != nil || got.State != "deleting" || got.Enabled {
		t.Fatal("tombstone", e)
	}
	if e := s.UpdateLibraryAsset(ctx, sc, a.ID, meta); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if e := s.RecordLibraryUse(ctx, sc, a.ID, 124); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if e := s.FinishLibraryDelete(ctx, sc, a.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.LibraryAsset(ctx, sc, a.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
}
func TestLibraryDuplicateAndPagination(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	a := libraryAdd(t, s, sc, 1)
	if e := s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{Enabled: true, Description: "original"}); e != nil {
		t.Fatal(e)
	}
	if e := s.RecordLibraryUse(ctx, sc, a.ID, 123); e != nil {
		t.Fatal(e)
	}
	before, _ := s.LibraryAsset(ctx, sc, a.ID)
	dup := libraryFixture(sc, 1)
	dup.Description = "replacement"
	got, added, e := s.AddLibraryAsset(ctx, sc, dup)
	if e != nil || added || !reflect.DeepEqual(got, before) {
		t.Fatalf("duplicate mutated: %v %v", added, e)
	}
	foreign := libraryAdd(t, s, Scope{"b", "u"}, 1)
	if foreign.ID == a.ID {
		t.Fatal("cross-scope dedup")
	}
	b := libraryAdd(t, s, sc, 2)
	page, e := s.LibraryAssets(ctx, sc, a.ID, 1)
	if e != nil || len(page) != 1 || page[0].ID != b.ID {
		t.Fatal("keyset", e)
	}
	all, e := s.LibrarySnapshot(ctx)
	if e != nil || len(all) != 3 {
		t.Fatal("snapshot", e)
	}
	for _, in := range []struct {
		after int64
		limit int
	}{{-1, 1}, {0, 0}, {0, 33}} {
		if _, e = s.LibraryAssets(ctx, sc, in.after, in.limit); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if e = s.MarkLibraryDeleting(ctx, sc, a.ID); e != nil {
		t.Fatal(e)
	}
	before, _ = s.LibraryAsset(ctx, sc, a.ID)
	got, added, e = s.AddLibraryAsset(ctx, sc, dup)
	if e != nil || added || !reflect.DeepEqual(got, before) {
		t.Fatal("duplicate revived tombstone", e)
	}
}
func TestLibraryGlobalQuotasAndTombstones(t *testing.T) {
	ctx := context.Background()
	t.Run("rows", func(t *testing.T) {
		s, _ := testStore(t)
		sc := Scope{"a", "u"}
		a := libraryAdd(t, s, sc, 1)
		for i := 2; i <= 128; i++ {
			libraryAdd(t, s, Scope{"b", "u"}, i)
		}
		if e := s.MarkLibraryDeleting(ctx, sc, a.ID); e != nil {
			t.Fatal(e)
		}
		if _, _, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 129)); !errors.Is(e, ErrCapacity) {
			t.Fatal(e)
		}
		if _, added, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 1)); e != nil || added {
			t.Fatal("duplicate at capacity", e)
		}
		if e := s.FinishLibraryDelete(ctx, sc, a.ID); e != nil {
			t.Fatal(e)
		}
		libraryAdd(t, s, sc, 129)
	})
	t.Run("bytes", func(t *testing.T) {
		s, _ := testStore(t)
		sc := Scope{"a", "u"}
		var first LibraryAsset
		for i := 1; i <= 16; i++ {
			a := libraryFixture(sc, i)
			a.Bytes = 4 << 20
			got, _, e := s.AddLibraryAsset(ctx, sc, a)
			if e != nil {
				t.Fatal(e)
			}
			if i == 1 {
				first = got
			}
		}
		if e := s.MarkLibraryDeleting(ctx, sc, first.ID); e != nil {
			t.Fatal(e)
		}
		if _, _, e := s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 17)); !errors.Is(e, ErrCapacity) {
			t.Fatal(e)
		}
		if e := s.FinishLibraryDelete(ctx, sc, first.ID); e != nil {
			t.Fatal(e)
		}
		libraryAdd(t, s, sc, 17)
	})
}
func TestLibraryInvalidInputsAndOverflow(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	mutations := []func(*LibraryAsset){func(a *LibraryAsset) { a.ID = 1 }, func(a *LibraryAsset) { a.Scope.User = "foreign" }, func(a *LibraryAsset) { a.Enabled = true }, func(a *LibraryAsset) { a.State = "deleting" }, func(a *LibraryAsset) { a.Uses = 1 }, func(a *LibraryAsset) { a.LastUsedMillis = 1 }, func(a *LibraryAsset) { a.SHA256 = strings.Repeat("A", 64) }, func(a *LibraryAsset) { a.Format = "webp" }, func(a *LibraryAsset) { a.Bytes = 0 }, func(a *LibraryAsset) { a.Bytes = (4 << 20) + 1 }, func(a *LibraryAsset) { a.Width = 8193 }, func(a *LibraryAsset) { a.Width = 4001; a.Height = 4000 }, func(a *LibraryAsset) { a.Frames = 2 }, func(a *LibraryAsset) { a.Format = "gif"; a.Frames = 33 }, func(a *LibraryAsset) { a.Tags = []string{"same", "same"} }, func(a *LibraryAsset) { a.Tags = []string{"\n"} }, func(a *LibraryAsset) { a.Tags = make([]string, 17) }, func(a *LibraryAsset) { a.Emotion = "\x7f" }, func(a *LibraryAsset) { a.Description = "\xff" }, func(a *LibraryAsset) { a.Description = strings.Repeat("x", 1025) }}
	for i, f := range mutations {
		a := libraryFixture(sc, 1)
		f(&a)
		if _, _, e := s.AddLibraryAsset(ctx, sc, a); !errors.Is(e, ErrInvalid) {
			t.Fatalf("mutation%d: %v", i, e)
		}
	}
	a := libraryAdd(t, s, sc, 1)
	if e := s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	for _, at := range []int64{0, -1, 253402300800000} {
		if e := s.RecordLibraryUse(ctx, sc, a.ID, at); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if _, e := s.db.Exec("UPDATE library_assets SET uses=? WHERE id=?", int64(math.MaxInt64), a.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.RecordLibraryUse(ctx, sc, a.ID, 123); !errors.Is(e, ErrCapacity) {
		t.Fatal("overflow", e)
	}
	got, e := s.LibraryAsset(ctx, sc, a.ID)
	if e != nil || got.Uses != math.MaxInt64 {
		t.Fatal("overflow changed", e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, e := s.AddLibraryAsset(canceled, sc, libraryFixture(sc, 2)); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel", e)
	}
}
