package document

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxXMLTag   = 16 * 1024
	maxXMLToken = 64 * 1024
	maxXMLAttrs = 128
	maxXMLDepth = 64
)

// xmlGuard releases only completely admitted lexical tokens. Decoder.Token
// therefore never receives an unbounded tag, attribute list or data token.
// Its source buffer is fixed; its carry is at most one bounded token plus a
// delimiter. All parts, including package metadata, use this same boundary.
type xmlGuard struct {
	ctx           context.Context
	source        *bufio.Reader
	token         []byte
	offset, depth int
	initial       bool
	firstToken    bool
	failed        error
	carry         [maxXMLToken + 12]byte
}

func newXMLGuard(ctx context.Context, r io.Reader) io.Reader {
	g := &xmlGuard{ctx: ctx, source: bufio.NewReaderSize(r, 4096), initial: true, firstToken: true}
	g.token = g.carry[:0]
	return g
}
func (g *xmlGuard) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if e := checkWork(g.ctx); e != nil {
		return 0, e
	}
	if g.failed != nil {
		return 0, g.failed
	}
	if g.offset == len(g.token) {
		g.token = g.token[:0]
		g.offset = 0
		if e := g.next(); e != nil {
			if workErr := checkWork(g.ctx); workErr != nil {
				e = workErr
			}
			g.failed = e
			return 0, e
		}
		if len(g.token) > 0 && (g.token[0] != '<' || len(g.token) > 1 && g.token[1] != '!' && g.token[1] != '?') {
			if e := validateXMLReferences(g.token); e != nil {
				g.failed = e
				return 0, e
			}
		}
	}
	n := copy(p, g.token[g.offset:])
	g.offset += n
	return n, nil
}
func (g *xmlGuard) rune() (rune, error) {
	if e := checkWork(g.ctx); e != nil {
		return 0, e
	}
	r, n, e := g.source.ReadRune()
	if e != nil {
		if e == io.EOF {
			return 0, e
		}
		return 0, ErrInvalid
	}
	if r == utf8.RuneError && n == 1 {
		return 0, ErrInvalid
	}
	if !(r == 9 || r == 10 || r == 13 || r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF) {
		return 0, ErrInvalid
	}
	return r, nil
}
func (g *xmlGuard) add(r rune) { g.token = utf8.AppendRune(g.token, r) }
func (g *xmlGuard) next() error {
	r, e := g.rune()
	if e != nil {
		return e
	}
	if g.initial {
		g.initial = false
		if r == 0xFEFF {
			r, e = g.rune()
			if e != nil {
				return e
			}
		}
	}
	g.add(r)
	first := g.firstToken
	g.firstToken = false
	if r != '<' {
		for {
			r, e = g.rune()
			if e == io.EOF {
				return nil
			}
			if e != nil {
				return e
			}
			if r == '<' {
				if e = g.source.UnreadRune(); e != nil {
					return ErrInvalid
				}
				return nil
			}
			if len(g.token)+utf8.RuneLen(r) > maxXMLToken {
				return ErrLimit
			}
			g.add(r)
		}
	}
	r, e = g.rune()
	if e != nil {
		return ErrInvalid
	}
	g.add(r)
	if r == '!' {
		r, e = g.rune()
		if e != nil {
			return ErrInvalid
		}
		g.add(r)
		switch r {
		case '-':
			r, e = g.rune()
			if e != nil || r != '-' {
				return ErrUnsupported
			}
			g.add(r)
			return g.delimited("-->", 4)
		case '[':
			for _, want := range "CDATA[" {
				r, e = g.rune()
				if e != nil || r != want {
					return ErrUnsupported
				}
				g.add(r)
			}
			return g.delimited("]]>", 9)
		default:
			return ErrUnsupported
		}
	}
	if r == '?' {
		if e := g.delimited("?>", 2); e != nil {
			return e
		}
		return validateXMLDeclaration(g.token, first)
	}
	end := r == '/'
	var quote rune
	attrs := 0
	for {
		r, e = g.rune()
		if e != nil {
			return ErrInvalid
		}
		if len(g.token)+utf8.RuneLen(r) > maxXMLTag {
			return ErrLimit
		}
		g.add(r)
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '=' {
			attrs++
			if attrs > maxXMLAttrs {
				return ErrLimit
			}
		}
		if r == '>' {
			if end {
				g.depth--
				if g.depth < 0 {
					return ErrInvalid
				}
			} else {
				if g.depth+1 > maxXMLDepth {
					return ErrLimit
				}
				if len(g.token) < 2 || g.token[len(g.token)-2] != '/' {
					g.depth++
				}
			}
			return nil
		}
	}
}

func validateXMLReferences(b []byte) error {
	for i := 0; i < len(b); i++ {
		if b[i] != '&' {
			continue
		}
		j := i + 1
		for j < len(b) && b[j] != ';' {
			j++
		}
		if j == len(b) {
			return ErrInvalid
		}
		s := string(b[i+1 : j])
		switch s {
		case "amp", "lt", "gt", "quot", "apos":
		default:
			if !strings.HasPrefix(s, "#") {
				return ErrUnsupported
			}
			base := 10
			s = s[1:]
			if strings.HasPrefix(s, "x") {
				base = 16
				s = s[1:]
			}
			if s == "" {
				return ErrInvalid
			}
			for _, r := range s {
				if !(r >= '0' && r <= '9' || base == 16 && (r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F')) {
					return ErrInvalid
				}
			}
			n, e := strconv.ParseUint(s, base, 32)
			if e != nil || !(n == 9 || n == 10 || n == 13 || n >= 0x20 && n <= 0xD7FF || n >= 0xE000 && n <= 0xFFFD || n >= 0x10000 && n <= 0x10FFFF) {
				return ErrInvalid
			}
		}
		i = j
	}
	return nil
}
func (g *xmlGuard) delimited(delimiter string, prefix int) error {
	for {
		r, e := g.rune()
		if e != nil {
			return ErrInvalid
		}
		if len(g.token)+utf8.RuneLen(r) > prefix+maxXMLToken+len(delimiter) {
			return ErrLimit
		}
		g.add(r)
		if bytes.HasSuffix(g.token, []byte(delimiter)) {
			if len(g.token)-prefix-len(delimiter) > maxXMLToken {
				return ErrLimit
			}
			return nil
		}
		// Permit only the delimiter's possible tail beyond the admitted payload.
		if len(g.token) > prefix+maxXMLToken+len(delimiter)-1 {
			return ErrLimit
		}
	}
}
