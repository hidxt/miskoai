package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type replyArtifact struct {
	*bytes.Reader
	closed bool
}

func (a *replyArtifact) Size() int64  { return a.Reader.Size() }
func (a *replyArtifact) Close() error { a.closed = true; return nil }

type replySender struct {
	*fakeSender
	images  int
	imageID string
	onImage func(context.Context) error
}

func (s *replySender) SendImage(ctx context.Context, to, token, id string, image weixin.ImageSend) error {
	s.images++
	s.imageID = id
	if s.onImage != nil {
		return s.onImage(ctx)
	}
	return nil
}

type replyFixture struct {
	workContext context.Context
	path        string
	a, base     *Agent
	s           *storage.Store
	m           *fakeModel
	sender      *replySender
	asset       storage.LibraryAsset
	local       attachment.LocalImage
	raw         []byte
	order       []string
	artifact    *replyArtifact
	borrowHook  func(context.Context, *storage.LibraryAsset, *attachment.LocalImage) error
	prepareHook func(context.Context) error
	closeHook   func() error
	borrowStore *storage.Store
}

func newReplyFixture(t *testing.T) *replyFixture {
	t.Helper()
	s, path, m, _, send, base := fixture(t)
	f := &replyFixture{path: path, s: s, m: m, base: base, sender: &replySender{fakeSender: send}, borrowStore: s}
	base.sender = f.sender
	m.reply.Text = "happy!"
	p := storage.Profile{ID: "custom", Name: "Synthetic", Length: "normal", Sticker: "normal"}
	if err := s.PutProfile(context.Background(), base.scope, p); err != nil {
		t.Fatal(err)
	}
	if err := s.SelectProfile(context.Background(), base.scope, p.ID); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	f.raw = b.Bytes()
	digest := sha256.Sum256(f.raw)
	info, err := media.ValidateImage(context.Background(), bytes.NewReader(f.raw), int64(len(f.raw)))
	if err != nil {
		t.Fatal(err)
	}
	f.local = attachment.LocalImage{Info: info, SHA256: hex.EncodeToString(digest[:]), Bytes: int64(len(f.raw))}
	a, _, err := s.AddLibraryAsset(context.Background(), base.scope, storage.LibraryAsset{Scope: base.scope, SHA256: f.local.SHA256, Format: "png", Bytes: f.local.Bytes, Width: 2, Height: 2, Frames: 1, State: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateLibraryAsset(context.Background(), base.scope, a.ID, storage.LibraryMetadata{Tags: []string{"happy"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.asset, err = s.LibraryAsset(context.Background(), base.scope, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.a, err = NewWithImageReplies(base, f.borrow, f.prepare)
	if err != nil {
		t.Fatal(err)
	}
	send.hook = func(ctx context.Context) { f.order = append(f.order, "text") }
	f.sender.onImage = func(ctx context.Context) error { f.order = append(f.order, "image"); return nil }
	return f
}
func (f *replyFixture) borrow(ctx context.Context, id int64, digest string, consume func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
	f.order = append(f.order, "borrow")
	row, err := f.s.LibraryAsset(ctx, f.base.scope, id)
	if err != nil {
		return err
	}
	local := f.local
	if f.borrowHook != nil {
		if err = f.borrowHook(ctx, &row, &local); err != nil {
			return err
		}
	}
	f.artifact = &replyArtifact{Reader: bytes.NewReader(f.raw)}
	work := ctx
	if f.workContext != nil {
		work = f.workContext
	}
	err = consume(work, f.borrowStore, row, local, f.artifact)
	_ = f.artifact.Close()
	f.order = append(f.order, "close")
	if f.closeHook != nil {
		err = errors.Join(err, f.closeHook())
	}
	return err
}
func (f *replyFixture) prepare(ctx context.Context, to string, local attachment.LocalImage, a attachment.Artifact) (weixin.ImageSend, error) {
	f.order = append(f.order, "prepare")
	if f.prepareHook != nil {
		if err := f.prepareHook(ctx); err != nil {
			return weixin.ImageSend{}, err
		}
	}
	return weixin.ImageSend{EncryptQueryParam: "synthetic-upload", AESKeyHex: "00000000000000000000000000000000", CiphertextBytes: 80}, nil
}
func (f *replyFixture) incoming() Incoming {
	for n := 0; ; n++ {
		in := incoming(fmt.Sprint("image-turn-", n), "happy")
		h := sha256.Sum256([]byte(fmt.Sprintf(`["account","alice",%q]`, in.ID)))
		if h[0]%8 < 2 {
			return in
		}
	}
}
func TestImageReplyOrchestrationSuccess(t *testing.T) {
	f := newReplyFixture(t)
	in := f.incoming()
	result, err := f.a.Handle(context.Background(), in)
	if err != nil || result.State != "sent" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(f.order, []string{"borrow", "prepare", "text", "image", "close"}) {
		t.Fatalf("order=%v", f.order)
	}
	receipt, err := f.s.ImageReplyReceipt(context.Background(), f.a.scope, in.ID)
	if err != nil || receipt.TextPhase != "acked" || receipt.ImagePhase != "acked" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	if f.sender.id != stableClientID(in) || f.sender.imageID == f.sender.id || receipt.ImageClientID != f.sender.imageID {
		t.Fatal("client IDs")
	}
	row, err := f.s.LibraryAsset(context.Background(), f.a.scope, f.asset.ID)
	if err != nil || row.Uses != 1 {
		t.Fatalf("uses=%d err=%v", row.Uses, err)
	}
	for _, a := range []*Agent{f.a, f.base} {
		r, e := a.Handle(context.Background(), in)
		if e != nil || r.State != "duplicate" {
			t.Fatalf("duplicate=%+v %v", r, e)
		}
	}
	if f.sender.calls != 1 || f.sender.images != 1 || !f.artifact.closed {
		t.Fatal("once-only send/lifetime")
	}
}

func (f *replyFixture) receipt(t *testing.T, in Incoming) storage.ImageReply {
	t.Helper()
	r, e := f.s.ImageReplyReceipt(context.Background(), f.a.scope, in.ID)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func (f *replyFixture) uses(t *testing.T) int64 {
	t.Helper()
	r, e := f.s.LibraryAsset(context.Background(), f.a.scope, f.asset.ID)
	if e != nil {
		t.Fatal(e)
	}
	return r.Uses
}
func (f *replyFixture) sql(t *testing.T, query string, args ...any) {
	t.Helper()
	db, e := sql.Open("sqlite", f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e = db.ExecContext(ctx, query, args...); e != nil {
		t.Fatal(e)
	}
}
func TestImageReplyOrchestrationConstructor(t *testing.T) {
	f := newReplyFixture(t)
	if f.base.replyImageBorrow != nil || f.base.replyImagePrepare != nil || f.a == f.base || f.a.admission != f.base.admission {
		t.Fatal("mutable composition/admission")
	}
	for _, tc := range []struct {
		base    *Agent
		borrow  ReplyImageBorrow
		prepare ReplyImagePrepare
	}{{nil, f.borrow, f.prepare}, {f.base, nil, f.prepare}, {f.base, f.borrow, nil}, {&Agent{}, f.borrow, f.prepare}} {
		if a, e := NewWithImageReplies(tc.base, tc.borrow, tc.prepare); e == nil || a != nil {
			t.Fatal("partial composition accepted")
		}
	}
	_, _, _, _, _, textOnly := fixture(t)
	if a, e := NewWithImageReplies(textOnly, f.borrow, f.prepare); e == nil || a != nil {
		t.Fatal("unsupported sender accepted")
	}
	if len(f.order) != 0 || f.sender.calls != 0 {
		t.Fatal("constructor I/O")
	}
	// A truly reconstructed Agent shares durable dedup, rather than an in-memory cache.
	in := f.incoming()
	if _, e := f.a.Handle(context.Background(), in); e != nil {
		t.Fatal(e)
	}
	reconstructed, e := New(f.s, f.m, nil, f.sender, f.base.opts)
	if e != nil {
		t.Fatal(e)
	}
	reconstructed, e = NewWithImageReplies(reconstructed, f.borrow, f.prepare)
	if e != nil {
		t.Fatal(e)
	}
	if r, e := reconstructed.Handle(context.Background(), in); e != nil || r.State != "duplicate" {
		t.Fatalf("replay %+v %v", r, e)
	}
	if f.sender.calls != 1 || f.sender.images != 1 {
		t.Fatal("replayed")
	}
}
func TestImageReplyOrchestrationOriginalProfile(t *testing.T) {
	for _, initial := range []string{"normal", "low", "off"} {
		t.Run(initial, func(t *testing.T) {
			f := newReplyFixture(t)
			p := storage.Profile{ID: "custom", Name: "Synthetic", Length: "normal", Sticker: initial}
			if e := f.s.PutProfile(context.Background(), f.a.scope, p); e != nil {
				t.Fatal(e)
			}
			f.m.hook = func(ctx context.Context) {
				p.Sticker = "off"
				if initial == "off" {
					p.Sticker = "normal"
				}
				if e := f.s.PutProfile(ctx, f.a.scope, p); e != nil {
					t.Fatal(e)
				}
			}
			in := f.incoming()
			if initial == "low" {
				for n := 0; ; n++ {
					in.ID = fmt.Sprint("low-", n)
					h := sha256.Sum256([]byte(fmt.Sprintf(`["account","alice",%q]`, in.ID)))
					if h[0]%8 == 0 {
						break
					}
				}
			}
			r, e := f.a.Handle(context.Background(), in)
			if e != nil {
				t.Fatal(e)
			}
			want := 1
			if initial == "off" {
				want = 0
			}
			if f.sender.images != want || (want == 0 && r.ImageState != "") {
				t.Fatalf("original profile ignored: %+v images=%d", r, f.sender.images)
			}
		})
	}
}
func TestImageReplyOrchestrationLegacyExclusions(t *testing.T) {
	for _, name := range []string{"legacy", "off", "builtin", "command", "search", "attachment", "document", "model-error", "refusal", "empty", "unrelated", "hash-gate"} {
		t.Run(name, func(t *testing.T) {
			f := newReplyFixture(t)
			in := f.incoming()
			switch name {
			case "legacy":
				f.a = f.base
			case "off":
				if e := f.s.PutProfile(context.Background(), f.a.scope, storage.Profile{ID: "custom", Length: "normal", Sticker: "off"}); e != nil {
					t.Fatal(e)
				}
			case "builtin":
				if e := f.s.SelectProfile(context.Background(), f.a.scope, "warm"); e != nil {
					t.Fatal(e)
				}
			case "command":
				in.Text = "/memory list"
			case "search":
				in.Text = "/search happy"
			case "attachment":
				in.Attachment = &storage.Attachment{Kind: "image", Query: "synthetic", MediaKey: "AAAAAAAAAAAAAAAAAAAAAA=="}
			case "document":
				in.Attachment = &storage.Attachment{Kind: "file", Query: "synthetic", MediaKey: "AAAAAAAAAAAAAAAAAAAAAA==", FileName: "fixture.txt"}
			case "model-error":
				f.m.err = errors.New("sensitive fixture remote")
			case "refusal":
				f.m.reply.FinishReason = "content_filter"
			case "empty":
				f.m.reply.Text = ""
			case "unrelated":
				f.m.reply.Text = "sad"
			case "hash-gate":
				for n := 0; ; n++ {
					in.ID = fmt.Sprint("skip-", n)
					h := sha256.Sum256([]byte(fmt.Sprintf(`["account","alice",%q]`, in.ID)))
					if h[0]%8 >= 2 {
						break
					}
				}
			}
			r, e := f.a.Handle(context.Background(), in)
			if name == "attachment" || name == "document" { // A valid descriptor is mandatory even when no processor exists.
				if r.State == "rejected" {
					t.Fatal("fixture descriptor invalid")
				}
			}
			if e != nil {
				t.Fatalf("%+v %v", r, e)
			}
			if f.sender.calls != 1 || f.sender.images != 0 || len(f.order) != 1 || r.ImageState != "" {
				t.Fatalf("excluded path selected %+v order=%v", r, f.order)
			}
		})
	}
}
func TestImageReplyOrchestrationBorrowRefusals(t *testing.T) {
	for _, name := range []string{"wrong-store", "scope", "id", "digest", "disabled", "deleting", "animated", "format", "dimensions", "size", "local-digest", "local-format", "stale-row", "borrow-error", "no-callback"} {
		t.Run(name, func(t *testing.T) {
			f := newReplyFixture(t)
			f.borrowHook = func(ctx context.Context, row *storage.LibraryAsset, local *attachment.LocalImage) error {
				switch name {
				case "wrong-store":
					f.borrowStore = nil
				case "scope":
					row.Scope.User = "other"
				case "id":
					row.ID++
				case "digest":
					row.SHA256 = strings.Repeat("a", 64)
				case "disabled":
					row.Enabled = false
				case "deleting":
					row.State = "deleting"
					row.Enabled = false
				case "animated":
					local.Info.Animated = true
				case "format":
					row.Format = "gif"
				case "dimensions":
					row.Width++
				case "size":
					local.Bytes++
				case "local-digest":
					local.SHA256 = strings.Repeat("a", 64)
				case "local-format":
					local.Info.Format = "jpeg"
				case "stale-row":
					return f.s.UpdateLibraryAsset(ctx, f.a.scope, row.ID, storage.LibraryMetadata{Enabled: false})
				case "borrow-error":
					return errors.New("sensitive borrow fixture")
				}
				return nil
			}
			if name == "no-callback" {
				f.a.replyImageBorrow = func(context.Context, int64, string, func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
					return nil
				}
			}
			in := f.incoming()
			r, e := f.a.Handle(context.Background(), in)
			if e == nil || r.State != "failed" || r.ImageState != "stopped" || strings.Contains(e.Error(), "sensitive") {
				t.Fatalf("%+v %v", r, e)
			}
			rr := f.receipt(t, in)
			if rr.Preparation != "stopped" || rr.TextPhase != "stopped" || rr.ImagePhase != "stopped" || f.sender.calls != 0 || f.sender.images != 0 || f.uses(t) != 0 {
				t.Fatalf("unsafe borrow refusal %+v", rr)
			}
		})
	}
}
func TestImageReplyOrchestrationFailureAndCancellation(t *testing.T) {
	for _, stage := range []string{"borrow", "prepare", "text", "image", "cleanup"} {
		for _, canceling := range []bool{false, true} {
			t.Run(fmt.Sprint(stage, "/cancel=", canceling), func(t *testing.T) {
				f := newReplyFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				fail := func() error {
					if canceling {
						cancel()
						return nil
					}
					return errors.New("sensitive remote fixture")
				}
				switch stage {
				case "borrow":
					f.borrowHook = func(context.Context, *storage.LibraryAsset, *attachment.LocalImage) error { return fail() }
				case "prepare":
					f.prepareHook = func(context.Context) error { return fail() }
				case "text":
					f.sender.hook = func(context.Context) {
						f.order = append(f.order, "text")
						if e := fail(); e != nil {
							f.sender.err = e
						}
					}
				case "image":
					f.sender.onImage = func(context.Context) error { f.order = append(f.order, "image"); return fail() }
				case "cleanup":
					f.closeHook = func() error {
						if canceling {
							cancel()
							return nil
						}
						return errors.Join(attachment.ErrCleanup, errors.New("sensitive cleanup fixture"))
					}
				}
				in := f.incoming()
				r, e := f.a.Handle(ctx, in)
				if e == nil || r.Code == "" || strings.Contains(e.Error(), "sensitive") {
					t.Fatalf("error lost/leaked %+v %v", r, e)
				}
				rr := f.receipt(t, in)
				switch stage {
				case "borrow", "prepare":
					if r.State != "failed" || f.sender.calls != 0 || f.sender.images != 0 || rr.TextPhase != "stopped" {
						t.Fatalf("pre-send %+v %+v", r, rr)
					}
				case "text":
					if r.State != "ambiguous" || rr.TextPhase != "unknown" || f.sender.images != 0 {
						t.Fatalf("text %+v %+v", r, rr)
					}
				case "image":
					if r.State != "sent" || r.ImageState != "unknown" || rr.TextPhase != "acked" || rr.ImagePhase != "unknown" || f.uses(t) != 0 {
						t.Fatalf("image %+v %+v", r, rr)
					}
				case "cleanup":
					if r.State != "sent" || r.ImageState != "acked" || rr.TextPhase != "acked" || rr.ImagePhase != "acked" || rr.StopPhase != "" || f.uses(t) != 1 {
						t.Fatalf("cleanup %+v %+v", r, rr)
					}
				}
				if stage == "prepare" && rr.Preparation != "unknown" {
					t.Fatal("external preparation was not uncertain")
				}
				if canceling && r.Code != "canceled" {
					t.Fatalf("cancellation %+v", r)
				}
				reconstructed, e := NewWithImageReplies(f.base, f.borrow, f.prepare)
				if e != nil {
					t.Fatal(e)
				}
				if r, e = reconstructed.Handle(context.Background(), in); e != nil || r.State != "duplicate" {
					t.Fatal("failed turn replayed")
				}
			})
		}
	}
}
func TestImageReplyOrchestrationReceiptBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, sql, state, textPhase, imagePhase string
		texts, images                           int
	}{
		{"reserve", `CREATE TRIGGER deny_receipt BEFORE INSERT ON image_replies BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "failed", "", "", 0, 0},
		{"enter", `CREATE TRIGGER deny_receipt BEFORE UPDATE ON image_replies WHEN new.preparation='external' BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "failed", "stopped", "stopped", 0, 0},
		{"finish", `CREATE TRIGGER deny_receipt BEFORE UPDATE ON image_replies WHEN new.preparation='ready' BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "failed", "stopped", "stopped", 0, 0},
		{"begin-text", `CREATE TRIGGER deny_receipt BEFORE UPDATE ON messages WHEN new.state='sending' BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "failed", "stopped", "stopped", 0, 0},
		{"confirm-text", `CREATE TRIGGER deny_receipt BEFORE UPDATE ON messages WHEN new.state='sent' BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "ambiguous", "unknown", "stopped", 1, 0},
		{"confirm-text-receipt", `CREATE TRIGGER deny_receipt BEFORE UPDATE ON image_replies WHEN new.text_phase='acked' BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "ambiguous", "unknown", "stopped", 1, 0},
		{"confirm-image-receipt", `CREATE TRIGGER deny_receipt BEFORE UPDATE ON image_replies WHEN new.image_phase='acked' BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "sent", "acked", "unknown", 1, 1},
		{"begin-image", `CREATE TRIGGER deny_receipt BEFORE UPDATE ON image_replies WHEN new.image_phase='sending' BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "sent", "acked", "stopped", 1, 0},
		{"confirm-image", `CREATE TRIGGER deny_receipt BEFORE UPDATE OF uses ON library_assets BEGIN SELECT RAISE(ABORT,'synthetic'); END`, "sent", "acked", "unknown", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReplyFixture(t)
			f.sql(t, tc.sql)
			in := f.incoming()
			r, e := f.a.Handle(context.Background(), in)
			if e == nil || r.Code != "storage" || r.State != tc.state || f.sender.calls != tc.texts || f.sender.images != tc.images {
				t.Fatalf("boundary %+v %v text=%d image=%d", r, e, f.sender.calls, f.sender.images)
			}
			if tc.name == "reserve" {
				if _, e = f.s.ImageReplyReceipt(context.Background(), f.a.scope, in.ID); !errors.Is(e, storage.ErrNotFound) {
					t.Fatal("reservation failed yet receipt exists")
				}
			} else {
				rr := f.receipt(t, in)
				if rr.TextPhase != tc.textPhase || rr.ImagePhase != tc.imagePhase {
					t.Fatalf("receipt %+v", rr)
				}
			}
			if f.uses(t) != 0 {
				t.Fatal("failed ACK incremented use")
			}
		})
	}
}
func TestImageReplyOrchestrationCallbackLifetimeAndNoTransaction(t *testing.T) {
	f := newReplyFixture(t)
	in := f.incoming()
	f.prepareHook = func(ctx context.Context) error {
		if f.artifact.closed {
			t.Fatal("artifact closed during callback")
		}
		rr := f.receipt(t, in)
		if rr.Preparation != "external" || rr.TextPhase != "planned" {
			t.Fatalf("prepare before durable enter %+v", rr)
		}
		// Store has one connection: a leaked transaction would block this real write.
		bounded, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_, e := f.s.AddFact(bounded, f.a.scope, storage.Fact{Content: "synthetic callback write", Source: "explicit_user"})
		return e
	}
	f.sender.hook = func(ctx context.Context) {
		rr := f.receipt(t, in)
		if rr.TextPhase != "sending" || rr.ImagePhase != "planned" {
			t.Fatalf("text order %+v", rr)
		}
	}
	f.sender.onImage = func(ctx context.Context) error {
		rr := f.receipt(t, in)
		if rr.TextPhase != "acked" || rr.ImagePhase != "sending" || f.artifact.closed {
			t.Fatalf("image order %+v", rr)
		}
		return nil
	}
	if r, e := f.a.Handle(context.Background(), in); e != nil || r.ImageState != "acked" {
		t.Fatalf("%+v %v", r, e)
	}
	if !f.artifact.closed {
		t.Fatal("outer cleanup not joined")
	}
}
func TestImageReplyOrchestrationSnapshotBoundAndScope(t *testing.T) {
	for _, count := range []int{128, 129} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := newReplyFixture(t)
			db, e := sql.Open("sqlite", f.path)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			tx, e := db.Begin()
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback()
			for n := 1; n < count; n++ {
				_, e = tx.Exec(`INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) VALUES('other','scope',?,'png',64,2,2,1,'["happy"]','','','ready',1,0,0)`, fmt.Sprintf("%064x", n))
				if e != nil {
					t.Fatal(e)
				}
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			in := f.incoming()
			r, e := f.a.Handle(context.Background(), in)
			if count == 128 {
				if e != nil || r.ImageState != "acked" {
					t.Fatalf("bounded snapshot %+v %v", r, e)
				}
			} else {
				if e == nil || r.Code != "capacity" || f.sender.calls != 0 || f.sender.images != 0 || len(f.order) != 0 {
					t.Fatalf("partial/oversize snapshot %+v %v", r, e)
				}
			}
		})
	}
}
func TestImageReplyOrchestrationStopFailureDoesNotRetry(t *testing.T) {
	f := newReplyFixture(t)
	f.prepareHook = func(context.Context) error { return errors.New("synthetic preparation failure") }
	f.sql(t, `CREATE TRIGGER deny_stop BEFORE UPDATE ON image_replies WHEN new.stop_phase<>'' BEGIN SELECT RAISE(ABORT,'synthetic'); END`)
	in := f.incoming()
	start := time.Now()
	r, e := f.a.Handle(context.Background(), in)
	if e == nil || r.State != "failed" || f.sender.calls != 0 || f.sender.images != 0 || time.Since(start) > 3*time.Second {
		t.Fatalf("unsafe stop failure %+v %v", r, e)
	}
	rr := f.receipt(t, in)
	if rr.Preparation != "external" || rr.StopPhase != "" {
		t.Fatalf("failed stop %+v", rr)
	}
	if r, e = f.a.Handle(context.Background(), in); e != nil || r.State != "duplicate" {
		t.Fatal("stop failure retried")
	}
}

func TestImageReplyOrchestrationBorrowedContextCancellation(t *testing.T) {
	for _, stage := range []string{"prepare", "text", "image"} {
		t.Run(stage, func(t *testing.T) {
			f := newReplyFixture(t)
			work, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.workContext = work
			switch stage {
			case "prepare":
				f.prepareHook = func(ctx context.Context) error { cancel(); return ctx.Err() }
			case "text":
				f.sender.hook = func(ctx context.Context) { cancel(); f.sender.err = ctx.Err() }
			case "image":
				f.sender.onImage = func(ctx context.Context) error { cancel(); return ctx.Err() }
			}
			in := f.incoming()
			r, e := f.a.Handle(context.Background(), in)
			if e == nil || r.Code != "canceled" {
				t.Fatalf("borrowed cancellation lost: %+v %v", r, e)
			}
			rr := f.receipt(t, in)
			if rr.StopCode != "canceled" {
				t.Fatalf("wrong cancellation receipt %+v", rr)
			}
		})
	}
}

func (f *replyFixture) parentState(t *testing.T, in Incoming) string {
	t.Helper()
	db, e := sql.Open("sqlite", f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var state string
	if e = db.QueryRowContext(ctx, "SELECT state FROM messages WHERE account=? AND user=? AND id=?", in.Account, in.User, in.ID).Scan(&state); e != nil {
		t.Fatal(e)
	}
	return state
}
func TestImageReplyReservationFailureCleansReceiptlessParent(t *testing.T) {
	f := newReplyFixture(t)
	in := f.incoming()
	f.sql(t, `CREATE TRIGGER deny_receipt BEFORE INSERT ON image_replies BEGIN SELECT RAISE(ABORT,'synthetic'); END`)
	r, e := f.a.Handle(context.Background(), in)
	if e == nil || r.State != "failed" || r.Code != "storage" {
		t.Fatalf("result %+v %v", r, e)
	}
	if state := f.parentState(t, in); state != "failed" {
		t.Fatalf("receiptless parent state=%s, want failed", state)
	}
	if _, e = f.s.ImageReplyReceipt(context.Background(), f.a.scope, in.ID); !errors.Is(e, storage.ErrNotFound) {
		t.Fatalf("receipt exists %v", e)
	}
	if len(f.order) != 0 || f.sender.calls != 0 || f.sender.images != 0 {
		t.Fatal("reservation failure caused external work")
	}
	if r, e = f.a.Handle(context.Background(), in); e != nil || r.State != "duplicate" {
		t.Fatal("receiptless failure retried")
	}
}
func TestImageReplyReservationCanceledCleanup(t *testing.T) {
	f := newReplyFixture(t)
	in := f.incoming()
	if claimed, e := f.s.ClaimMessage(context.Background(), f.a.scope, in.ID, in.Text); e != nil || !claimed {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	r, e := f.a.sendImageReply(ctx, in, "happy!", provider.Usage{}, f.asset)
	if e == nil || r.State != "failed" || r.Code != "storage" || time.Since(started) > 3*time.Second {
		t.Fatalf("canceled reservation %+v %v", r, e)
	}
	if state := f.parentState(t, in); state != "failed" {
		t.Fatalf("canceled cleanup did not detach: %s", state)
	}
	if len(f.order) != 0 || f.sender.calls != 0 || f.sender.images != 0 {
		t.Fatal("canceled reservation caused external work")
	}
	if _, e = f.s.ImageReplyReceipt(context.Background(), f.a.scope, in.ID); !errors.Is(e, storage.ErrNotFound) {
		t.Fatal("canceled reservation persisted receipt")
	}
}
func TestImageReplyReservationCleanupFailureNoRetry(t *testing.T) {
	f := newReplyFixture(t)
	in := f.incoming()
	f.sql(t, `CREATE TRIGGER deny_receipt BEFORE INSERT ON image_replies BEGIN SELECT RAISE(ABORT,'synthetic'); END`)
	f.sql(t, `CREATE TRIGGER deny_failed BEFORE UPDATE ON messages WHEN new.state='failed' BEGIN SELECT RAISE(ABORT,'synthetic'); END`)
	start := time.Now()
	r, e := f.a.Handle(context.Background(), in)
	if e == nil || r.Code != "storage" || r.State != "failed" || time.Since(start) > 3*time.Second {
		t.Fatalf("failed cleanup %+v %v", r, e)
	}
	if state := f.parentState(t, in); state != "processing" {
		t.Fatalf("unexpected failed-write state %s", state)
	}
	if len(f.order) != 0 || f.sender.calls != 0 || f.sender.images != 0 {
		t.Fatal("failed cleanup caused external work")
	}
	if r, e = f.a.Handle(context.Background(), in); e != nil || r.State != "duplicate" {
		t.Fatal("failed cleanup retried")
	}
}
func TestImageReplyReservationCleanupCannotChangeSelectedReceipt(t *testing.T) {
	for _, phase := range []string{"planned", "external", "ready", "text-sending", "text-acked", "image-sending", "image-acked", "stopped"} {
		t.Run(phase, func(t *testing.T) {
			f := newReplyFixture(t)
			in := f.incoming()
			ctx := context.Background()
			if claimed, e := f.s.ClaimMessage(ctx, f.a.scope, in.ID, in.Text); e != nil || !claimed {
				t.Fatal(e)
			}
			if e := f.s.PrepareImageReply(ctx, f.a.scope, in.ID, f.asset.ID, f.asset.SHA256, stableClientID(in), imageReplyClientID(in)); e != nil {
				t.Fatal(e)
			}
			must := func(e error) {
				t.Helper()
				if e != nil {
					t.Fatal(e)
				}
			}
			if phase != "planned" && phase != "stopped" {
				must(f.s.EnterImagePreparation(ctx, f.a.scope, in.ID))
			}
			if phase != "planned" && phase != "external" && phase != "stopped" {
				must(f.s.FinishImagePreparation(ctx, f.a.scope, in.ID))
			}
			if phase == "text-sending" || phase == "text-acked" || phase == "image-sending" || phase == "image-acked" {
				must(f.s.BeginImageReplyText(ctx, f.a.scope, in.ID))
			}
			if phase == "text-acked" || phase == "image-sending" || phase == "image-acked" {
				must(f.s.ConfirmImageReplyText(ctx, f.a.scope, in.ID, "happy!"))
			}
			if phase == "image-sending" || phase == "image-acked" {
				must(f.s.BeginImageReplyImage(ctx, f.a.scope, in.ID))
			}
			if phase == "image-acked" {
				must(f.s.ConfirmImageReplyImage(ctx, f.a.scope, in.ID, time.Now().UnixMilli()))
			}
			if phase == "stopped" {
				must(f.s.StopImageReply(ctx, f.a.scope, in.ID, storage.ImageStopPreparation, storage.ImageStopRefused))
			}
			before := f.receipt(t, in)
			state := f.parentState(t, in)
			uses := f.uses(t)
			// A reservation error may coexist with a sidecar (e.g. uncertain return).
			// The generic processing->failed write must be guarded by NOT EXISTS.
			r, e := f.a.sendImageReply(ctx, in, "happy!", provider.Usage{}, f.asset)
			if e == nil || r.Code != "storage" {
				t.Fatalf("selected reservation %+v %v", r, e)
			}
			after := f.receipt(t, in)
			if !reflect.DeepEqual(before, after) || f.parentState(t, in) != state || f.uses(t) != uses {
				t.Fatalf("selected receipt changed: before=%+v after=%+v", before, after)
			}
			if len(f.order) != 0 || f.sender.calls != 0 || f.sender.images != 0 {
				t.Fatal("selected receipt replayed")
			}
		})
	}
}
