package document

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

const testWordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const testStrictNS = "http://purl.oclc.org/ooxml/wordprocessingml/main"
const testOfficeRel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
const testStrictRel = "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument"
const testContent = `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`

func testRelationships(rel string) string {
	return `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="` + rel + `" Target="word/document.xml"/></Relationships>`
}
func testMain(ns, body string) string {
	return `<w:document xmlns:w="` + ns + `"><w:body>` + body + `</w:body></w:document>`
}
func testDOCX(t *testing.T, ct, rel, main string, extra map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, p := range []struct{ name, body string }{{"[Content_Types].xml", ct}, {"_rels/.rels", rel}, {"word/document.xml", main}} {
		w, e := z.Create(p.name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(p.body)); e != nil {
			t.Fatal(e)
		}
	}
	for name, body := range extra {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(body)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func runDOCX(t *testing.T, ct, rel, main string, extra map[string]string) (Result, error) {
	t.Helper()
	b := testDOCX(t, ct, rel, main, extra)
	return New().Extract(context.Background(), DOCX, bytes.NewReader(b), int64(len(b)))
}
func TestDOCXValidLatinChineseAndPages(t *testing.T) {
	for _, pair := range [][2]string{{testWordNS, testOfficeRel}, {testStrictNS, testStrictRel}} {
		got, e := runDOCX(t, testContent, testRelationships(pair[1]), testMain(pair[0], `<w:p><w:r><w:t xml:space="preserve"> Hi 中文 &amp; &#x1F600;</w:t><w:tab/><w:t>B</w:t><w:br/><w:br w:type="page"/></w:r></w:p><w:p><w:r><w:t>C</w:t></w:r></w:p>`), nil)
		if e != nil || got != (Result{Text: " Hi 中文 & 😀\tB\n\n\nC\n", Pages: 2, Format: DOCX}) {
			t.Fatalf("main-story semantics: %#v %v", got, e)
		}
	}
}
func TestDOCXNoEntitiesOrRemoteRelationships(t *testing.T) {
	cases := []struct {
		name, ct, rel, main string
		want                error
	}{
		{"DTD", testContent, testRelationships(testOfficeRel), `<!DOCTYPE document [<!ENTITY x "private">]>` + testMain(testWordNS, `<w:p><w:r><w:t>&x;</w:t></w:r></w:p>`), ErrUnsupported},
		{"external", testContent, strings.Replace(testRelationships(testOfficeRel), `Target="word/document.xml"`, `Target="https://example.invalid/private" TargetMode="External"`, 1), testMain(testWordNS, `<w:p><w:r><w:t>x</w:t></w:r></w:p>`), ErrUnsupported},
		{"foreign", testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<x:t xmlns:x="urn:foreign">x</x:t>`), ErrInvalid},
		{"alternate", testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"/>`), ErrUnsupported},
		{"mixed", testContent, testRelationships(testStrictRel), testMain(testWordNS, `<w:p><w:r><w:t>x</w:t></w:r></w:p>`), ErrInvalid},
		{"duplicateoverride", strings.Replace(testContent, `</Types>`, `<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`, 1), testRelationships(testOfficeRel), testMain(testWordNS, `<w:p><w:r><w:t>x</w:t></w:r></w:p>`), ErrInvalid},
		{"empty", testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<w:p><w:r><w:t>  </w:t></w:r></w:p>`), ErrEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, e := runDOCX(t, tc.ct, tc.rel, tc.main, nil)
			if !errors.Is(e, tc.want) || got != (Result{}) {
				t.Fatalf("got %#v %v want %v", got, e, tc.want)
			}
		})
	}
}
func TestDOCXAggregateAndOutputBounds(t *testing.T) {
	for _, n := range []int{maxOutputBytes, maxOutputBytes + 1} {
		body := `<w:p><w:r>`
		remaining := n - 1
		for remaining > 0 {
			k := remaining
			if k > 32000 {
				k = 32000
			}
			body += `<w:t>` + strings.Repeat("a", k) + `</w:t>`
			remaining -= k
		}
		body += `</w:r></w:p>`
		got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, body), nil)
		if n == maxOutputBytes {
			if e != nil || len(got.Text) != n {
				t.Fatalf("exact output: %v %d", e, len(got.Text))
			}
		} else if !errors.Is(e, ErrLimit) || got != (Result{}) {
			t.Fatalf("output prefix escaped: %v", e)
		}
	}
	for _, pages := range []int{64, 65} {
		body := `<w:p><w:r><w:t>x</w:t>` + strings.Repeat(`<w:br w:type="page"/>`, pages-1) + `</w:r></w:p>`
		got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, body), nil)
		if pages == 64 {
			if e != nil || got.Pages != 64 {
				t.Fatalf("64 pages: %v", e)
			}
		} else if !errors.Is(e, ErrLimit) || got != (Result{}) {
			t.Fatalf("65 pages: %v", e)
		}
	}
}

func TestDOCXStructureAndMetadata(t *testing.T) {
	base := `<w:p><w:r><w:t>ok</w:t></w:r></w:p>`
	for _, tc := range []struct {
		name, ct, rel, body string
		want                error
	}{
		{"ambiguousmain", strings.Replace(testContent, `</Types>`, `<Override PartName="/other.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`, 1), testRelationships(testOfficeRel), base, ErrUnsupported},
		{"aliasedduplicate", testContent, testRelationships(testOfficeRel), `<w:p><w:r><w:t>x</w:t><w:br xmlns:q="` + testWordNS + `" w:type="page" q:type="page"/></w:r></w:p>`, ErrInvalid},
		{"nestedparagraph", testContent, testRelationships(testOfficeRel), `<w:p>` + base + `</w:p>`, ErrInvalid},
		{"tableparagraph", testContent, testRelationships(testOfficeRel), `<w:tbl>` + base + `</w:tbl>`, ErrInvalid},
		{"tablecell", testContent, testRelationships(testOfficeRel), `<w:tc>` + base + `</w:tc>`, ErrInvalid},
		{"nestedtext", testContent, testRelationships(testOfficeRel), `<w:p><w:r><w:t>x<w:unknown/></w:t></w:r></w:p>`, ErrInvalid},
		{"field", testContent, testRelationships(testOfficeRel), `<w:p><w:r><w:fldChar/><w:t>x</w:t></w:r></w:p>`, ErrUnsupported},
		{"badrelid", testContent, strings.Replace(testRelationships(testOfficeRel), `rId1`, ` bad `, 1), base, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, e := runDOCX(t, tc.ct, tc.rel, testMain(testWordNS, tc.body), nil)
			if got != (Result{}) || !errors.Is(e, tc.want) {
				t.Fatalf("%#v %v want %v", got, e, tc.want)
			}
		})
	}
	got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<w:p><w:r><w:drawing>`+base+`</w:drawing><w:t>ok</w:t></w:r></w:p><x:unknown xmlns:x="urn:foreign">`+base+`</x:unknown>`), nil)
	if e != nil || got.Text != "ok\n" {
		t.Fatalf("ignored subtree leaked into main story: %#v %v", got, e)
	}
}

func TestDOCXAllMembersAndAggregate(t *testing.T) {
	main := testMain(testWordNS, `<w:p><w:r><w:t>x</w:t></w:r></w:p>`)
	for _, over := range []int{0, 1} {
		n := maxZIPDecoded - len(testContent) - len(testRelationships(testOfficeRel)) - len(main) + over
		b := testDOCX(t, testContent, testRelationships(testOfficeRel), main, map[string]string{"word/ignored.bin": strings.Repeat("z", n)})
		got, e := New().Extract(context.Background(), DOCX, bytes.NewReader(b), int64(len(b)))
		if over == 0 {
			if e != nil || got.Text != "x\n" {
				t.Fatalf("aggregate exact: %v", e)
			}
		} else if !errors.Is(e, ErrLimit) || got != (Result{}) {
			t.Fatalf("aggregate over: %#v %v", got, e)
		}
	}
	b := testDOCX(t, testContent, testRelationships(testOfficeRel), main, map[string]string{"word/ignored.bin": "unselected-integrity-canary"})
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range z.File {
		if f.Name == "word/ignored.bin" {
			off, e := f.DataOffset()
			if e != nil {
				t.Fatal(e)
			}
			b[off] ^= 0xff
		}
	}
	got, e := New().Extract(context.Background(), DOCX, bytes.NewReader(b), int64(len(b)))
	if e == nil || got != (Result{}) {
		t.Fatalf("unselected corruption accepted: %#v %v", got, e)
	}
}

func TestDOCXUTF8OutputByteBounds(t *testing.T) {
	for _, over := range []int{0, 1} {
		body := `<w:p><w:r>`
		for i := 0; i < 4; i++ {
			body += `<w:t>` + strings.Repeat("中", 10000) + `</w:t>`
		}
		body += `<w:t>` + strings.Repeat("中", 3690) + strings.Repeat("a", 1+over) + `</w:t></w:r></w:p>`
		got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, body), nil)
		if over == 0 {
			if e != nil || len(got.Text) != maxOutputBytes {
				t.Fatalf("UTF8 exact: %v %d", e, len(got.Text))
			}
		} else if !errors.Is(e, ErrLimit) || got != (Result{}) {
			t.Fatalf("UTF8 overflow: %#v %v", got, e)
		}
	}
}

func TestDOCXMainStoryContainers(t *testing.T) {
	body := `<w:tbl><w:tblPr/><w:tr><w:tc><w:tcPr/><w:p><w:pPr/><w:hyperlink><w:r><w:rPr/><w:t>表格</w:t></w:r></w:hyperlink></w:p></w:tc></w:tr></w:tbl><w:sdt><w:sdtPr/><w:sdtContent><w:p><w:ins><w:r><w:t>inserted</w:t></w:r></w:ins></w:p></w:sdtContent></w:sdt>`
	got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, body), nil)
	if e != nil || got.Text != "表格\ninserted\n" || got.Pages != 1 {
		t.Fatalf("supported containers: %#v %v", got, e)
	}
}

// Treating an unrecognized WordML wrapper as an excluded subtree would
// return a successful prefix/suffix while silently dropping main-story text.
func TestDOCXUnsupportedWordStoryWrappers(t *testing.T) {
	for _, pair := range [][2]string{{testWordNS, testOfficeRel}, {testStrictNS, testStrictRel}} {
		for _, name := range []string{"customXml", "moveTo", "unknownStoryBlock", "cr"} {
			for _, position := range []string{"block", "inline"} {
				t.Run(pair[0]+"/"+name+"/"+position, func(t *testing.T) {
					prefix := `<w:p><w:r><w:t>visible prefix</w:t></w:r></w:p>`
					suffix := `<w:p><w:r><w:t>visible suffix</w:t></w:r></w:p>`
					inner := `<w:p><w:r><w:t>additional body text</w:t></w:r></w:p>`
					body := prefix + `<w:` + name + `>` + inner + `</w:` + name + `>` + suffix
					if position == "inline" {
						body = `<w:p><w:r><w:t>visible prefix</w:t></w:r><w:` + name + `><w:r><w:t>additional body text</w:t></w:r></w:` + name + `><w:r><w:t>visible suffix</w:t></w:r></w:p>`
					}
					got, e := runDOCX(t, testContent, testRelationships(pair[1]), testMain(pair[0], body), nil)
					if !errors.Is(e, ErrUnsupported) || got != (Result{}) {
						t.Fatalf("story wrapper produced prefix/suffix: %#v %v want unsupported zero", got, e)
					}
				})
			}
		}
	}
	for _, inner := range []string{`<w:p><w:r><w:t>missing-end`, strings.Repeat(`<w:p><w:r><w:t>`+strings.Repeat("a", 32000)+`</w:t></w:r></w:p>`, 5)} {
		body := `<w:p><w:r><w:t>prefix</w:t></w:r></w:p><w:customXml>` + inner + `</w:customXml><w:p><w:r><w:t>suffix</w:t></w:r></w:p>`
		got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, body), nil)
		if !errors.Is(e, ErrUnsupported) || got != (Result{}) {
			t.Fatalf("unsupported story need not resolve structure/output: %#v %v", got, e)
		}
	}
	got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), `<w:document xmlns:w="`+testWordNS+`"><w:unknownStoryBlock/><w:body><w:p><w:r><w:t>prefix</w:t></w:r></w:p></w:body></w:document>`, nil)
	if !errors.Is(e, ErrUnsupported) || got != (Result{}) {
		t.Fatalf("unknown document child: %#v %v", got, e)
	}
}

func TestDOCXExplicitExcludedWordSubtrees(t *testing.T) {
	run := `<w:r><w:t>prefixsuffix</w:t></w:r>`
	for _, tc := range []struct{ name, body string }{
		{"paragraph-properties", `<w:p><w:pPr><w:r><w:t>excluded</w:t></w:r></w:pPr>` + run + `</w:p>`},
		{"run-properties", `<w:p><w:r><w:rPr><w:t>excluded</w:t></w:rPr><w:t>prefixsuffix</w:t></w:r></w:p>`},
		{"table-properties", `<w:tbl><w:tblPr><w:p><w:r><w:t>excluded</w:t></w:r></w:p></w:tblPr><w:tblGrid/><w:tr><w:trPr/><w:tc><w:tcPr/><w:p>` + run + `</w:p></w:tc></w:tr></w:tbl>`},
		{"sdt-properties", `<w:sdt><w:sdtPr/><w:sdtEndPr/><w:sdtContent><w:p>` + run + `</w:p></w:sdtContent></w:sdt>`},
		{"section-properties", `<w:p>` + run + `</w:p><w:sectPr><w:p><w:r><w:t>excluded</w:t></w:r></w:p></w:sectPr>`},
		{"graphics", `<w:p><w:r><w:drawing><w:t>excluded</w:t></w:drawing><w:pict/><w:object/><w:t>prefixsuffix</w:t></w:r></w:p>`},
		{"deleted-content", `<w:p><w:del><w:r><w:t>excluded</w:t></w:r></w:del><w:r><w:delText>excluded</w:delText><w:t>prefixsuffix</w:t></w:r></w:p>`},
		{"references-and-layout", `<w:p><w:r><w:footnoteReference/><w:endnoteReference/><w:commentReference/><w:lastRenderedPageBreak/><w:t>prefixsuffix</w:t></w:r></w:p>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, tc.body), nil)
			if e != nil || got != (Result{Text: "prefixsuffix\n", Pages: 1, Format: DOCX}) {
				t.Fatalf("explicit exclusion: %#v %v", got, e)
			}
		})
	}
	for _, body := range []string{`<w:p><w:r><w:pPr/><w:t>prefix</w:t></w:r></w:p>`, `<w:drawing/><w:p>` + run + `</w:p>`, `<w:p><w:sectPr/>` + run + `</w:p>`} {
		got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, body), nil)
		if !errors.Is(e, ErrInvalid) || got != (Result{}) {
			t.Fatalf("excluded element has invalid parent: %#v %v", got, e)
		}
	}
	got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), `<w:document xmlns:w="`+testWordNS+`"><w:background><w:t>excluded</w:t></w:background><w:body><w:p>`+run+`</w:p></w:body></w:document>`, nil)
	if e != nil || got.Text != "prefixsuffix\n" {
		t.Fatalf("document background exclusion: %#v %v", got, e)
	}
}

func TestDOCXFix1WhitelistStillBoundsSupportedAndExcludedContent(t *testing.T) {
	text := strings.Repeat(`<w:r><w:t>`+strings.Repeat("a", 32000)+`</w:t></w:r>`, 5)
	got, e := runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<w:p><w:ins>`+text+`</w:ins></w:p>`), nil)
	if !errors.Is(e, ErrLimit) || got != (Result{}) {
		t.Fatalf("supported wrapper bypassed output quota: %#v %v", got, e)
	}
	got, e = runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<w:p><w:pPr><!--`+strings.Repeat("a", maxXMLToken+1)+`--></w:pPr><w:r><w:t>prefix</w:t></w:r></w:p>`), nil)
	if !errors.Is(e, ErrLimit) || got != (Result{}) {
		t.Fatalf("excluded subtree bypassed lexical guard: %#v %v", got, e)
	}
	got, e = runDOCX(t, testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<w:p><w:ins><w:p><w:r><w:t>prefix</w:t></w:r></w:p></w:ins></w:p>`), nil)
	if !errors.Is(e, ErrInvalid) || got != (Result{}) {
		t.Fatalf("supported wrapper bypassed structure: %#v %v", got, e)
	}
}
func TestDOCXCooperativeCancellationKeepsAdmission(t *testing.T) {
	p := New()
	r := &heldReader{entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan extraction, 1)
	var release sync.Once
	joined := false
	t.Cleanup(func() {
		cancel()
		release.Do(func() { close(r.release) })
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("DOCX did not join")
			}
		}
	})
	go func() { got, e := p.Extract(ctx, DOCX, r, 100); done <- extraction{got, e} }()
	select {
	case <-r.entered:
	case <-time.After(time.Second):
		t.Fatal("DOCX not admitted")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("DOCX abandoned actual read")
	default:
	}
	wait := &countReader{}
	waitCtx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	expectError(t, p, waitCtx, TXT, wait, 1, ErrCanceled)
	if wait.calls.Load() != 0 {
		t.Fatal("DOCX released shared admission")
	}
	release.Do(func() { close(r.release) })
	got := awaitExtraction(t, done)
	joined = true
	if !errors.Is(got.err, ErrCanceled) || got.result != (Result{}) {
		t.Fatal("cancellation prefix")
	}
	expectTextWithParser(t, p)
}
