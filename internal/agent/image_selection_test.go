package agent

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/hidxt/miskoai/internal/storage"
)

func selectionFixture() imageSelectionInput {
	sc := storage.Scope{Account: "acct", User: "user"}
	return imageSelectionInput{Scope: sc, InboundID: "turn-1", UserText: "I am happy!", Reply: "Happy to hear it.", SuggestedID: "7", Profile: storage.Profile{ID: "custom", Name: "Synthetic", Length: "normal", Sticker: "normal"}, Candidates: []storage.LibraryAsset{{ID: 7, Scope: sc, SHA256: strings.Repeat("a", 64), Format: "png", Bytes: 1024, Width: 32, Height: 32, Frames: 1, Tags: []string{" happy "}, State: "ready", Enabled: true}}, NowMillis: 200000000, OrdinarySuccess: true}
}

// Every previously gated identity now accepts the same offered semantic advice.
func TestImageSelectionSemanticAdviceHasNoRandomGate(t *testing.T) {
	for _, tc := range []struct {
		id, frequency string
		want          bool
	}{{"turn-1", "low", true}, {"turn-1", "normal", true}, {"turn-2", "low", true}, {"turn-2", "normal", true}, {"turn-0", "low", true}, {"turn-0", "normal", true}} {
		t.Run(tc.id+"/"+tc.frequency, func(t *testing.T) {
			in := selectionFixture()
			in.InboundID = tc.id
			in.Profile.Sticker = tc.frequency
			got, ok := selectReplyImage(in)
			if ok != tc.want || ok && got.ID != 7 {
				t.Fatalf("got id=%d selected=%v want=%v", got.ID, ok, tc.want)
			}
		})
	}
}

func TestImageSelectionSemanticAdviceNeedsNoLiteralOverlap(t *testing.T) {
	for _, tc := range []struct {
		name, user, reply, tag, emotion, description string
		want                                         bool
	}{
		{"trim and case", "HAPPY!", "happy.", " HAPPY ", "", "", true},
		{"user only", "happy", "sad", "happy", "", "", true},
		{"reply only", "sad", "happy", "happy", "", "", true},
		{"description", "happy", "happy", "sad", "", "happy", true},
		{"text id irrelevant", "asset 7", "asset 7", "happy", "", "", true},
		{"word prefix", "unhappy", "happy", "happy", "", "", true},
		{"word suffix", "happyish", "happy", "happy", "", "", true},
		{"underscore boundary", "happy_person", "happy", "happy", "", "", true},
		{"digit boundary", "happy2", "happy", "happy", "", "", true},
		{"later valid occurrence", "unhappy happy", "happy", "happy", "", "", true},
		{"emotion", "JOY!", "joy.", "sad", " JOY ", "", true},
		{"different semantic words", "happy", "joy", "happy", "joy", "", true},
		{"nonascii literal", "今天开心", "很开心", "开心", "", "", true},
		{"punctuation literal", "a+b", "a+b", "a+b", "", "", true},
		{"blank tags", "happy", "happy", "  ", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := selectionFixture()
			in.UserText = tc.user
			in.Reply = tc.reply
			in.Candidates[0].Tags = []string{tc.tag}
			in.Candidates[0].Emotion = tc.emotion
			in.Candidates[0].Description = tc.description
			_, ok := selectReplyImage(in)
			if ok != tc.want {
				t.Fatalf("selected=%v want=%v", ok, tc.want)
			}
		})
	}
}

func TestImageSelectionAdmissionRefusals(t *testing.T) {
	cases := map[string]func(*imageSelectionInput){
		"ordinary false":          func(i *imageSelectionInput) { i.OrdinarySuccess = false },
		"empty account":           func(i *imageSelectionInput) { i.Scope.Account = "" },
		"empty user":              func(i *imageSelectionInput) { i.Scope.User = " " },
		"account overflow":        func(i *imageSelectionInput) { i.Scope.Account = strings.Repeat("a", 257) },
		"user overflow":           func(i *imageSelectionInput) { i.Scope.User = strings.Repeat("a", 257) },
		"empty inbound":           func(i *imageSelectionInput) { i.InboundID = "" },
		"inbound overflow":        func(i *imageSelectionInput) { i.InboundID = strings.Repeat("a", 513) },
		"user text overflow":      func(i *imageSelectionInput) { i.UserText = strings.Repeat("a", 16385) },
		"reply overflow":          func(i *imageSelectionInput) { i.Reply = strings.Repeat("a", 16385) },
		"invalid utf8":            func(i *imageSelectionInput) { i.UserText = "\xffhappy" },
		"nul scope":               func(i *imageSelectionInput) { i.Scope.Account = "acct\x00" },
		"blank reply":             func(i *imageSelectionInput) { i.Reply = " " },
		"blank user text":         func(i *imageSelectionInput) { i.UserText = " " },
		"empty profile":           func(i *imageSelectionInput) { i.Profile = storage.Profile{} },
		"unknown stub":            func(i *imageSelectionInput) { i.Profile = storage.Profile{ID: "unknown", Sticker: "normal"} },
		"builtin warm":            func(i *imageSelectionInput) { i.Profile.ID = "warm" },
		"builtin concise":         func(i *imageSelectionInput) { i.Profile.ID = "concise" },
		"builtin professional":    func(i *imageSelectionInput) { i.Profile.ID = "professional" },
		"off":                     func(i *imageSelectionInput) { i.Profile.Sticker = "off" },
		"unknown frequency":       func(i *imageSelectionInput) { i.Profile.Sticker = "often" },
		"bad profile id":          func(i *imageSelectionInput) { i.Profile.ID = "Custom" },
		"bad profile length":      func(i *imageSelectionInput) { i.Profile.Length = "huge" },
		"bad profile humor":       func(i *imageSelectionInput) { i.Profile.Humor = 4 },
		"profile style oversized": func(i *imageSelectionInput) { i.Profile.Style = strings.Repeat("x", 513) },
		"profile controls":        func(i *imageSelectionInput) { i.Profile.Name = "\x01" },
		"too many assets":         func(i *imageSelectionInput) { i.Candidates = make([]storage.LibraryAsset, 129) },
		"empty assets":            func(i *imageSelectionInput) { i.Candidates = nil },
		"no suggestion":           func(i *imageSelectionInput) { i.SuggestedID = "" },
		"unoffered suggestion":    func(i *imageSelectionInput) { i.SuggestedID = "8" },
		"overflow suggestion":     func(i *imageSelectionInput) { i.SuggestedID = "9223372036854775808" },
		"noncanonical suggestion": func(i *imageSelectionInput) { i.SuggestedID = "07" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := selectionFixture()
			mutate(&in)
			if got, ok := selectReplyImage(in); ok || got.ID != 0 {
				t.Fatalf("refusal returned %+v, %v", got, ok)
			}
		})
	}
}

func TestImageSelectionMalformedSnapshotFailsClosed(t *testing.T) {
	cases := map[string]func(*storage.LibraryAsset){
		"wrong scope":          func(a *storage.LibraryAsset) { a.Scope.User = "other" },
		"zero id":              func(a *storage.LibraryAsset) { a.ID = 0 },
		"negative id":          func(a *storage.LibraryAsset) { a.ID = -1 },
		"uppercase digest":     func(a *storage.LibraryAsset) { a.SHA256 = strings.Repeat("A", 64) },
		"short digest":         func(a *storage.LibraryAsset) { a.SHA256 = "a" },
		"zero bytes":           func(a *storage.LibraryAsset) { a.Bytes = 0 },
		"oversize bytes":       func(a *storage.LibraryAsset) { a.Bytes = 4<<20 + 1 },
		"width zero":           func(a *storage.LibraryAsset) { a.Width = 0 },
		"width overflow":       func(a *storage.LibraryAsset) { a.Width = 8193 },
		"height overflow":      func(a *storage.LibraryAsset) { a.Height = 8193 },
		"pixel overflow":       func(a *storage.LibraryAsset) { a.Width = 4001; a.Height = 4000 },
		"zero frames":          func(a *storage.LibraryAsset) { a.Frames = 0 },
		"too many frames":      func(a *storage.LibraryAsset) { a.Format = "gif"; a.Frames = 33 },
		"animated png":         func(a *storage.LibraryAsset) { a.Frames = 2 },
		"unknown format":       func(a *storage.LibraryAsset) { a.Format = "webp" },
		"bad state":            func(a *storage.LibraryAsset) { a.State = "unknown" },
		"enabled deleting":     func(a *storage.LibraryAsset) { a.State = "deleting" },
		"negative uses":        func(a *storage.LibraryAsset) { a.Uses = -1 },
		"too many tags":        func(a *storage.LibraryAsset) { a.Tags = make([]string, 17) },
		"duplicate tags":       func(a *storage.LibraryAsset) { a.Tags = []string{"happy", "happy"} },
		"empty tag":            func(a *storage.LibraryAsset) { a.Tags = []string{""} },
		"long tag":             func(a *storage.LibraryAsset) { a.Tags = []string{strings.Repeat("x", 33)} },
		"control tag":          func(a *storage.LibraryAsset) { a.Tags = []string{"happy\t"} },
		"emotion overflow":     func(a *storage.LibraryAsset) { a.Emotion = strings.Repeat("a", 65) },
		"emotion control":      func(a *storage.LibraryAsset) { a.Emotion = "\n" },
		"description overflow": func(a *storage.LibraryAsset) { a.Description = strings.Repeat("a", 1025) },
		"description invalid":  func(a *storage.LibraryAsset) { a.Description = "\xff" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := selectionFixture()
			bad := in.Candidates[0]
			bad.ID = 8
			mutate(&bad)
			in.Candidates = append(in.Candidates, bad)
			if _, ok := selectReplyImage(in); ok {
				t.Fatal("malformed later asset must invalidate snapshot")
			}
		})
	}
	t.Run("duplicate IDs", func(t *testing.T) {
		in := selectionFixture()
		in.Candidates = append(in.Candidates, in.Candidates[0])
		if _, ok := selectReplyImage(in); ok {
			t.Fatal("duplicate id selected")
		}
	})
}

func TestImageSelectionCooldownAndClock(t *testing.T) {
	for _, tc := range []struct {
		name, freq        string
		now, scope, asset int64
		want              bool
	}{
		{"never confirmed", "normal", 200000000, 0, 0, true},
		{"scope normal boundary", "normal", 200000000, 199970000, 0, true},
		{"scope normal inside", "normal", 200000000, 199970001, 0, false},
		{"scope low boundary", "low", 200000000, 199820000, 0, true},
		{"scope low inside", "low", 200000000, 199820001, 0, false},
		{"asset boundary", "normal", 200000000, 0, 199400000, true},
		{"asset inside", "normal", 200000000, 0, 199400001, false},
		{"future scope", "normal", 200000000, 200000001, 0, false},
		{"future asset", "normal", 200000000, 0, 200000001, false},
		{"negative scope", "normal", 200000000, -1, 0, false},
		{"negative asset", "normal", 200000000, 0, -1, false},
		{"negative now", "normal", -1, 0, 0, false},
		{"zero now", "normal", 0, 0, 0, false},
		{"max now", "normal", 253402300799999, 0, 0, true},
		{"out of range now", "normal", 253402300800000, 0, 0, false},
		{"overflow now", "normal", math.MaxInt64, 0, 0, false},
		{"overflow scope", "normal", 200000000, math.MaxInt64, 0, false},
		{"overflow asset", "normal", 200000000, 0, math.MaxInt64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := selectionFixture()
			in.NowMillis = tc.now
			in.LastConfirmedMillis = tc.scope
			in.Candidates[0].LastUsedMillis = tc.asset
			in.Profile.Sticker = tc.freq
			_, ok := selectReplyImage(in)
			if ok != tc.want {
				t.Fatalf("selected=%v want=%v", ok, tc.want)
			}
		})
	}
}

func TestImageSelectionOrderSkipAndImmutableResult(t *testing.T) {
	in := selectionFixture()
	base := in.Candidates[0]
	low := base
	low.ID = 3
	high := base
	high.ID = 9
	disabled := base
	disabled.ID = 1
	disabled.Enabled = false
	deleting := disabled
	deleting.ID = 2
	deleting.State = "deleting"
	gif := base
	gif.ID = 4
	gif.Format = "gif"
	gif.Frames = 2
	stale := base
	stale.ID = 5
	stale.LastUsedMillis = in.NowMillis
	in.Candidates = []storage.LibraryAsset{high, gif, disabled, base, deleting, stale, low}
	in.SuggestedID = "3"
	before := make([]storage.LibraryAsset, len(in.Candidates))
	for n, a := range in.Candidates {
		before[n] = a
		before[n].Tags = append([]string(nil), a.Tags...)
	}
	for run := 0; run < 2; run++ {
		got, ok := selectReplyImage(in)
		if !ok || got.ID != 3 || got.SHA256 != strings.Repeat("a", 64) {
			t.Fatalf("got %+v selected=%v", got, ok)
		}
		got.Tags[0] = "changed"
		if !reflect.DeepEqual(in.Candidates, before) {
			t.Fatal("input mutated or result aliases tags")
		}
		for l, r := 0, len(in.Candidates)-1; l < r; l, r = l+1, r-1 {
			in.Candidates[l], in.Candidates[r] = in.Candidates[r], in.Candidates[l]
			before[l], before[r] = before[r], before[l]
		}
	}
	t.Run("jpeg bounds", func(t *testing.T) {
		in := selectionFixture()
		a := &in.Candidates[0]
		a.Format = "jpeg"
		a.Bytes = 4 << 20
		a.Width = 4000
		a.Height = 4000
		a.Uses = math.MaxInt64
		a.Description = "\n"
		if got, ok := selectReplyImage(in); !ok || got.ID != 7 {
			t.Fatal("current storage maxima refused")
		}
	})
	t.Run("128 admitted records", func(t *testing.T) {
		in := selectionFixture()
		a := in.Candidates[0]
		in.Candidates = make([]storage.LibraryAsset, 128)
		in.SuggestedID = "1"
		for n := range in.Candidates {
			in.Candidates[n] = a
			in.Candidates[n].ID = int64(n + 1)
		}
		if got, ok := selectReplyImage(in); !ok || got.ID != 1 {
			t.Fatal("128 snapshot refused")
		}
	})
}

func TestImageSelectionEarlyRefusalAllocations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*imageSelectionInput)
	}{
		{"count", func(i *imageSelectionInput) { i.Candidates = make([]storage.LibraryAsset, 100000) }},
		{"scope", func(i *imageSelectionInput) { i.Scope.Account = strings.Repeat("a", 1000000) }},
		{"id", func(i *imageSelectionInput) { i.InboundID = strings.Repeat("a", 1000000) }},
		{"text", func(i *imageSelectionInput) { i.UserText = strings.Repeat("a", 1000000) }},
		{"reply", func(i *imageSelectionInput) { i.Reply = strings.Repeat("a", 1000000) }},
		{"eligibility", func(i *imageSelectionInput) { i.OrdinarySuccess = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := selectionFixture()
			tc.mutate(&in)
			if n := testing.AllocsPerRun(10, func() {
				if _, ok := selectReplyImage(in); ok {
					panic("selected")
				}
			}); n != 0 {
				t.Fatalf("early refusal allocated %v", n)
			}
		})
	}
}

// Scope/identity Unicode or escaping never adds a random selection gate.
func TestImageSelectionScopeIdentityDoesNotChangeAdvice(t *testing.T) {
	for _, tc := range []struct {
		account, user, id string
		want              bool
	}{
		{"ab", "c", "turn-0", true}, {"a", "bc", "turn-0", true},
		{"a<&>", "user", "turn-0", true}, {"账", "用户", "turn-1", true},
	} {
		t.Run(tc.account+"/"+tc.user, func(t *testing.T) {
			in := selectionFixture()
			in.Scope = storage.Scope{Account: tc.account, User: tc.user}
			in.InboundID = tc.id
			in.Candidates[0].Scope = in.Scope
			if got, ok := selectReplyImage(in); ok != tc.want || ok && got.ID != 7 {
				t.Fatalf("got %d %v want=%v", got.ID, ok, tc.want)
			}
		})
	}
}

func TestImageSelectionExplicitPathIneligibility(t *testing.T) {
	for _, path := range []string{"attachment", "command", "search", "error", "cancelled", "unvalidated reply"} {
		t.Run(path, func(t *testing.T) {
			in := selectionFixture()
			in.OrdinarySuccess = false
			if _, ok := selectReplyImage(in); ok {
				t.Fatal("explicitly ineligible ordinary-success flag ignored")
			}
		})
	}
}

func TestImageSelectionNonselectedMalformedAndFutureRefuse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*storage.LibraryAsset)
	}{
		{"disabled malformed", func(a *storage.LibraryAsset) { a.Enabled = false; a.Description = strings.Repeat("a", 1025) }},
		{"disabled future", func(a *storage.LibraryAsset) { a.Enabled = false; a.LastUsedMillis = 200000001 }},
		{"gif malformed", func(a *storage.LibraryAsset) { a.Format = "gif"; a.Tags = []string{"\xff"} }},
		{"deleting future", func(a *storage.LibraryAsset) { a.Enabled = false; a.State = "deleting"; a.LastUsedMillis = 200000001 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := selectionFixture()
			other := in.Candidates[0]
			other.ID = 8
			tc.mutate(&other)
			in.Candidates = append(in.Candidates, other)
			if _, ok := selectReplyImage(in); ok {
				t.Fatal("malformed nonselected row allowed a partial selection")
			}
		})
	}
}

func TestImageSelectionCanonicalTagJSONBound(t *testing.T) {
	// Fifteen tags each encode as123bytes, last encodes186bytes;16 tags
	// have15 commas plus2 brackets:15*123+186+15+2=2048. A quote
	// replacing the final letter of tag15 adds one escaped byte =>2049.
	for _, tc := range []struct {
		name     string
		overflow bool
		want     bool
	}{{"exact2048", false, true}, {"over2049", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			in := selectionFixture()
			a := &in.Candidates[0]
			a.Emotion = "happy"
			a.Tags = make([]string, 16)
			for n := 0; n < 15; n++ {
				a.Tags[n] = strings.Repeat("<", 20) + string(rune('a'+n))
			}
			a.Tags[15] = strings.Repeat("<", 30) + "\\\\"
			if tc.overflow {
				a.Tags[14] = strings.Repeat("<", 20) + "\""
			}
			if _, ok := selectReplyImage(in); ok != tc.want {
				t.Fatalf("selected=%v want=%v", ok, tc.want)
			}
		})
	}
	t.Run("unicode separators escape", func(t *testing.T) {
		in := selectionFixture()
		a := &in.Candidates[0]
		a.Emotion = "happy"
		a.Tags = []string{"\u2028\u2029"}
		if _, ok := selectReplyImage(in); !ok {
			t.Fatal("valid Unicode escaping refused")
		}
	})
}
