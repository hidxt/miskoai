package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestProfilesScopedSelectionSnapshotAndDeletion(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	other := Scope{"a", "other"}
	p := Profile{ID: "tea", Name: "Tea", Description: "friendly", Style: "plain", Address: "friend", Length: "normal", Humor: 1, Sticker: "low"}
	if e := s.PutProfile(ctx, sc, p); e != nil {
		t.Fatal(e)
	}
	if e := s.SelectProfile(ctx, other, p.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("foreign %v", e)
	}
	if e := s.SelectProfile(ctx, sc, p.ID); e != nil {
		t.Fatal(e)
	}
	got, e := s.ChatContext(ctx, sc, "", 16, 8)
	if e != nil || got.Profile != p {
		t.Fatalf("context %v %v", got, e)
	}
	p.Style = "changed"
	if e = s.PutProfile(ctx, sc, p); e != nil {
		t.Fatal(e)
	}
	active, e := s.ActiveProfile(ctx, sc)
	if e != nil || active.Style != "changed" {
		t.Fatalf("hot %v %v", active, e)
	}
	if e = s.DeleteProfile(ctx, sc, p.ID); e != nil {
		t.Fatal(e)
	}
	active, e = s.ActiveProfile(ctx, sc)
	if e != nil || active != (Profile{ID: "warm"}) {
		t.Fatalf("fallback %v %v", active, e)
	}
	ps, e := s.Profiles(ctx, other)
	if e != nil || len(ps) != 0 {
		t.Fatalf("isolation %v %v", ps, e)
	}
}

func TestChatContextRejectsOverlongSearchBeforeQuery(t *testing.T) {
	s, _ := testStore(t)
	if _, e := s.ChatContext(context.Background(), Scope{"a", "u"}, strings.Repeat("x", 1025), 1, 1); !errors.Is(e, ErrInvalid) {
		t.Fatalf("query bound bypass: %v", e)
	}
}

func TestProfileValidationAndQuotas(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	base := Profile{ID: "valid", Length: "normal", Sticker: "off"}
	for _, id := range []string{"warm", "concise", "professional", "Upper", "../x", "中文", ""} {
		p := base
		p.ID = id
		if e := s.PutProfile(ctx, sc, p); !errors.Is(e, ErrInvalid) {
			t.Fatalf("ID %q: %v", id, e)
		}
	}
	for i := 0; i < 16; i++ {
		p := base
		p.ID = fmt.Sprintf("p%d", i)
		if e := s.PutProfile(ctx, sc, p); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.PutProfile(ctx, sc, base); !errors.Is(e, ErrCapacity) {
		t.Fatalf("scope quota %v", e)
	}
	for i := 0; i < 16; i++ {
		if e := s.SelectProfile(ctx, Scope{"sel", fmt.Sprint(i)}, "warm"); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.SelectProfile(ctx, sc, "warm"); !errors.Is(e, ErrCapacity) {
		t.Fatalf("selection quota %v", e)
	}
}

func TestProfileFieldsCanonicalJSONAndGlobalQuota(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	base := Profile{ID: "p", Length: "normal", Sticker: "off"}
	cases := []Profile{}
	p := base
	p.Name = strings.Repeat("x", 129)
	cases = append(cases, p)
	p = base
	p.Description = strings.Repeat("x", 1025)
	cases = append(cases, p)
	p = base
	p.Style = strings.Repeat("x", 513)
	cases = append(cases, p)
	p = base
	p.Address = strings.Repeat("x", 129)
	cases = append(cases, p)
	p = base
	p.Humor = 4
	cases = append(cases, p)
	p = base
	p.Length = "long"
	cases = append(cases, p)
	p = base
	p.Sticker = "high"
	cases = append(cases, p)
	p = base
	p.Style = "x\x01"
	cases = append(cases, p)
	p = base
	p.Name = string([]byte{0xff})
	cases = append(cases, p)
	for i, p := range cases {
		if e := s.PutProfile(ctx, Scope{"a", "u"}, p); !errors.Is(e, ErrInvalid) {
			t.Fatalf("field case%d %v", i, e)
		}
	}
	body, e := profileJSON(base)
	if e != nil {
		t.Fatal(e)
	}
	for _, unsafe := range []string{" " + body, strings.TrimSuffix(body, "}") + `,"unknown":true}`, strings.TrimSuffix(body, "}") + `,"id":"p"}`, body + body} {
		if _, e = decodeProfile(unsafe); !errors.Is(e, ErrInvalid) {
			t.Fatalf("noncanonical accepted %q %v", unsafe, e)
		}
	}
	for a := 0; a < 4; a++ {
		for i := 0; i < 16; i++ {
			p := base
			p.ID = fmt.Sprintf("p%d", i)
			if e = s.PutProfile(ctx, Scope{fmt.Sprint(a), "u"}, p); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e = s.PutProfile(ctx, Scope{"fifth", "u"}, base); !errors.Is(e, ErrCapacity) {
		t.Fatalf("global profile count %v", e)
	}
	p = base
	p.Name = "replace within cap"
	if e = s.PutProfile(ctx, Scope{"0", "u"}, p); !errors.Is(e, ErrCapacity) {
		t.Fatalf("new scope row should fail %v", e)
	}
	p.ID = "p0"
	if e = s.PutProfile(ctx, Scope{"0", "u"}, p); e != nil {
		t.Fatalf("existing update refused %v", e)
	}
}

func TestProfileBodyGlobalPayloadQuota(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	p := Profile{ID: "p", Name: strings.Repeat("x", 128), Description: strings.Repeat("<", 1024), Style: strings.Repeat("x", 512), Address: strings.Repeat("x", 128), Length: "normal", Sticker: "normal"}
	var expectedBytes int
	body, e := profileJSON(p)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 64; i++ {
		p.ID = fmt.Sprintf("p%02d", i)
		nextBody, e := profileJSON(p)
		if e != nil {
			t.Fatal(e)
		}
		sc := Scope{fmt.Sprint(i / 16), "u"}
		e = s.PutProfile(ctx, sc, p)
		if expectedBytes+len(nextBody) > 128<<10 {
			if !errors.Is(e, ErrCapacity) {
				t.Fatalf("payload quota accepted %d: %v", expectedBytes+len(nextBody), e)
			}
			var n int
			if e = s.db.QueryRow("SELECT count(*) FROM profiles").Scan(&n); e != nil || n != i {
				t.Fatalf("partial profile %d %v", n, e)
			}
			return
		}
		if e != nil {
			t.Fatal(e)
		}
		expectedBytes += len(nextBody)
	}
	t.Fatalf("fixture did not exercise payload cap: base=%d", len(body))
}
