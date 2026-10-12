package pdfsafe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const latinFont = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"

func textPDF(content, font string) []byte {
	return objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", font, pdfStream(content)}, "")
}
func pdfStream(s string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
}
func wantExtract(t *testing.T, p []byte, want string, pages int) {
	t.Helper()
	got, n, e := Extract(context.Background(), bytes.NewReader(p), int64(len(p)))
	if e != nil || got != want || n != pages {
		t.Fatalf("extraction differs: text bytes=%d pages=%d error=%v", len(got), n, e)
	}
}
func wantExtractError(t *testing.T, p []byte, want error) {
	t.Helper()
	got, n, e := Extract(context.Background(), bytes.NewReader(p), int64(len(p)))
	if !errors.Is(e, want) || got != "" || n != 0 {
		t.Fatalf("failure exposed output or wrong code: text bytes=%d pages=%d error=%v", len(got), n, e)
	}
}
func TestPDFExtractionTask2Boundaries(t *testing.T) {
	t.Run("literal", func(t *testing.T) {
		wantExtract(t, textPDF("BT /F1 12 Tf (A\r\nB\\q\\777) Tj ET", latinFont), "A\nBqÿ", 1)
	})
	t.Run("quote_and_TJ", func(t *testing.T) {
		wantExtract(t, textPDF(`BT /F1 12 Tf 0 0 (one) " [(two) -20 (three)] TJ (four) ' ET`, latinFont), "onetwothree\nfour", 1)
	})
	for _, n := range []int{maxOutput, maxOutput + 1} {
		t.Run(fmt.Sprint("output", n), func(t *testing.T) {
			var s strings.Builder
			s.WriteString("BT /F1 12 Tf ")
			for left := n; left > 0; {
				part := left
				if part > 32000 {
					part = 32000
				}
				fmt.Fprintf(&s, "(%s) Tj ", strings.Repeat("a", part))
				left -= part
			}
			s.WriteString("ET")
			p := textPDF(s.String(), latinFont)
			if n == maxOutput {
				wantExtract(t, p, strings.Repeat("a", n), 1)
			} else {
				wantExtractError(t, p, ErrLimit)
			}
		})
	}
	for _, n := range []int{64, 65} {
		t.Run(fmt.Sprint("pages", n), func(t *testing.T) {
			objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", ""}
			var kids strings.Builder
			for i := 0; i < n; i++ {
				page := len(objects) + 1
				fmt.Fprintf(&kids, "%d 0 R ", page)
				objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", page+1, page+2), latinFont, pdfStream("BT /F1 12 Tf (x) Tj ET"))
			}
			objects[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", n, kids.String())
			p := objectPDF(objects, "")
			if n == 64 {
				wantExtract(t, p, strings.TrimSuffix(strings.Repeat("x\n", 64), "\n"), 64)
			} else {
				wantExtractError(t, p, ErrLimit)
			}
		})
	}
	for _, tc := range []struct {
		name, body string
		want       error
	}{{"scanned", "", ErrUnsupported}, {"trailing_operand", "BT /F1 12 Tf (prefix) Tj ET 1", ErrInvalid}, {"unknown_visible_op", "BT /F1 12 Tf (prefix) Tj ET /Fm Do", ErrInvalid}, {"PS_content", "BT /F1 12 Tf (prefix) Tj ET 0 dict", ErrUnsupported}, {"truncated_text", "BT /F1 12 Tf (prefix) Tj", ErrInvalid}} {
		t.Run(tc.name, func(t *testing.T) { wantExtractError(t, textPDF(tc.body, latinFont), tc.want) })
	}
	t.Run("unsupported_font", func(t *testing.T) {
		wantExtractError(t, textPDF("BT /F1 12 Tf (x) Tj ET", "<< /Type /Font /Subtype /Type3 >>"), ErrUnsupported)
	})
	t.Run("encrypted", func(t *testing.T) {
		p := textPDF("BT /F1 12 Tf (x) Tj ET", latinFont)
		p = bytes.Replace(p, []byte("/Root 1 0 R"), []byte("/Root 1 0 R /Encrypt << >>"), 1)
		wantExtractError(t, p, ErrUnsupported)
	})
	t.Run("input_limits", func(t *testing.T) {
		r := &countReaderAt{}
		_, _, e := Extract(context.Background(), r, maxInput+1)
		if e != ErrLimit || r.calls != 0 {
			t.Fatal("input guard read")
		}
		var nilInput *bytes.Reader
		_, _, e = Extract(context.Background(), nilInput, 20)
		if e != ErrInvalid {
			t.Fatal("typed nil accepted")
		}
		_, _, e = Extract(nil, bytes.NewReader(nil), 0)
		if e != ErrInvalid {
			t.Fatal("nil context accepted")
		}
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := &countReaderAt{}
		s, n, e := Extract(ctx, r, 20)
		if !errors.Is(e, context.Canceled) || s != "" || n != 0 || r.calls != 0 {
			t.Fatal("canceled admitted")
		}
	})
}
func TestPDFPageGraphTask2(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint("image_stream_syntax", valid), func(t *testing.T) {
			image := strings.Replace(pdfStream("synthetic-image"), "<< /Length", "<< /Subtype /Image /Length", 1)
			if !valid {
				image = strings.Replace(image, "endstream", "badstream", 1)
			}
			p := objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> /XObject << /Im 6 0 R >> >> /Contents 5 0 R >>", latinFont, pdfStream("BT /F1 12 Tf (prefix) Tj ET /Im Do"), image}, "")
			if valid {
				wantExtract(t, p, "prefix", 1)
			} else {
				wantExtractError(t, p, ErrInvalid)
			}
		})
	}
	for _, tc := range []struct{ name, tree, page string }{{"cycle", "<< /Type /Pages /Count 1 /Kids [2 0 R] >>", "<< /Type /Page >>"}, {"repeated_page", "<< /Type /Pages /Count 2 /Kids [3 0 R 3 0 R] >>", "<< /Type /Page /Parent 2 0 R >>"}, {"parent_cycle", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 3 0 R >>"}, {"count_type", "<< /Type /Pages /Count (1) /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R >>"}, {"count_negative", "<< /Type /Pages /Count -1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R >>"}, {"count_mismatch", "<< /Type /Pages /Count 0 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R >>"}} {
		t.Run(tc.name, func(t *testing.T) {
			p := objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", tc.tree, tc.page}, "")
			wantExtractError(t, p, ErrInvalid)
		})
	}
	t.Run("indirect_cycle", func(t *testing.T) {
		wantExtractError(t, objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "3 0 R", "2 0 R"}, ""), ErrInvalid)
	})
	t.Run("inherited_resource", func(t *testing.T) {
		p := objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] /Resources << /Font << /F1 4 0 R >> >> >>", "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>", latinFont, pdfStream("BT /F1 12 Tf (inherited) Tj ET")}, "")
		wantExtract(t, p, "inherited", 1)
	})
	t.Run("content_array", func(t *testing.T) {
		p := objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents [5 0 R 6 0 R] >>", latinFont, pdfStream("BT /F1 12 Tf (a) Tj "), pdfStream("(b) Tj ET")}, "")
		wantExtract(t, p, "ab", 1)
	})
}
func FuzzPDFExtractTask2Seeds(f *testing.F) {
	for _, p := range [][]byte{textPDF("BT /F1 12 Tf (ok) Tj ET", latinFont), textPDF("BT", latinFont), []byte("%PDF-1.7\n%%EOF"), nil} {
		f.Add(p)
	}
	f.Fuzz(func(t *testing.T, p []byte) {
		if len(p) > maxInput {
			return
		}
		s, n, e := Extract(context.Background(), bytes.NewReader(p), int64(len(p)))
		if e != nil && (s != "" || n != 0) {
			t.Fatal("partial failure")
		}
		if e == nil && (len(s) > maxOutput || n > maxPages) {
			t.Fatal("unbounded success")
		}
	})
}

func TestPDFLiteralCompatibilityTask2(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"(a\rb\r\nc\nd)", "a\nb\nc\nd"}, {`(\q\777\1\12\123)`, "q\xff\x01\nS"}, {"(a\\\r\nb)", "ab"}} {
		t.Run(tc.in, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("valid literal refused: %T", p)
				}
			}()
			b := newBuffer(strings.NewReader(tc.in), 0, newBudget(context.Background()))
			b.allowEOF = true
			if got := b.readObject(); got != tc.want {
				t.Errorf("literal semantics differ")
			}
		})
	}
}

func TestPDFParsedStreamClosingTask2(t *testing.T) {
	t.Run("missing_EOL", func(t *testing.T) {
		r := mustParse(t, objectPDF([]string{"<< /Length 3 >>\nstream\nabcendstream"}, ""))
		mustFault(t, ErrInvalid, func() { r.trailerValue().key("Root").reader() })
	})
	t.Run("included_EOL", func(t *testing.T) {
		r := mustParse(t, objectPDF([]string{"<< /Length 4 >>\nstream\nabc\nendstream"}, ""))
		s := r.trailerValue().key("Root").reader()
		_ = s.Close()
	})
	t.Run("indirect_length", func(t *testing.T) {
		r := mustParse(t, objectPDF([]string{"<< /Length 2 0 R >>\nstream\nabc\nendstream", "3"}, ""))
		s := r.trailerValue().key("Root").reader()
		_ = s.Close()
	})
	for _, end := range []string{"endstream", "badstream", "endstream junk"} {
		t.Run(end, func(t *testing.T) {
			p := objectPDF([]string{"<< /Length 3 >>\nstream\nabc\n" + end}, "")
			r := mustParse(t, p)
			if end == "endstream" {
				v := r.trailerValue().key("Root")
				s := v.reader()
				var b [4]byte
				_, _ = s.Read(b[:])
				_ = s.Close()
				return
			}
			mustFault(t, errMalformed, func() {
				v := r.trailerValue().key("Root")
				s := v.reader()
				var b [4]byte
				_, _ = s.Read(b[:])
				_ = s.Close()
			})
		})
	}
	// Missing endobj is an actual parsed object, with an intact xref table.
	p := objectPDF([]string{"<< /Length 3 >>\nstream\nabc\nendstream"}, "")
	p = bytes.Replace(p, []byte("endobj"), []byte("badobj"), 1)
	r := mustParse(t, p)
	mustFault(t, errMalformed, func() { r.trailerValue().key("Root").reader() })
}
