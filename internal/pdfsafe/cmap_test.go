package pdfsafe

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func cmapPDF(cmap, codes string) []byte {
	font := "<< /Type /Font /Subtype /Type0 /Encoding /Identity-H /ToUnicode 6 0 R /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SyntheticGB /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 5 >> >>] >>"
	return objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", font, pdfStream("BT /F1 12 Tf <" + codes + "> Tj ET"), pdfStream(cmap)}, "")
}
func TestPDFCMapTask2(t *testing.T) {
	t.Run("canonical", func(t *testing.T) {
		cmap := `/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CIDSystemInfo << /Registry (synthetic) /Ordering (UCS) /Supplement 0 >> def /CMapName /Synthetic def /CMapType 2 def 1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfchar <01> <4E2D6587> endbfchar 1 beginbfrange <02> <04> <0041> endbfrange 1 beginbfrange <05> <06> [<D83DDE00> <0061>] endbfrange endcmap CMapName currentdict /CMap defineresource pop end end`
		wantExtract(t, cmapPDF(cmap, "010203040506"), "中文ABC😀a", 1)
	})
	t.Run("variable_length", func(t *testing.T) {
		cmap := "begincmap 2 begincodespacerange <00> <7F> <8000> <80FF> endcodespacerange 2 beginbfchar <41> <0041> <8001> <4E2D> endbfchar endcmap"
		wantExtract(t, cmapPDF(cmap, "418001"), "A中", 1)
	})
	t.Run("four_byte", func(t *testing.T) {
		cmap := "begincmap 1 begincodespacerange <FFFFFFFF> <FFFFFFFF> endcodespacerange 1 beginbfchar <FFFFFFFF> <0041> endbfchar endcmap"
		wantExtract(t, cmapPDF(cmap, "FFFFFFFF"), "A", 1)
	})
	for _, tc := range []struct {
		name, body string
		want       error
	}{{"count_limit", "8193 beginbfchar", ErrLimit}, {"negative_count", "-1 beginbfchar", ErrInvalid}, {"odd_unicode", "1 beginbfchar <41> <00> endbfchar", ErrInvalid}, {"surrogate", "1 beginbfchar <41> <D800> endbfchar", ErrInvalid}, {"range_reverse", "1 beginbfrange <42> <41> <0041> endbfrange", ErrInvalid}, {"range_length", "1 beginbfrange <41> <0042> <0041> endbfrange", ErrInvalid}, {"array_length", "1 beginbfrange <41> <42> [<0041>] endbfrange", ErrInvalid}, {"unknown_code", "1 beginbfchar <42> <0041> endbfchar", ErrInvalid}, {"duplicate", "2 beginbfchar <41> <0041> <41> <0042> endbfchar", ErrInvalid}, {"bad_ending", "1 beginbfchar <41> <0041> endbfrange", ErrInvalid}, {"overflow_replacement", "1 beginbfrange <41> <42> <FFFF> endbfrange", ErrInvalid}, {"unsupported_usecmap", "/External usecmap", ErrUnsupported}} {
		t.Run(tc.name, func(t *testing.T) {
			cmap := "begincmap 1 begincodespacerange <00> <FF> endcodespacerange " + tc.body + " endcmap"
			wantExtractError(t, cmapPDF(cmap, "41"), tc.want)
		})
	}
	for _, tc := range []string{"2 begincodespacerange <00> <FF> <00> <7F> endcodespacerange", "2 begincodespacerange <00> <FF> <0000> <FFFF> endcodespacerange", "1 begincodespacerange <FF> <00> endcodespacerange", "1 begincodespacerange <00> <FFFF> endcodespacerange", "1 begincodespacerange <0000000000> <0000000001> endcodespacerange"} {
		t.Run(tc, func(t *testing.T) { wantExtractError(t, cmapPDF("begincmap "+tc+" endcmap", "41"), ErrInvalid) })
	}
	t.Run("expansion", func(t *testing.T) {
		cmap := "begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfrange <0000> <2000> <0041> endbfrange endcmap"
		wantExtractError(t, cmapPDF(cmap, "0000"), ErrLimit)
	})
	t.Run("entries_exact", func(t *testing.T) {
		cmap := "begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfrange <0000> <1FFF> <0041> endbfrange endcmap"
		wantExtract(t, cmapPDF(cmap, "0000"), "A", 1)
	})
	for _, n := range []int{1024, 1026} {
		t.Run(fmt.Sprint("replacement", n), func(t *testing.T) {
			replacement := strings.Repeat("0041", n/2)
			cmap := "begincmap 1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfchar <41> <" + replacement + "> endbfchar endcmap"
			if n == 1024 {
				wantExtract(t, cmapPDF(cmap, "41"), strings.Repeat("A", 512), 1)
			} else {
				wantExtractError(t, cmapPDF(cmap, "41"), ErrLimit)
			}
		})
	}
	t.Run("truncated_code", func(t *testing.T) {
		cmap := "begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfchar <0041> <0041> endbfchar endcmap"
		wantExtractError(t, cmapPDF(cmap, "00"), ErrInvalid)
	})
	t.Run("retention_and_work_before_mapping", func(t *testing.T) {
		for _, work := range []bool{false, true} {
			b := newBudget(context.Background())
			f := &textFont{mappings: make(map[string]string), spaces: []codeSpace{{0, 255, 1}}}
			if work {
				b.work = maxWork
			} else {
				b.retained = maxRetained
			}
			mustFault(t, errLimit, func() { f.addMapping(b, "A", "\x00A") })
			if len(f.mappings) != 0 {
				t.Fatal("mapping admitted before budget")
			}
		}
	})
}

// Every font and page shares the admitted extraction's CMap lifetime budget.
func TestPDFCMapLifetimeFix1(t *testing.T) {
	t.Run("code_spaces_separate_and_before_append", func(t *testing.T) {
		b := newBudget(context.Background())
		b.cmapMappings = maxCMapEntries
		f := &textFont{}
		f.addSpace(b, "\x00", "\xff")
		if b.cmapSpaces != 1 || b.cmapMappings != maxCMapEntries {
			t.Fatal("code spaces consumed mapping allowance")
		}
		b.cmapSpaces = maxCMapEntries
		next := &textFont{}
		retained := b.retained
		mustFault(t, ErrLimit, func() { next.addSpace(b, "\x00", "\xff") })
		if len(next.spaces) != 0 || cap(next.spaces) != 0 || b.retained != retained {
			t.Fatal("code-space limit checked after allocation")
		}
	})
	t.Run("range_preflight_before_replacement", func(t *testing.T) {
		first, b := streamFixture([]byte("begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfrange <0000> <1FFF> <0041> endbfrange endcmap"), nil, nil)
		parseCMap(first)
		second, _ := streamFixture([]byte("begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfrange <0000> <0000>"), nil, nil)
		second.r.b = b
		second.r.f.(*checkedReaderAt).b = b
		mustFault(t, ErrLimit, func() { parseCMap(second) })
		if len(b.streams) != 0 {
			t.Fatal("preflight failure leaked actual stream")
		}
	})
	font := func(id int) string {
		return fmt.Sprintf("<< /Type /Font /Subtype /Type0 /Encoding /Identity-H /ToUnicode %d 0 R /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SyntheticGB /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 5 >> >>] >>", id)
	}
	cmap := func(end string) string {
		return pdfStream("begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfrange <0000> <" + end + "> <0041> endbfrange endcmap")
	}
	for _, samePage := range []bool{false, true} {
		for _, extra := range []bool{false, true} {
			t.Run(fmt.Sprintf("same_page_%t_over_limit_%t", samePage, extra), func(t *testing.T) {
				hi := "0FFF"
				if extra {
					hi = "1000"
				}
				var p []byte
				if samePage {
					p = objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R /F2 6 0 R >> >> /Contents 5 0 R >>", font(7), pdfStream("BT /F1 12 Tf <0000> Tj /F2 12 Tf <0000> Tj ET"), font(8), cmap("0FFF"), cmap(hi)}, "")
				} else {
					p = objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 2 /Kids [3 0 R 6 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", font(9), pdfStream("BT /F1 12 Tf <0000> Tj ET"), "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 7 0 R >> >> /Contents 8 0 R >>", font(10), pdfStream("BT /F1 12 Tf <0000> Tj ET"), cmap("0FFF"), cmap(hi)}, "")
				}
				if extra {
					wantExtractError(t, p, ErrLimit)
				} else if samePage {
					wantExtract(t, p, "AA", 1)
				} else {
					wantExtract(t, p, "A\nA", 2)
				}
			})
		}
	}
	for _, extra := range []bool{false, true} {
		t.Run(fmt.Sprint("multiple_blocks_over_limit_", extra), func(t *testing.T) {
			hi := "1FFF"
			if extra {
				hi = "2000"
			}
			s := "begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfrange <0000> <0FFF> <0041> endbfrange 1 beginbfrange <1000> <" + hi + "> <0041> endbfrange endcmap"
			p := cmapPDF(s, "00001000")
			if extra {
				wantExtractError(t, p, ErrLimit)
			} else {
				wantExtract(t, p, "AA", 1)
			}
		})
	}
	t.Run("cached_font_across_pages", func(t *testing.T) {
		p := objectPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 2 /Kids [3 0 R 6 0 R] >>", "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", font(8), pdfStream("BT /F1 12 Tf <0000> Tj /F1 12 Tf <0000> Tj ET"), "<< /Type /Page /Parent 2 0 R /Resources << /Font << /Renamed 4 0 R >> >> /Contents 7 0 R >>", pdfStream("BT /Renamed 12 Tf <0000> Tj ET"), cmap("1FFF")}, "")
		wantExtract(t, p, "AA\nA", 2)
	})
}
