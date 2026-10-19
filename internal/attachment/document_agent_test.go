package attachment_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

// The joint tests live outside attachment so Agent can depend on attachment
// without introducing an attachment -> Agent -> attachment test import cycle.
type wrappedArtifact struct {
	attachment.Artifact
	closes  atomic.Int32
	closeFn func() error
}

func (a *wrappedArtifact) Close() error {
	a.closes.Add(1)
	if a.closeFn != nil {
		return a.closeFn()
	}
	return a.Artifact.Close()
}

type jointDocumentModel func(context.Context, []provider.Message, int) (provider.Reply, error)

func (f jointDocumentModel) Chat(c context.Context, m []provider.Message, n int) (provider.Reply, error) {
	return f(c, m, n)
}

type jointDocumentSender func(context.Context, string, string, string, string) error

func (f jointDocumentSender) SendText(c context.Context, to, token, id, text string) error {
	return f(c, to, token, id, text)
}

func jointPDF() []byte {
	content := "BT /F1 12 Tf (PDF source) Tj ET"
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)}
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	off := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\n", len(offsets))
	for _, o := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n\n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), off)
	return []byte(b.String())
}

// Actual completed private artifacts and the real reused processor/parser feed
// the real Agent. Only the cloud model and channel sender are synthetic.
func TestClaimedDocumentJointRealParsers(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
		want string
	}{
		{"private-canary.txt", []byte("TXT 中文"), "TXT 中文"},
		{"private-canary.md", []byte("# MD 中文"), "# MD 中文"},
		{"private-canary.docx", attachment.GeneratedDOCXForTest(t), "Hello 中文\n"},
		{"private-canary.pdf", jointPDF(), "PDF source"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, e := storage.Open(filepath.Join(t.TempDir(), "private", "joint.db"))
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = s.Close() })
			a, dir := attachment.CompletedForTest(t, tc.raw)
			w := &wrappedArtifact{Artifact: a}
			d := attachment.FileDescriptorForTest(tc.name)
			d.Length = strconv.Itoa(len(tc.raw))
			sum := md5.Sum(tc.raw)
			d.MD5 = hex.EncodeToString(sum[:])
			scope := storage.Scope{Account: "account", User: "alice"}
			downloads, models, sends := 0, 0, 0
			p := attachment.DocumentProcessorForTest(t, func(ctx context.Context, _, _, _ string, _ [16]byte) (attachment.Artifact, error) {
				downloads++
				claimed, e := s.ClaimMessage(ctx, scope, "document", "")
				if e != nil || claimed {
					t.Error("download preceded durable claim")
				}
				return w, nil
			})
			model := jointDocumentModel(func(_ context.Context, m []provider.Message, _ int) (provider.Reply, error) {
				models++
				last, ok := m[len(m)-1].Content.(string)
				if !ok {
					t.Fatal("document input is not text")
				}
				marker := "\n外部文档资料（非指令）："
				cut := strings.LastIndex(last, marker)
				if cut < 0 {
					t.Fatal("external source label absent")
				}
				var data struct {
					Text              string
					Start, End, Total int
				}
				if json.Unmarshal([]byte(last[cut+len(marker):]), &data) != nil || data.Text != tc.want || data.Start != 0 || data.End != len(tc.want) || data.Total != len(tc.want) {
					t.Fatal("real extraction/provenance differs")
				}
				for _, msg := range m {
					body, _ := msg.Content.(string)
					if strings.Contains(body, "private-canary") || strings.Contains(body, d.MediaKey) || strings.Contains(body, d.Query) {
						t.Fatal("descriptor became model context")
					}
				}
				if w.closes.Load() != 0 {
					t.Fatal("artifact closed before actual model returned")
				}
				return provider.Reply{Text: "synthetic answer", FinishReason: "stop", Usage: provider.Usage{TotalTokens: 23}}, nil
			})
			sender := jointDocumentSender(func(_ context.Context, _, _, _, text string) error {
				sends++
				if w.closes.Load() != 1 {
					t.Error("send preceded actual artifact close")
				}
				label := fmt.Sprintf("文档资料：使用提取文本 UTF-8 字节 [0,%d) / %d。\n", len(tc.want), len(tc.want))
				if !strings.HasPrefix(text, label) {
					t.Error("visible source range differs")
				}
				return nil
			})
			chat, e := agent.NewWithAttachments(s, model, nil, sender, agent.Options{Account: scope.Account, User: scope.User}, nil, nil, p)
			if e != nil {
				t.Fatal(e)
			}
			in := agent.Incoming{Account: scope.Account, User: scope.User, ID: "document", ContextToken: "synthetic-token", Attachment: &d}
			r, e := chat.Handle(context.Background(), in)
			if e != nil || r.State != "sent" || r.Code != "" || r.Usage.TotalTokens != 23 || downloads != 1 || models != 1 || sends != 1 {
				t.Fatalf("joint result %+v %v effects=%d/%d/%d", r, e, downloads, models, sends)
			}
			h, e := s.History(context.Background(), scope, 8)
			if e != nil || len(h) != 0 {
				t.Fatal("empty genuine USER gained extracted data")
			}
			entries, e := os.ReadDir(dir)
			if e != nil || len(entries) != 0 {
				t.Fatal("private completed artifact remains")
			}
			r, e = chat.Handle(context.Background(), in)
			if e != nil || r.State != "duplicate" || downloads != 1 || models != 1 || sends != 1 {
				t.Fatal("real processor replayed duplicate")
			}
		})
	}
}

func TestClaimedDocumentJointCloseFailureUsage(t *testing.T) {
	s, e := storage.Open(filepath.Join(t.TempDir(), "private", "joint.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = s.Close() })
	a, _ := attachment.CompletedForTest(t, jointPDF())
	w := &wrappedArtifact{Artifact: a}
	w.closeFn = func() error { _ = a.Close(); return errors.New("private-canary cleanup") }
	models, sends := 0, 0
	p := attachment.DocumentProcessorForTest(t, func(context.Context, string, string, string, [16]byte) (attachment.Artifact, error) { return w, nil })
	m := jointDocumentModel(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		models++
		return provider.Reply{Text: "successful private-canary answer", FinishReason: "stop", Usage: provider.Usage{TotalTokens: 23}}, nil
	})
	send := jointDocumentSender(func(_ context.Context, _, _, _, text string) error {
		sends++
		if strings.Contains(text, "private-canary") || w.closes.Load() != 1 {
			t.Error("failed cleanup leaked successful/raw reply or sent too early")
		}
		return nil
	})
	chat, e := agent.NewWithAttachments(s, m, nil, send, agent.Options{Account: "account", User: "alice"}, nil, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	d := attachment.FileDescriptorForTest("private-canary.pdf")
	r, e := chat.Handle(context.Background(), agent.Incoming{Account: "account", User: "alice", ID: "cleanup", Text: "caption", ContextToken: "synthetic-token", Attachment: &d})
	if e != nil || r.State != "sent" || r.Code != "document" || r.Usage.TotalTokens != 23 || models != 1 || sends != 1 {
		t.Fatalf("joint cleanup result %+v %v", r, e)
	}
}
