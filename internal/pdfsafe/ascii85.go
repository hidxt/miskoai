// file with help function for ascii85 decoder
// later if new decoders is going to add it reasonable to rename file and add them here
// also create interfaces to switch between them (like in unidoc)

// Adapted from the pinned ledongthuc/pdf ascii85.go; see UPSTREAM.md.
package pdfsafe

import "io"

type alphaReader struct {
	reader io.Reader
	b      *budget
	eod    bool
	tilde  bool
}

func newAlphaReader(r io.Reader, b *budget) *alphaReader { return &alphaReader{reader: r, b: b} }
func isASCII85(c byte) bool                              { return c >= '!' && c <= 'u' || c == 'z' }
func (a *alphaReader) Read(p []byte) (int, error) {
	a.b.charge(1)
	if len(p) == 0 {
		return 0, nil
	}
	if a.eod {
		return 0, io.EOF
	}
	for {
		a.b.charge(1)
		n, e := a.reader.Read(p)
		if n < 0 || n > len(p) || n == 0 && e == nil {
			fail(errMalformed)
		}
		out := 0
		for i := 0; i < n; i++ {
			a.b.charge(1)
			c := p[i]
			if a.tilde {
				if c != '>' {
					fail(errMalformed)
				}
				a.eod = true
				return out, io.EOF
			}
			if c == '~' {
				a.tilde = true
				continue
			}
			if isSpace(c) {
				continue
			}
			if !isASCII85(c) {
				fail(errMalformed)
			}
			p[out] = c
			out++
		}
		if e != nil {
			if e != io.EOF {
				fail(errMalformed)
			}
			if a.tilde {
				fail(errMalformed)
			}
			a.eod = true
			return out, io.EOF
		}
		if out > 0 {
			return out, nil
		}
	}
}
