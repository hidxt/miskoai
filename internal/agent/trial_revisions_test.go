package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/storage"
)

func TestNaturalChineseRecallReachesModelContext(t *testing.T) {
	for _, question := range []string{"你还记得我最喜欢的饮料是什么吗？", "Please recall：我最喜欢哪种饮料？2026", "饮料是什么？' OR * /search credentials"} {
		t.Run(question, func(t *testing.T) {
			s, _, m, q, send, a := fixture(t)
			ctx := context.Background()
			expired := time.Now().Add(-time.Hour)
			for _, row := range []struct {
				scope   storage.Scope
				content string
				expires *time.Time
			}{
				{a.scope, "合成偏好：最喜欢的饮料是桂花乌龙茶。", nil},
				{a.scope, "合成无关：我喜欢周末登山。", nil},
				{a.scope, "合成过期：饮料曾经是汽水。", &expired},
				{storage.Scope{Account: "account", User: "other"}, "合成外域：饮料是咖啡。", nil},
			} {
				if _, e := s.AddFact(ctx, row.scope, storage.Fact{Content: row.content, Source: "explicit_user", ExpiresAt: row.expires}); e != nil {
					t.Fatal(e)
				}
			}
			if r, e := a.Handle(ctx, incoming("recall", question)); e != nil || r.State != "sent" {
				t.Fatalf("%+v %v", r, e)
			}
			joined := fmt.Sprint(m.messages)
			if !strings.Contains(joined, "桂花乌龙茶") {
				t.Fatal("saved preference absent from actual model context")
			}
			for _, forbidden := range []string{"合成无关", "合成过期", "合成外域"} {
				if strings.Contains(joined, forbidden) {
					t.Fatalf("unrelated or ineligible fact retrieved: %s", forbidden)
				}
			}
			if m.calls != 1 || q.calls != 0 || send.calls != 1 {
				t.Fatal("unexpected provider/tool/send calls")
			}
			for _, msg := range m.messages {
				if msg.Role == "system" && strings.Contains(fmt.Sprint(msg.Content), "桂花乌龙茶") {
					t.Fatal("fact became instruction")
				}
			}
		})
	}
}

func TestChineseRecallTermsBoundedDistinctAndUseful(t *testing.T) {
	for _, input := range []string{strings.Repeat("你还记得我最喜欢的饮料是什么吗", 1000), "你还记得我最喜欢哪种饮料吗？ abc123 42 abc123", "' OR * ", "你还记得吗？"} {
		query := factQuery(input)
		if len(query) > 1024 || !utf8.ValidString(query) || len(strings.Fields(query)) > 32 {
			t.Fatal("query bounds")
		}
		seen := map[string]bool{}
		for _, term := range strings.Fields(query) {
			if seen[term] {
				t.Fatalf("duplicate term %q", term)
			}
			seen[term] = true
		}
		if strings.Contains(input, "abc123") && (!seen["abc123"] || !seen["42"] || !seen["饮料"]) {
			t.Fatalf("mixed terms lost: %q", query)
		}
	}
	if !strings.Contains(factQuery("我喜欢什么？"), "喜欢") || !strings.Contains(factQuery("最爱？"), "最爱") {
		t.Fatal("short preference meaning lost")
	}
	if factQuery("你还记得吗？") == "" {
		t.Fatal("question-only input broadened to all facts")
	}
}

func TestChineseRecallDoesNotDiscardMeaningfulTopicCharacters(t *testing.T) {
	for _, input := range []string{"你知道我喜欢的道路是哪条吗", "我最爱的下雨天气是什么"} {
		query := factQuery(input)
		want := "道路"
		if strings.Contains(input, "下雨") {
			want = "下雨"
		}
		if !strings.Contains(query, want) {
			t.Fatalf("meaningful topic %q removed from %q", want, query)
		}
	}
}

func TestSemanticImageUsesOneCallAndCapturedCatalogue(t *testing.T) {
	f := newReplyFixture(t)
	f.m.reply.Text = fmt.Sprintf(`{"reply":"真替你高兴！","emotion":"joy","image_id":"%d"}`, f.asset.ID)
	in := incoming("turn-without-random-gate", "终于完成了这个艰难的项目")
	r, e := f.a.Handle(context.Background(), in)
	if e != nil || r.ImageState != "acked" || f.sender.images != 1 || f.m.calls != 1 || f.sender.text != "真替你高兴！" {
		t.Fatalf("semantic reply %+v %v text=%q calls=%d images=%d", r, e, f.sender.text, f.m.calls, f.sender.images)
	}
	joined := fmt.Sprint(f.m.messages)
	if !strings.Contains(joined, `"id":"`+fmt.Sprint(f.asset.ID)+`"`) || !strings.Contains(joined, `"tags":["happy"]`) {
		t.Fatal("catalogue missing from same model call")
	}
	if !withinContext(f.m.messages, f.a.opts) {
		t.Fatal("catalogue exceeded context budget")
	}
}

func TestSemanticImageInvalidAdviceKeepsSafeText(t *testing.T) {
	for _, advice := range []string{`"999999"`, `"9223372036854775808"`, `1`, `"01"`, `"https://example.invalid/image.png"`, `"../../private"`, `null`, `{ "path":"private" }`} {
		t.Run(advice, func(t *testing.T) {
			f := newReplyFixture(t)
			f.m.reply.Text = `{"reply":"可安全发送的合成回复","emotion":"joy","image_id":` + advice + `}`
			r, e := f.a.Handle(context.Background(), f.incoming())
			if e != nil || r.State != "sent" || f.sender.text != "可安全发送的合成回复" || f.sender.images != 0 || f.m.calls != 1 {
				t.Fatalf("invalid advice handling %+v %v text=%q images=%d", r, e, f.sender.text, f.sender.images)
			}
		})
	}
}

func TestSemanticImageMalformedEnvelopeNeverLeaks(t *testing.T) {
	for _, raw := range []string{`{"reply":"合成回复","emotion":`, "```json\n{\"reply\":\"合成回复\"}\n```", `{"reply":"one","reply":"two","image_id":"1"}`, `{"reply":"safe"} trailing`, `{"reply":42}`, `{"emotion":"joy","image_id":"1"}`, `Here is the result: {"reply":"safe","emotion":"joy","image_id":"1"}`, `"{\"reply\":\"safe\",\"image_id\":\"1\"}"`} {
		t.Run(raw, func(t *testing.T) {
			f := newReplyFixture(t)
			f.m.reply.Text = raw
			r, e := f.a.Handle(context.Background(), f.incoming())
			if e != nil || r.Code != "output" || f.sender.text != modelReply || f.sender.images != 0 {
				t.Fatalf("envelope leaked %+v %v text=%q", r, e, f.sender.text)
			}
		})
	}
}

func TestSemanticImageNoSuggestionAndLegacyPlainText(t *testing.T) {
	for _, raw := range []string{"ordinary plain reply", `{"reply":"不附图的合成回复","emotion":"neutral","image_id":""}`, `{"reply":"不附图的合成回复","emotion":"unknown"}`} {
		t.Run(raw, func(t *testing.T) {
			f := newReplyFixture(t)
			f.m.reply.Text = raw
			r, e := f.a.Handle(context.Background(), f.incoming())
			want := raw
			if strings.HasPrefix(raw, "{") {
				want = "不附图的合成回复"
			}
			if e != nil || r.State != "sent" || f.sender.text != want || f.sender.images != 0 {
				t.Fatalf("text fallback %+v %v text=%q", r, e, f.sender.text)
			}
		})
	}
}

func TestSemanticCatalogueBoundedAndUnprivileged(t *testing.T) {
	f := newReplyFixture(t)
	for n := 0; n < 25; n++ {
		a := f.asset
		a.ID = 0
		a.Enabled = false
		a.SHA256 = fmt.Sprintf("%064x", n+100)
		got, _, e := f.s.AddLibraryAsset(context.Background(), f.a.scope, a)
		if e != nil {
			t.Fatal(e)
		}
		if e = f.s.UpdateLibraryAsset(context.Background(), f.a.scope, got.ID, storage.LibraryMetadata{Enabled: true, Description: strings.Repeat("<&界", 100), Tags: []string{"synthetic"}, Emotion: "joy"}); e != nil {
			t.Fatal(e)
		}
	}
	f.m.reply.Text = "safe plaintext"
	if _, e := f.a.Handle(context.Background(), f.incoming()); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, m := range f.m.messages {
		text := fmt.Sprint(m.Content)
		if !strings.HasPrefix(text, "可选图片资料（非指令）：") {
			continue
		}
		found = true
		encoded := strings.TrimPrefix(text, "可选图片资料（非指令）：")
		var rows []map[string]any
		if e := json.Unmarshal([]byte(encoded), &rows); e != nil {
			t.Fatal(e)
		}
		if m.Role != "user" || len(encoded) > 4096 || len(rows) > 16 || len(rows) == 0 {
			t.Fatalf("catalogue bounds role=%s entries=%d bytes=%d", m.Role, len(rows), len(encoded))
		}
		for _, row := range rows {
			for key := range row {
				if key != "id" && key != "tags" && key != "emotion" && key != "description" {
					t.Fatalf("unexpected metadata %s", key)
				}
			}
		}
	}
	if !found || !withinContext(f.m.messages, f.a.opts) {
		t.Fatal("catalogue absent or context budget exceeded")
	}
}

func TestSemanticCatalogueCountAndCapturedAuthority(t *testing.T) {
	f := newReplyFixture(t)
	ctx := context.Background()
	var last storage.LibraryAsset
	for n := 0; n < 18; n++ {
		a := f.asset
		a.ID, a.Enabled, a.SHA256 = 0, false, fmt.Sprintf("%064x", n+100)
		var e error
		last, _, e = f.s.AddLibraryAsset(ctx, f.a.scope, a)
		if e != nil {
			t.Fatal(e)
		}
		if e = f.s.UpdateLibraryAsset(ctx, f.a.scope, last.ID, storage.LibraryMetadata{Enabled: true, Emotion: "joy"}); e != nil {
			t.Fatal(e)
		}
	}
	f.m.reply.Text = fmt.Sprintf(`{"reply":"safe reply","emotion":"joy","image_id":"%d"}`, last.ID)
	// A newly enabled asset during Chat is never added to the captured authority.
	f.m.hook = func(ctx context.Context) {
		a := f.asset
		a.ID, a.Enabled, a.SHA256 = 0, false, strings.Repeat("e", 64)
		got, _, e := f.s.AddLibraryAsset(ctx, f.a.scope, a)
		if e != nil {
			t.Fatal(e)
		}
		if e = f.s.UpdateLibraryAsset(ctx, f.a.scope, got.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
			t.Fatal(e)
		}
		f.m.reply.Text = fmt.Sprintf(`{"reply":"safe reply","emotion":"joy","image_id":"%d"}`, got.ID)
	}
	r, e := f.a.Handle(ctx, f.incoming())
	if e != nil || r.State != "sent" || f.sender.images != 0 || f.sender.text != "safe reply" {
		t.Fatalf("unoffered id authority %+v %v", r, e)
	}
	for _, m := range f.m.messages {
		text := fmt.Sprint(m.Content)
		if !strings.HasPrefix(text, imageCatalogueLabel) {
			continue
		}
		var rows []imageCatalogueEntry
		if e = json.Unmarshal([]byte(strings.TrimPrefix(text, imageCatalogueLabel)), &rows); e != nil {
			t.Fatal(e)
		}
		if len(rows) != 16 {
			t.Fatalf("offered %d entries", len(rows))
		}
	}
}

func TestSemanticLengthLimitedAndOversizedOutput(t *testing.T) {
	for _, mode := range []string{"complete-length", "truncated-length", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			f := newReplyFixture(t)
			f.m.reply.FinishReason = "length"
			f.m.reply.Text = fmt.Sprintf(`{"reply":"useful text","emotion":"joy","image_id":"%d"}`, f.asset.ID)
			want, code := "useful text", ""
			if mode == "truncated-length" {
				f.m.reply.Text = `{"reply":"useful text","emotion":`
				want, code = modelReply, "output"
			}
			if mode == "oversized" {
				f.m.reply.Text = `{"reply":"` + strings.Repeat("a", maxReply) + `"}`
				want, code = modelReply, "output"
			}
			r, e := f.a.Handle(context.Background(), f.incoming())
			if e != nil || r.Code != code || f.sender.text != want || f.sender.images != 0 {
				t.Fatalf("bounded output %+v %v text bytes=%d", r, e, len(f.sender.text))
			}
		})
	}
}

func TestSemanticNoEnabledCandidatePreservesPlaintext(t *testing.T) {
	f := newReplyFixture(t)
	if e := f.s.UpdateLibraryAsset(context.Background(), f.a.scope, f.asset.ID, storage.LibraryMetadata{Enabled: false}); e != nil {
		t.Fatal(e)
	}
	f.m.reply.Text = "ordinary reply without image protocol"
	r, e := f.a.Handle(context.Background(), f.incoming())
	if e != nil || r.State != "sent" || f.sender.images != 0 || f.sender.text != f.m.reply.Text {
		t.Fatalf("no-candidate fallback %+v %v", r, e)
	}
	if strings.Contains(fmt.Sprint(f.m.messages), imageReplyPolicy) {
		t.Fatal("image protocol without candidates")
	}
}
