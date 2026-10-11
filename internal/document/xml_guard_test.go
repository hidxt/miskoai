package document

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

func guardDecode(s string, r io.Reader) error {
	if r == nil {
		r = strings.NewReader(s)
	}
	d := xml.NewDecoder(newXMLGuard(context.Background(), r))
	for {
		_, e := d.Token()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
	}
}
func TestXMLPreTokenBudgets(t *testing.T) {
	for _, tc := range []struct {
		name, s string
		limit   bool
	}{
		{"depth64", strings.Repeat("<a>", 64) + strings.Repeat("</a>", 64), false}, {"depth65", strings.Repeat("<a>", 65) + strings.Repeat("</a>", 65), true},
		{"selfcloseddepth65", strings.Repeat("<a>", 64) + "<b/>" + strings.Repeat("</a>", 64), true},
		{"text64k", "<a>" + strings.Repeat("x", 65536) + "</a>", false}, {"text64k1", "<a>" + strings.Repeat("x", 65537) + "</a>", true},
		{"utf8text64k", "<a>" + strings.Repeat("中", 21845) + "a</a>", false},
		{"utf8text64k1", "<a>" + strings.Repeat("中", 21845) + "ab</a>", true},
		{"tag16k", "<a v='" + strings.Repeat("x", 16384-9) + "'/>", false}, {"tag16k1", "<a v='" + strings.Repeat("x", 16385-9) + "'/>", true},
		{"endtag16k", "<" + strings.Repeat("a", 16381) + "></" + strings.Repeat("a", 16381) + ">", false},
		{"endtag16k1", "<" + strings.Repeat("a", 16382) + "></" + strings.Repeat("a", 16382) + ">", true},
		{"comment64k", "<a><!--" + strings.Repeat("x", 65536) + "--></a>", false}, {"comment64k1", "<a><!--" + strings.Repeat("x", 65537) + "--></a>", true},
		{"cdata64k", "<a><![CDATA[" + strings.Repeat("x", 65536) + "]]></a>", false}, {"cdata64k1", "<a><![CDATA[" + strings.Repeat("x", 65537) + "]]></a>", true},
		{"pi64k", "<a><?p " + strings.Repeat("x", 65534) + "?></a>", false}, {"pi64k1", "<a><?p " + strings.Repeat("x", 65535) + "?></a>", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := guardDecode(tc.s, nil)
			if tc.limit {
				if !errors.Is(e, ErrLimit) {
					t.Fatalf("want pre-token limit got %v", e)
				}
			} else if e != nil {
				t.Fatal(e)
			}
		})
	}
	for _, n := range []int{128, 129} {
		s := "<a"
		for i := 0; i < n; i++ {
			s += ` x` + strings.Repeat("a", i) + `="v"`
		}
		s += "/>"
		e := guardDecode(s, nil)
		if n == 128 && e != nil {
			t.Fatal(e)
		}
		if n == 129 && !errors.Is(e, ErrLimit) {
			t.Fatalf("attributes: %v", e)
		}
	}
}

type splitXMLReader struct {
	s string
	n int
}

func (r *splitXMLReader) Read(p []byte) (int, error) {
	if len(r.s) == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.n {
		n = r.n
	}
	if n > len(r.s) {
		n = len(r.s)
	}
	copy(p, r.s[:n])
	r.s = r.s[n:]
	return n, nil
}
func TestXMLDelimiterChunkBoundaries(t *testing.T) {
	s := "\ufeff<?xml version='1.0' encoding='UTF-8'?><a x='&lt; > / &quot; ='><![CDATA[中文 >]]><!-- 中文 --><?p hi?><b>&amp;&apos;&quot;&gt;&lt;&#x1F600;&#9;&#10;&#13;</b></a>"
	for n := 1; n <= len(s); n++ {
		if e := guardDecode(s, &splitXMLReader{s, n}); e != nil {
			t.Fatalf("chunk %d: %v", n, e)
		}
	}
	for _, s := range []string{"<a>\xff</a>", "<a>\xed\xa0\x80</a>", "<a>\x00</a>", "<a>\xc2</a>", "<a>&#xD800;</a>", "<a>&custom;</a>", "<a>&#0;</a>"} {
		if e := guardDecode(s, nil); e == nil {
			t.Fatalf("invalid character/entity accepted: %q", s)
		}
	}
	if e := guardDecode(`<!DOCTYPE a><a/>`, nil); !errors.Is(e, ErrUnsupported) {
		t.Fatalf("DTD: %v", e)
	}
	if e := guardDecode(`<?xml version="1.0" encoding="UTF-16"?><a/>`, nil); e == nil {
		t.Fatal("encoding accepted")
	}
}

func TestXMLNamespaceAttrsAndMetadataGuard(t *testing.T) {
	for _, n := range []int{128, 129} {
		s := "<a"
		for i := 0; i < n; i++ {
			s += ` xmlns:n` + strings.Repeat("a", i) + `="urn:x"`
		}
		s += "/>"
		e := guardDecode(s, nil)
		if n == 128 && e != nil {
			t.Fatal(e)
		}
		if n == 129 && !errors.Is(e, ErrLimit) {
			t.Fatalf("namespace attrs: %v", e)
		}
	}
	main := testMain(testWordNS, `<w:p><w:r><w:t>prefix</w:t></w:r></w:p>`)
	for _, ct := range []string{`<!DOCTYPE Types>` + testContent, strings.Replace(testContent, `</Types>`, `<!--`+strings.Repeat("x", maxXMLToken+1)+`--></Types>`, 1)} {
		got, e := runDOCX(t, ct, testRelationships(testOfficeRel), main, nil)
		if e == nil || got != (Result{}) {
			t.Fatalf("content types guard: %#v %v", got, e)
		}
	}
	for _, rel := range []string{`<!DOCTYPE Relationships>` + testRelationships(testOfficeRel), strings.Replace(testRelationships(testOfficeRel), `</Relationships>`, `<!--`+strings.Repeat("x", maxXMLToken+1)+`--></Relationships>`, 1)} {
		got, e := runDOCX(t, testContent, rel, main, nil)
		if e == nil || got != (Result{}) {
			t.Fatalf("relationships guard: %#v %v", got, e)
		}
	}
}

func FuzzXMLGuard(f *testing.F) {
	for _, s := range []string{`<a/>`, `<a>&amp;&#x1F600;</a>`, `<!DOCTYPE a><a/>`, `<a><![CDATA[中文]]></a>`, `<a>\xff</a>`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 128*1024 {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100000000)
		defer cancel()
		d := xml.NewDecoder(newXMLGuard(ctx, strings.NewReader(s)))
		for {
			_, e := d.Token()
			if e != nil {
				return
			}
		}
	})
}
