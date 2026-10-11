package pdfsafe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func tablePDF(extra string) []byte { return objectPDF([]string{"<< /Type /Catalog >>"}, extra) }
func objectPDF(objects []string, extra string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offs := []int{0}
	for i, o := range objects {
		offs = append(offs, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	off := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\n", len(offs))
	for _, o := range offs[1:] {
		fmt.Fprintf(&b, "%010d 00000 n\n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R %s >>\nstartxref\n%d\n%%%%EOF\n", len(offs), extra, off)
	return []byte(b.String())
}
func mustFault(t *testing.T, want error, fn func()) {
	t.Helper()
	defer func() {
		p := recover()
		f, ok := p.(fault)
		if !ok || !errors.Is(f.err, want) {
			t.Errorf("expected safe %v fault, got %T", want, p)
		}
	}()
	fn()
}
func mustParse(t *testing.T, p []byte) *reader {
	t.Helper()
	r, e := parse(context.Background(), bytes.NewReader(p), int64(len(p)))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := r.b.closeStreams(); e != nil {
			t.Error(e)
		}
	})
	return r
}

type countReaderAt struct {
	calls int
	data  []byte
}

func (r *countReaderAt) ReadAt(p []byte, o int64) (int, error) {
	r.calls++
	return bytes.NewReader(r.data).ReadAt(p, o)
}
func TestPDFSizeWidthAndPredictorBeforeAllocation(t *testing.T) {
	t.Run("input_before_read", func(t *testing.T) {
		in := &countReaderAt{}
		_, e := parse(context.Background(), in, maxInput+1)
		if e != errLimit || in.calls != 0 {
			t.Fatal("input admission occurred after read")
		}
	})
	t.Run("size", func(t *testing.T) {
		p := tablePDF("")
		p = bytes.Replace(p, []byte("/Size 2"), []byte("/Size 32769"), 1)
		_, e := parse(context.Background(), bytes.NewReader(p), int64(len(p)))
		if e != errLimit {
			t.Fatal("missing declared size limit")
		}
	})
	t.Run("width", func(t *testing.T) {
		r := &reader{b: newBudget(context.Background()), end: 100}
		s := stream{hdr: dict{"W": array{int64(9), int64(1), int64(1)}}}
		mustFault(t, errMalformed, func() { r.readXrefStreamData(s, nil, 1) })
		if len(r.b.streams) != 0 || r.b.decoded != 0 {
			t.Fatal("width admitted decoder")
		}
	})
	t.Run("predictor", func(t *testing.T) {
		r := &reader{b: newBudget(context.Background())}
		p := value{r: r, data: dict{"Predictor": int64(12), "Columns": int64(4096)}}
		ch := &streamReader{b: r.b}
		mustFault(t, errLimit, func() { applyFilter(bytes.NewReader(nil), "FlateDecode", p, ch) })
		if len(ch.closers) != 0 {
			t.Fatal("predictor checked after decoder allocation")
		}
	})
	t.Run("table_sparse", func(t *testing.T) {
		p := []byte("%PDF-1.7\nxref\n32768 1\n0 0 n\ntrailer\n<< /Size 1 >>\nstartxref\n9\n%%EOF\n")
		_, e := parse(context.Background(), bytes.NewReader(p), int64(len(p)))
		if e != errLimit {
			t.Fatal("unchecked sparse table")
		}
	})
}
func TestPDFPrevExtendsIndirectCycles(t *testing.T) {
	t.Run("Prev", func(t *testing.T) {
		p := tablePDF("/Prev 45")
		off := bytes.Index(p, []byte("xref\n"))
		p = tablePDF(fmt.Sprintf("/Prev %d", off))
		_, e := parse(context.Background(), bytes.NewReader(p), int64(len(p)))
		if e != errMalformed {
			t.Fatal("Prev cycle accepted")
		}
	})
	t.Run("indirect", func(t *testing.T) {
		r := mustParse(t, objectPDF([]string{"2 0 R", "1 0 R"}, ""))
		mustFault(t, errMalformed, func() { r.trailerValue().key("Root") })
		if len(r.b.objects) != 0 || r.b.depth != 0 {
			t.Fatal("active resolution state leaked")
		}
	})
	t.Run("Extends", func(t *testing.T) {
		r := mustParse(t, xrefStreamPDF(false, true))
		mustFault(t, errMalformed, func() { r.trailerValue().key("Root") })
		if len(r.b.streams) != 0 {
			t.Fatal("Extends stream leaked")
		}
	})
}
func xrefStreamPDF(flate, extends bool) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	objoff := b.Len()
	body := []byte("1 0 << /Type /Catalog >>")
	ext := ""
	if extends {
		body = []byte("3 0 << /Type /Catalog >>")
		ext = " /Extends 2 0 R"
	}
	fmt.Fprintf(&b, "2 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length %d%s >>\nstream\n", len(body), ext)
	b.Write(body)
	b.WriteString("\nendstream\nendobj\n")
	xoff := b.Len()
	data := []byte{0, 0, 0, 255, 2, 0, 2, 0, 1, byte(objoff >> 8), byte(objoff), 0, 1, byte(xoff >> 8), byte(xoff), 0}
	filter := ""
	if flate {
		var z bytes.Buffer
		writeZlib(&z, data)
		data = z.Bytes()
		filter = " /Filter /FlateDecode"
	}
	fmt.Fprintf(&b, "3 0 obj\n<< /Type /XRef /Size 4 /W [1 2 1] /Root 1 0 R /Length %d%s >>\nstream\n", len(data), filter)
	b.Write(data)
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xoff)
	return b.Bytes()
}
func TestPDFValidCoreFixtures(t *testing.T) {
	for _, flate := range []bool{false, true} {
		t.Run(fmt.Sprintf("xref_object_stream_flate_%t", flate), func(t *testing.T) {
			r := mustParse(t, xrefStreamPDF(flate, false))
			v := r.trailerValue().key("Root")
			if v.kind() != dictKind || v.key("Type").name() != "Catalog" {
				t.Fatal("wrong resolved catalog")
			}
			if len(r.b.streams) != 0 {
				t.Fatal("object stream remains open")
			}
		})
	}
	t.Run("table", func(t *testing.T) {
		r := mustParse(t, tablePDF(""))
		if r.trailerValue().key("Root").key("Type").name() != "Catalog" {
			t.Fatal("catalog mismatch")
		}
	})
	t.Run("encrypted", func(t *testing.T) {
		p := tablePDF("/Encrypt 1 0 R")
		_, e := parse(context.Background(), bytes.NewReader(p), int64(len(p)))
		if e != errUnsupported {
			t.Fatal("encrypted PDF was not refused")
		}
	})
}
func TestPDFSharedCountersAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cancel()
	p := tablePDF("")
	in := &countReaderAt{data: p}
	_, e := parse(ctx, in, int64(len(p)))
	if !errors.Is(e, context.Canceled) || in.calls != 0 {
		t.Fatal("cancel ignored before reads")
	}
	r := mustParse(t, p)
	r.b.work = maxWork
	mustFault(t, errLimit, func() { r.trailerValue().key("Root") })
	r = mustParse(t, p)
	r.b.retained = maxRetained
	mustFault(t, errLimit, func() { r.trailerValue().key("Root") })
	r = mustParse(t, p)
	r.b.nodes = maxNodes
	mustFault(t, errLimit, func() { r.trailerValue().key("Root") })
}
func FuzzPDFCoreSeeds(f *testing.F) {
	f.Add(tablePDF(""))
	f.Add(xrefStreamPDF(true, false))
	f.Add([]byte("%PDF-1.7\ninvalid"))
	f.Fuzz(func(t *testing.T, p []byte) {
		if len(p) > 65536 {
			return
		}
		r, _ := parse(context.Background(), bytes.NewReader(p), int64(len(p)))
		if r != nil {
			_ = r.b.closeStreams()
		}
	})
}

var _ = io.EOF

func TestPDFIncrementalAndObjectReferences(t *testing.T) {
	t.Run("older_smaller_Size", func(t *testing.T) {
		p := tablePDF("")
		old := bytes.Index(p, []byte("xref\n"))
		var b bytes.Buffer
		b.Write(p)
		o := b.Len()
		b.WriteString("2 0 obj\n42\nendobj\n")
		x := b.Len()
		fmt.Fprintf(&b, "xref\n2 1\n%010d 00000 n\ntrailer\n<< /Size 3 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", o, old, x)
		r := mustParse(t, b.Bytes())
		if r.trailerValue().key("Root").key("Type").name() != "Catalog" || r.b.revisions != 2 {
			t.Fatal("valid incremental xref failed")
		}
	})
	t.Run("object_stream_reference", func(t *testing.T) {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("valid object-stream reference rejected by parser fault (%T)", p)
			}
		}()
		r := mustParse(t, xrefStreamBody([]byte("1 0 3 0 R")))
		if r.trailerValue().key("Root").key("Type").name() != "XRef" {
			t.Fatal("object stream indirect value not resolved")
		}
	})
}
func xrefStreamBody(body []byte) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	o := b.Len()
	fmt.Fprintf(&b, "2 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length %d >>\nstream\n", len(body))
	b.Write(body)
	b.WriteString("\nendstream\nendobj\n")
	x := b.Len()
	data := []byte{0, 0, 0, 255, 2, 0, 2, 0, 1, byte(o >> 8), byte(o), 0, 1, byte(x >> 8), byte(x), 0}
	fmt.Fprintf(&b, "3 0 obj\n<< /Type /XRef /Size 4 /W [1 2 1] /Root 1 0 R /Length %d >>\nstream\n", len(data))
	b.Write(data)
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", x)
	return b.Bytes()
}

func TestPDFXrefRevisionLimitAndSafeErrors(t *testing.T) {
	t.Run("revisions", func(t *testing.T) {
		var b bytes.Buffer
		b.WriteString("%PDF-1.7\n")
		prev := -1
		last := 0
		for i := 0; i < 33; i++ {
			last = b.Len()
			fmt.Fprintf(&b, "xref\n0 1\n0000000000 65535 f\ntrailer\n<< /Size 1")
			if prev >= 0 {
				fmt.Fprintf(&b, " /Prev %d", prev)
			}
			b.WriteString(" >>\n")
			prev = last
		}
		fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", last)
		_, e := parse(context.Background(), bytes.NewReader(b.Bytes()), int64(b.Len()))
		if e != errLimit {
			t.Fatal("xref revisions reset or unbounded")
		}
	})
	t.Run("negative_offset", func(t *testing.T) {
		p := tablePDF("/Prev -1")
		_, e := parse(context.Background(), bytes.NewReader(p), int64(len(p)))
		if e != errMalformed {
			t.Fatal("negative offset accepted")
		}
	})
	t.Run("input_error", func(t *testing.T) {
		_, e := parse(context.Background(), failingReaderAt{}, 100)
		if e != errMalformed || strings.Contains(e.Error(), "SYNTHETIC_SECRET") {
			t.Fatal("raw input error leaked")
		}
	})
	t.Run("expired_deadline", func(t *testing.T) {
		r := mustParse(t, tablePDF(""))
		r.b.deadline = time.Time{}
		mustFault(t, context.DeadlineExceeded, func() { r.trailerValue().key("Root") })
	})
}

type failingReaderAt struct{}

func (failingReaderAt) ReadAt([]byte, int64) (int, error) { return 0, errors.New("SYNTHETIC_SECRET") }

// Consistent fixture: compressed objects1 and4, ObjStm2, XRef3.
func fixObjectStreamPDF(body []byte, n, first int, flate, corrupt bool) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	o := b.Len()
	filter := ""
	if flate {
		var z bytes.Buffer
		writeZlib(&z, body)
		body = append([]byte(nil), z.Bytes()...)
		if corrupt {
			body[len(body)-1] ^= 1
		}
		filter = " /Filter /FlateDecode"
	}
	fmt.Fprintf(&b, "2 0 obj\n<< /Type /ObjStm /N %d /First %d /Length %d%s >>\nstream\n", n, first, len(body), filter)
	b.Write(body)
	b.WriteString("\nendstream\nendobj\n")
	x := b.Len()
	fourth := []byte{0, 0, 0, 255}
	if n > 1 {
		fourth = []byte{2, 0, 2, 1}
	}
	data := []byte{0, 0, 0, 255, 2, 0, 2, 0, 1, byte(o >> 8), byte(o), 0, 1, byte(x >> 8), byte(x), 0}
	data = append(data, fourth...)
	fmt.Fprintf(&b, "3 0 obj\n<< /Type /XRef /Size 5 /W [1 2 1] /Root 1 0 R /Length %d >>\nstream\n", len(data))
	b.Write(data)
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", x)
	return b.Bytes()
}
func TestPDFObjectStreamSpanIsolation(t *testing.T) {
	for _, c := range []struct {
		name, body string
		n, first   int
	}{
		{"cross_neighbor", "1 0 4 1 (CROSS)", 2, 8},
		{"overlap", "1 0 4 6 (first)(second)", 2, 8},
		{"equal_offsets", "1 0 4 0 (first)(second)", 2, 8},
		{"out_of_order", "1 7 4 0 (first)(second)", 2, 8},
		{"extra_object", "1 0 (first) (extra)", 1, 4},
		{"stream_forbidden", "1 0 << /Length 0 >>\nstream\n\nendstream", 1, 4},
		{"duplicate_ids", "1 0 1 7 (first)(second)", 2, 8},
		{"header_truncated", "1 0 (first)", 2, 4},
		{"First_beyond_data", "1 0 (first)", 1, 100},
		{"stray_delimiter", "1 0 ]", 1, 4},
		{"other_index_mismatch", "1 0 4 7 (first)(second)", 2, 8},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := mustParse(t, fixObjectStreamPDF([]byte(c.body), c.n, c.first, false, false))
			if c.name == "other_index_mismatch" {
				r.xref[4].offset = 0
			}
			mustFault(t, errMalformed, func() { r.trailerValue().key("Root") })
			if len(r.b.streams) != 0 || len(r.b.objects) != 0 {
				t.Fatal("failed object span leaked active state")
			}
		})
	}
}

type fixCallbackReaderAt struct {
	source io.ReaderAt
	after  func()
}

func (s fixCallbackReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := s.source.ReadAt(p, off)
	s.after()
	return n, err
}
func TestPDFObjectStreamDrainSharedBudget(t *testing.T) {
	for _, mode := range []string{"retained", "decoded", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			p := fixObjectStreamPDF([]byte("1 0 (prefix)"+strings.Repeat(" ", 5000)), 1, 4, true, false)
			r, err := parse(ctx, bytes.NewReader(p), int64(len(p)))
			if err != nil {
				t.Fatal("fixture rejected")
			}
			strm := r.resolve(objptr{}, objptr{2, 0})
			want := errLimit
			switch mode {
			case "retained":
				r.b.retained = maxRetained - 4096 - 64 - 65536 - 16
			case "decoded":
				r.b.decoded = maxDecoded - 32
			case "cancel":
				want = context.Canceled
				r.f = &checkedReaderAt{r: fixCallbackReaderAt{r.f, cancel}, b: r.b, size: r.end}
			}
			mustFault(t, want, func() { r.objectStream(strm, objptr{1, 0}, 0) })
			if len(r.b.streams) != 0 {
				t.Fatal("drain failure leaked actual decoder")
			}
		})
	}
}
func TestPDFObjectStreamLiteralCorruptTail(t *testing.T) {
	p := fixObjectStreamPDF([]byte("1 0 (prefix)"+strings.Repeat(" ", 5000)), 1, 4, true, true)
	r := mustParse(t, p)
	mustFault(t, errMalformed, func() { r.trailerValue().key("Root") })
	if len(r.b.streams) != 0 {
		t.Fatal("corrupt decoder tail leaked actual stream")
	}
}
func TestPDFObjectStreamMultipleValid(t *testing.T) {
	for _, flate := range []bool{false, true} {
		t.Run(fmt.Sprintf("flate_%t", flate), func(t *testing.T) {
			r := mustParse(t, fixObjectStreamPDF([]byte("1 0 4 7 (first)(second)"), 2, 8, flate, false))
			if r.trailerValue().key("Root").rawString() != "first" || r.resolve(objptr{}, objptr{4, 0}).rawString() != "second" {
				t.Fatal("multiple object spans changed")
			}
			if len(r.b.streams) != 0 {
				t.Fatal("valid object stream remains open")
			}
		})
	}
}
