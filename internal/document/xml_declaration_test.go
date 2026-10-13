package document

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

// Removing declaration admission must make these finite, real DOCX fixtures
// return successful text for malformed declarations instead of a safe refusal.
func TestDOCXDeclarationRefusals(t *testing.T) {
	for _, tc := range declarationRefusals() {
		for part := 0; part < 3; part++ {
			t.Run(tc.name+"/"+[]string{"content", "relationships", "main"}[part], func(t *testing.T) {
				parts := declarationParts()
				parts[part] = tc.wrap(parts[part])
				got, err := runDOCX(t, parts[0], parts[1], parts[2], nil)
				if got != (Result{}) || !errors.Is(err, tc.want) {
					t.Fatalf("safe declaration refusal: result=%#v error=%v want=%v", got, err, tc.want)
				}
				if err != nil && strings.Contains(err.Error(), "HOSTILE_CANARY") {
					t.Fatal("declaration data leaked into error")
				}
			})
		}
	}
}

type declarationCase struct {
	name string
	wrap func(string) string
	want error
}

func declarationRefusals() []declarationCase {
	var cases []declarationCase
	for _, tc := range []struct {
		name, declaration string
		want              error
	}{
		{"empty", "<?xml?>", ErrInvalid},
		{"missing-version", `<?xml encoding="UTF-8"?>`, ErrInvalid},
		{"unknown", `<?xml version="1.0" secret="HOSTILE_CANARY"?>`, ErrInvalid},
		{"extra-data", `<?xml version="1.0" HOSTILE_CANARY?>`, ErrInvalid},
		{"duplicate-version", `<?xml version="1.0" version="1.0"?>`, ErrInvalid},
		{"duplicate-encoding", `<?xml version="1.0" encoding="UTF-8" encoding="UTF-8"?>`, ErrInvalid},
		{"duplicate-standalone", `<?xml version="1.0" standalone="yes" standalone="no"?>`, ErrInvalid},
		{"reordered", `<?xml version="1.0" standalone="yes" encoding="UTF-8"?>`, ErrInvalid},
		{"encoding-first", `<?xml encoding="UTF-8" version="1.0"?>`, ErrInvalid},
		{"no-separator", `<?xml version="1.0"encoding="UTF-8"?>`, ErrInvalid},
		{"unquoted", `<?xml version=1.0?>`, ErrInvalid},
		{"mismatched-quote", `<?xml version='1.0"?>`, ErrInvalid},
		{"no-equals", `<?xml version "1.0"?>`, ErrInvalid},
		{"double-equals", `<?xml version=="1.0"?>`, ErrInvalid},
		{"entity-version", `<?xml version="1.&#48;"?>`, ErrInvalid},
		{"entity-encoding", `<?xml version="1.0" encoding="UTF&#45;8"?>`, ErrInvalid},
		{"bad-standalone", `<?xml version="1.0" standalone="true"?>`, ErrInvalid},
		{"case-standalone", `<?xml version="1.0" standalone="YES"?>`, ErrInvalid},
		{"empty-version", `<?xml version=""?>`, ErrInvalid},
		{"empty-encoding", `<?xml version="1.0" encoding=""?>`, ErrInvalid},
		{"bad-encoding-name", `<?xml version="1.0" encoding="8UTF"?>`, ErrInvalid},
		{"spaced-encoding", `<?xml version="1.0" encoding="UTF 8"?>`, ErrInvalid},
		{"version-data", `<?xml version="HOSTILE_CANARY"?>`, ErrInvalid},
		{"standalone-entity", `<?xml version="1.0" standalone="y&#101;s"?>`, ErrInvalid},
		{"unsupported-and-malformed", `<?xml version="1.1" unknown="HOSTILE_CANARY"?>`, ErrInvalid},
		{"non-xml-attribute-space", "<?xml version\u00a0='1.0'?>", ErrInvalid},
		{"attribute-case", `<?xml Version="1.0"?>`, ErrInvalid},
		{"non-xml-space", "<?xml\u00a0version='1.0'?>", ErrInvalid},
		{"version-1.1", `<?xml version="1.1"?>`, ErrUnsupported},
		{"version-2.0", `<?xml version="2.0"?>`, ErrUnsupported},
		{"encoding-utf16", `<?xml version="1.0" encoding="UTF-16"?>`, ErrUnsupported},
		{"encoding-latin", `<?xml version="1.0" encoding="ISO-8859-1"?>`, ErrUnsupported},
	} {
		decl := tc.declaration
		cases = append(cases, declarationCase{tc.name, func(s string) string { return decl + s }, tc.want})
	}
	for _, target := range []string{"XML", "Xml", "xMl", "xmL", "XMl", "xML", "XmL"} {
		decl := "<?" + target + ` version="1.0"?>`
		cases = append(cases, declarationCase{"reserved-" + target, func(s string) string { return decl + s }, ErrInvalid})
	}
	decl := `<?xml version="1.0"?>`
	cases = append(cases,
		declarationCase{"duplicate", func(s string) string { return decl + decl + s }, ErrInvalid},
		declarationCase{"whitespace-prefix", func(s string) string { return " \r\n" + decl + s }, ErrInvalid},
		declarationCase{"comment-prefix", func(s string) string { return "<!--x-->" + decl + s }, ErrInvalid},
		declarationCase{"pi-prefix", func(s string) string { return "<?p x?>" + decl + s }, ErrInvalid},
		declarationCase{"trailing", func(s string) string { return s + decl }, ErrInvalid},
		declarationCase{"nested", func(s string) string { i := strings.IndexByte(s, '>') + 1; return s[:i] + decl + s[i:] }, ErrInvalid},
		declarationCase{"bom-space", func(s string) string { return "\ufeff " + decl + s }, ErrInvalid},
	)
	return cases
}

func declarationParts() [3]string {
	return [3]string{testContent, testRelationships(testOfficeRel), testMain(testWordNS, `<w:p><w:r><w:t>ok 中文</w:t></w:r></w:p>`)}
}

func TestDOCXDeclarationControls(t *testing.T) {
	for _, decl := range []string{
		"", "\ufeff", `<?xml version="1.0"?>`, "\ufeff<?xml version='1.0'?>",
		`<?xml version='1.0' encoding="uTf-8"?>`,
		`<?xml version='1.0' standalone='yes'?>`,
		`<?xml version="1.0" encoding='UTF-8' standalone="no"?>`,
		"<?xml\tversion \r=\n '1.0'\r\nencoding\t= \"utf-8\"\nstandalone = 'yes' \t?>",
	} {
		t.Run(decl, func(t *testing.T) {
			parts := declarationParts()
			for i := range parts {
				parts[i] = decl + parts[i]
			}
			got, err := runDOCX(t, parts[0], parts[1], parts[2], nil)
			if err != nil || got != (Result{Text: "ok 中文\n", Pages: 1, Format: DOCX}) {
				t.Fatalf("supported declaration: %#v %v", got, err)
			}
		})
	}
	parts := declarationParts()
	for i := range parts {
		parts[i] = `<?xml-stylesheet href="https://example.invalid/HOSTILE_CANARY"?><?XML-stylesheet inert?>` + parts[i] + `<?p HOSTILE_CANARY?>`
	}
	got, err := runDOCX(t, parts[0], parts[1], parts[2], nil)
	if err != nil || got.Text != "ok 中文\n" {
		t.Fatalf("inert ordinary PI: %#v %v", got, err)
	}
}

func TestXMLDeclarationFragmentation(t *testing.T) {
	for _, tc := range declarationRefusals() {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.wrap("<a>ok</a>")
			for _, fragment := range []int{1, 2, 3, len(s)} {
				d := xml.NewDecoder(newXMLGuard(context.Background(), &splitXMLReader{s, fragment}))
				var err error
				for err == nil {
					_, err = safeXMLToken(context.Background(), d)
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("fragment=%d error=%v want=%v", fragment, err, tc.want)
				}
			}
		})
	}
	for _, s := range []string{"\ufeff<?xml version='1.0' encoding='uTf-8' standalone='no'?><a>中文</a>", "<?xml-stylesheet inert?><a/>", "<?xml version='1.0'?><?xml-stylesheet inert?><a/>"} {
		for _, fragment := range []int{1, 2, 3, len(s)} {
			if err := guardDecode(s, &splitXMLReader{s, fragment}); err != nil {
				t.Fatalf("supported fragmented input: %v", err)
			}
		}
	}
}

func TestXMLDeclarationCancellationAndBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := io.ReadAll(newXMLGuard(ctx, strings.NewReader(`<?xml version='1.0'?><a/>`)))
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("canceled declaration: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	reader := &declarationCancelReader{splitXMLReader: splitXMLReader{"\ufeff<?xml version='1.0' encoding='UTF-8'?><a/>", 1}, cancel: cancel}
	_, err = io.ReadAll(newXMLGuard(ctx, reader))
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("canceled during fragmented declaration: %v", err)
	}
	for _, n := range []int{maxXMLToken, maxXMLToken + 1} {
		prefix := "xml version='1.0'"
		s := "<?" + prefix + strings.Repeat(" ", n-len(prefix)) + "?><a/>"
		_, err = io.ReadAll(newXMLGuard(context.Background(), strings.NewReader(s)))
		if n == maxXMLToken && err != nil {
			t.Fatalf("exact admitted declaration PI: %v", err)
		}
		if n > maxXMLToken && !errors.Is(err, ErrLimit) {
			t.Fatalf("oversized declaration PI: %v", err)
		}
	}
	_, err = io.ReadAll(newXMLGuard(context.Background(), strings.NewReader("<?xml "+strings.Repeat("x", maxXMLToken)+"?><a/>")))
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("PI budget: %v", err)
	}
}

type declarationCancelReader struct {
	splitXMLReader
	cancel context.CancelFunc
	reads  int
}

func (r *declarationCancelReader) Read(p []byte) (int, error) {
	r.reads++
	if r.reads == 12 {
		r.cancel()
	}
	return r.splitXMLReader.Read(p)
}

func TestDOCXDeclarationDelimiterRefusals(t *testing.T) {
	for _, decl := range []string{"<?xml/?>", "<?xml=?>", "<?XML/?>"} {
		for part := 0; part < 3; part++ {
			t.Run(decl+"/"+[]string{"content", "relationships", "main"}[part], func(t *testing.T) {
				parts := declarationParts()
				parts[part] = decl + parts[part]
				got, err := runDOCX(t, parts[0], parts[1], parts[2], nil)
				if got != (Result{}) || !errors.Is(err, ErrInvalid) {
					t.Fatalf("reserved parsed PI target: result=%#v error=%v want=%v", got, err, ErrInvalid)
				}
			})
		}
	}
}

func TestDOCXDeclarationOrdinaryTargetControls(t *testing.T) {
	// These are distinct XML Name targets. The correction must not invent a
	// narrower ASCII-only Name grammar or treat a reserved prefix as reserved.
	for _, target := range []string{"xml:", "XML:", "xml-stylesheet", "XML-stylesheet", "xml.foo", "xml_foo", "xml中文", "中文", "xmlλ", "λ", "xmlversion"} {
		t.Run(target, func(t *testing.T) {
			parts := declarationParts()
			for i := range parts {
				parts[i] = "<?" + target + " HOSTILE_CANARY?>" + parts[i] + "<?" + target + "?>"
			}
			got, err := runDOCX(t, parts[0], parts[1], parts[2], nil)
			if err != nil || got != (Result{Text: "ok 中文\n", Pages: 1, Format: DOCX}) {
				t.Fatalf("ordinary parsed PI target: %#v %v", got, err)
			}
		})
	}
}

func TestXMLDeclarationParsedDelimiterFragmentation(t *testing.T) {
	for _, decl := range []string{"<?xml/?>", "<?xml=?>", "<?XML/?>", "<?xml/version='1.0'?>", "<?xml=version='1.0'?>"} {
		t.Run(decl, func(t *testing.T) {
			for _, prefix := range []string{"", "\ufeff", "<?p inert?>", "<a>"} {
				s := prefix + decl + "<a/>"
				for _, fragment := range []int{1, 2, 3, len(s)} {
					d := xml.NewDecoder(newXMLGuard(context.Background(), &splitXMLReader{s, fragment}))
					var err error
					for err == nil {
						_, err = safeXMLToken(context.Background(), d)
					}
					if !errors.Is(err, ErrInvalid) {
						t.Fatalf("reserved target fragment=%d error=%v", fragment, err)
					}
				}
			}
		})
	}
	for _, s := range []string{"<?xml:?><?xml中文 HOSTILE_CANARY?><a/>", "\ufeff<?xml version='1.0' encoding='uTf-8' standalone='no'?><a/>"} {
		for _, fragment := range []int{1, 2, 3, len(s)} {
			d := xml.NewDecoder(newXMLGuard(context.Background(), &splitXMLReader{s, fragment}))
			var err error
			for err == nil {
				_, err = safeXMLToken(context.Background(), d)
			}
			if err != io.EOF {
				t.Fatalf("ordinary or declaration fragment=%d error=%v", fragment, err)
			}
		}
	}
}
