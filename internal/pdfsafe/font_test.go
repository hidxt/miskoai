package pdfsafe

import (
	"fmt"
	"testing"
)

func TestPDFFontIdentityTask2(t *testing.T) {
	t.Run("missing_descendant", func(t *testing.T) {
		wantExtractError(t, textPDF("BT /F1 12 Tf <4E2D> Tj ET", "<< /Type /Font /Subtype /Type0 /Encoding /UniGB-UCS2-H >>"), ErrInvalid)
	})
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprint("direct", direct), func(t *testing.T) {
			objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 2 /Kids [3 0 R 6 0 R] >>", "", latinFont, pdfStream("BT /F1 12 Tf <80> Tj ET"), "", "<< /Type /Font /Subtype /Type0 /Encoding /UniGB-UCS2-H /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SyntheticGB /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 5 >> >>] >>", pdfStream("BT /F1 12 Tf <4E2D6587> Tj ET")}
			f1 := "4 0 R"
			f2 := "7 0 R"
			if direct {
				f1 = objects[3]
				f2 = objects[6]
			}
			objects[2] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 %s >> >> /Contents 5 0 R >>", f1)
			objects[5] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 %s >> >> /Contents 8 0 R >>", f2)
			wantExtract(t, objectPDF(objects, ""), "€\n中文", 2)
		})
	}
	t.Run("differences", func(t *testing.T) {
		font := "<< /Type /Font /Subtype /Type1 /Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /A] >> >>"
		wantExtractError(t, textPDF("BT /F1 12 Tf (A) Tj ET", font), ErrUnsupported)
	})
	t.Run("mac_roman", func(t *testing.T) {
		wantExtract(t, textPDF("BT /F1 12 Tf <80> Tj ET", "<< /Type /Font /Subtype /Type1 /Encoding /MacRomanEncoding >>"), "Ä", 1)
	})
	t.Run("standard", func(t *testing.T) {
		wantExtract(t, textPDF("BT /F1 12 Tf <2760> Tj ET", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"), "’‘", 1)
	})
	t.Run("unsupported_byte", func(t *testing.T) { wantExtractError(t, textPDF("BT /F1 12 Tf <81> Tj ET", latinFont), ErrUnsupported) })
	for _, hex := range []string{"D800", "DC00", "D8000041", "4E2D65"} {
		t.Run(hex, func(t *testing.T) {
			wantExtractError(t, textPDF("BT /F1 12 Tf <"+hex+"> Tj ET", "<< /Type /Font /Subtype /Type0 /Encoding /UniGB-UCS2-H /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SyntheticGB /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 5 >> >>] >>"), ErrInvalid)
		})
	}
	t.Run("surrogate", func(t *testing.T) {
		wantExtract(t, textPDF("BT /F1 12 Tf <D83DDE00> Tj ET", "<< /Type /Font /Subtype /Type0 /Encoding /UniGB-UCS2-H /DescendantFonts [<< /Type /Font /Subtype /CIDFontType2 /BaseFont /SyntheticGB /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 5 >> >>] >>"), "😀", 1)
	})
}
