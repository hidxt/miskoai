package document

import "bytes"

// validateXMLDeclaration receives one completely bounded PI token. Reserved
// xml targets are declarations only in the first lexical position after the
// optional initial BOM. Ordinary processing instructions remain inert.
func validateXMLDeclaration(token []byte, first bool) error {
	b := token[2 : len(token)-2]
	n := 0
	for n < len(b) && !xmlDeclarationSpace(b[n]) {
		n++
	}
	if !bytes.EqualFold(b[:n], []byte("xml")) {
		return nil
	}
	if !first || !bytes.Equal(b[:n], []byte("xml")) {
		return ErrInvalid
	}
	return validateXMLDeclarationBody(b[n:], true)
}

// The lexical body starts immediately after the target and must begin with
// XML whitespace. encoding/xml's parsed Inst has that whitespace stripped;
// both paths then apply exactly the same bounded declaration grammar.
func validateXMLDeclarationBody(body []byte, requireLeadingSpace bool) error {
	s := xmlDeclarationScanner{b: body}
	if requireLeadingSpace && !s.space() {
		return ErrInvalid
	}
	if !s.literal("version") {
		return ErrInvalid
	}
	version, ok := s.value()
	if !ok || !xmlVersionNumber(version) {
		return ErrInvalid
	}
	unsupported := !bytes.Equal(version, []byte("1.0"))
	encodingSeen, standaloneSeen := false, false
	for s.at < len(s.b) {
		if !s.space() {
			return ErrInvalid
		}
		if s.at == len(s.b) {
			break
		}
		switch {
		case !encodingSeen && !standaloneSeen && s.literal("encoding"):
			encodingSeen = true
			encoding, ok := s.value()
			if !ok || !xmlEncodingName(encoding) {
				return ErrInvalid
			}
			unsupported = unsupported || !bytes.EqualFold(encoding, []byte("UTF-8"))
		case !standaloneSeen && s.literal("standalone"):
			standaloneSeen = true
			standalone, ok := s.value()
			if !ok || !(bytes.Equal(standalone, []byte("yes")) || bytes.Equal(standalone, []byte("no"))) {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	if unsupported {
		return ErrUnsupported
	}
	return nil
}

type xmlDeclarationScanner struct {
	b  []byte
	at int
}

func xmlDeclarationSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func (s *xmlDeclarationScanner) space() bool {
	start := s.at
	for s.at < len(s.b) && xmlDeclarationSpace(s.b[s.at]) {
		s.at++
	}
	return s.at != start
}

func (s *xmlDeclarationScanner) literal(want string) bool {
	if !bytes.HasPrefix(s.b[s.at:], []byte(want)) {
		return false
	}
	s.at += len(want)
	return true
}

// value accepts only paired quotes around plain ASCII, never references or
// markup. It returns a slice of the admitted token without another allocation.
func (s *xmlDeclarationScanner) value() ([]byte, bool) {
	s.space()
	if s.at == len(s.b) || s.b[s.at] != '=' {
		return nil, false
	}
	s.at++
	s.space()
	if s.at == len(s.b) || s.b[s.at] != '\'' && s.b[s.at] != '"' {
		return nil, false
	}
	quote := s.b[s.at]
	s.at++
	start := s.at
	for s.at < len(s.b) && s.b[s.at] != quote {
		c := s.b[s.at]
		if c < 0x20 || c > 0x7e || c == '&' || c == '<' || c == '>' || c == '\'' || c == '"' {
			return nil, false
		}
		s.at++
	}
	if s.at == len(s.b) {
		return nil, false
	}
	v := s.b[start:s.at]
	s.at++
	return v, true
}

func xmlVersionNumber(v []byte) bool {
	// Well-formed numeric versions outside 1.0 are unsupported. Empty values
	// and nonnumeric data are malformed instead of another capability.
	dot := false
	for i, c := range v {
		if c == '.' && !dot && i > 0 && i < len(v)-1 {
			dot = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return dot
}

func xmlEncodingName(v []byte) bool {
	for i, c := range v {
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (i == 0 || !(c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-')) {
			return false
		}
	}
	return len(v) > 0
}
