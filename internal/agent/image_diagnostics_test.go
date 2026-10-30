package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/storage"
	"testing"
)

func TestImageDecisionDiagnosticHandle(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		images          int
	}{
		{"plain", "safe plaintext", "plain_text", 0},
		{"none", `{"reply":"safe","emotion":"joy","image_id":""}`, "no_suggestion", 0},
		{"neutral", `{"reply":"safe","emotion":"neutral","image_id":"1"}`, "no_suggestion", 0},
		{"numeric", `{"reply":"safe","emotion":"joy","image_id":1}`, "invalid_advice", 0},
		{"unknown", `{"reply":"safe","emotion":"joy","image_id":"99999"}`, "not_offered", 0},
		{"malformed", `{"reply":"safe","emotion":`, "invalid_output", 0},
		{"acked", `{"reply":"safe","emotion":"joy","image_id":"1"}`, "image_acked", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReplyFixture(t)
			f.m.reply.Text = tc.raw
			r, e := f.a.Handle(context.Background(), f.incoming())
			b, _ := json.Marshal(r)
			var v map[string]any
			json.Unmarshal(b, &v)
			if e != nil || v["ImageDecision"] != tc.want || f.sender.calls != 1 || f.sender.images != tc.images || f.m.calls != 1 {
				t.Fatalf("decision=%v result=%+v err=%v texts=%d images=%d model=%d", v["ImageDecision"], r, e, f.sender.calls, f.sender.images, f.m.calls)
			}
			before := f.sender.calls
			duplicate, _ := f.a.Handle(context.Background(), f.incoming())
			b, _ = json.Marshal(duplicate)
			json.Unmarshal(b, &v)
			if v["ImageDecision"] != "" || f.sender.calls != before {
				t.Fatal("duplicate retained diagnostic or replayed", string(b))
			}
		})
	}
}

func TestImageDecisionDiagnosticGates(t *testing.T) {
	for _, mode := range []string{"off", "empty", "unavailable", "length", "model", "cooldown", "pipeline"} {
		t.Run(mode, func(t *testing.T) {
			f := newReplyFixture(t)
			ctx := context.Background()
			a := f.a
			want := mode
			switch mode {
			case "off":
				if e := f.s.PutProfile(ctx, a.scope, storage.Profile{ID: "custom", Name: "Synthetic", Length: "normal", Sticker: "off"}); e != nil {
					t.Fatal(e)
				}
				want = "profile_off"
				f.m.reply.Text = "safe"
			case "empty":
				if e := f.s.UpdateLibraryAsset(ctx, a.scope, f.asset.ID, storage.LibraryMetadata{Enabled: false}); e != nil {
					t.Fatal(e)
				}
				want = "empty_catalogue"
				f.m.reply.Text = "safe"
			case "unavailable":
				a = f.base
				f.m.reply.Text = "safe"
			case "length":
				f.m.reply.FinishReason = "length"
				want = "output_limited"
			case "model":
				f.m.err = errors.New("private-provider-canary")
				want = "model_error"
			case "pipeline":
				f.prepareHook = func(context.Context) error { return errors.New("private-upload-canary") }
				want = "image_pipeline"
			case "cooldown":
				r, e := a.Handle(ctx, f.incoming())
				if e != nil || r.ImageState != "acked" {
					t.Fatal(r, e)
				}
			}
			r, _ := a.Handle(ctx, incoming("diagnostic-new-turn", "synthetic context"))
			b, _ := json.Marshal(r)
			var v map[string]any
			json.Unmarshal(b, &v)
			if v["ImageDecision"] != want {
				t.Fatalf("want %s got %s", want, b)
			}
			expected := 1
			if mode == "cooldown" {
				expected = 2
			}
			if f.m.calls != expected || f.sender.images > 1 {
				t.Fatal("diagnostic altered calls", f.m.calls, f.sender.images)
			}
			if mode == "pipeline" && f.sender.calls != 0 {
				t.Fatal("failed preparation sent text")
			}
		})
	}
}

func TestImageDecisionDiagnosticReviewEdges(t *testing.T) {
	for _, mode := range []string{"blank", "cancel", "snapshot"} {
		t.Run(mode, func(t *testing.T) {
			f := newReplyFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want, code, texts, calls := "invalid_output", "output", 1, 1
			switch mode {
			case "blank":
				f.m.reply.Text = " "
			case "cancel":
				f.m.hook = func(context.Context) { cancel() }
				want, code, texts = "canceled", "canceled", 0
			case "snapshot":
				f.sql(t, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<128) INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) SELECT 'other','scope',printf('%064x',i),'png',64,2,2,1,'[]','','','ready',1,0,0 FROM n`)
				want, code, texts, calls = "snapshot_error", "capacity", 0, 0
			}
			r, _ := f.a.Handle(ctx, f.incoming())
			if r.ImageDecision != want || r.Code != code || f.sender.calls != texts || f.sender.images != 0 || f.m.calls != calls {
				t.Fatalf("result=%+v texts=%d images=%d calls=%d", r, f.sender.calls, f.sender.images, f.m.calls)
			}
			if duplicate, _ := f.a.Handle(context.Background(), f.incoming()); duplicate.State != "duplicate" || f.sender.calls != texts {
				t.Fatal("diagnostic error replayed side effects", duplicate)
			}
		})
	}
}
