// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Reading of PDF tokens and objects from a raw byte stream.

package pdfsafe

import (
	"io"
	"strconv"
)

// A token is a PDF token in the input stream, one of the following Go types:
//
//	bool, a PDF boolean
//	int64, a PDF integer
//	float64, a PDF real
//	string, a PDF string literal
//	keyword, a PDF keyword
//	name, a PDF name without the leading slash
type token interface{}

// A name is a PDF name, without the leading slash.
type name string

// A keyword is a PDF keyword.
// Delimiter tokens used in higher-level syntax,
// such as "<<", ">>", "[", "]", "{", "}", are also treated as keywords.
type keyword string

// maxObjectDepth is the maximum nesting depth for PDF objects (dicts, arrays,
// and indirect object definitions). Malicious files can nest millions of
// "N N obj" tokens to exhaust the Go call stack; this limit turns that into
// a recoverable panic instead of a fatal process crash.
const maxObjectDepth = maxDepth

// A buffer holds buffered input bytes from the PDF file.
type buffer struct {
	budget      *budget
	r           io.Reader // source of data
	buf         []byte    // buffered data
	pos         int       // read index in buf
	offset      int64     // offset at end of buf; aka offset of next read
	tmp         []byte    // scratch space for accumulating token
	unread      []token   // queue of read but then unread tokens
	allowEOF    bool
	allowObjptr bool
	allowStream bool
	eof         bool
	objptr      objptr
}

// newBuffer returns a new buffer reading from r at the given offset.
func newBuffer(r io.Reader, offset int64, budget *budget) *buffer {
	budget.retain(4096)
	return &buffer{
		budget:      budget,
		r:           r,
		offset:      offset,
		buf:         make([]byte, 0, 4096),
		allowObjptr: true,
		allowStream: true,
	}
}

func (b *buffer) readByte() byte {
	b.budget.charge(1)
	if b.eof {
		return '\n'
	}
	if b.pos >= len(b.buf) {
		b.reload()
		if b.pos >= len(b.buf) {
			return '\n'
		}
	}
	c := b.buf[b.pos]
	b.pos++
	return c
}

func (b *buffer) reload() bool {
	n := cap(b.buf) - int(b.offset%int64(cap(b.buf)))
	b.budget.charge(1)
	n, err := b.r.Read(b.buf[:n])
	if n < 0 || n > cap(b.buf) || n == 0 && err == nil {
		fail(errMalformed)
	}
	if n == 0 && err != nil {
		b.buf = b.buf[:0]
		b.pos = 0
		if b.allowEOF && err == io.EOF {
			b.eof = true
			return false
		}
		fail(errMalformed)
		return false
	}
	b.offset += int64(n)
	b.buf = b.buf[:n]
	b.pos = 0
	return true
}

func (b *buffer) seekForward(offset int64) {
	if offset < b.readOffset() || offset > maxDecoded {
		fail(errMalformed)
	}
	for b.offset < offset {
		if !b.reload() {
			fail(errMalformed)
		}
	}
	b.pos = len(b.buf) - int(b.offset-offset)
}

func (b *buffer) readOffset() int64 {
	return b.offset - int64(len(b.buf)) + int64(b.pos)
}

func (b *buffer) unreadByte() {
	if b.pos > 0 {
		b.pos--
	}
}

func (b *buffer) unreadToken(t token) {
	if len(b.unread) >= 4 {
		fail(errLimit)
	}
	b.budget.retain(16)
	b.unread = append(b.unread, t)
}

func (b *buffer) readToken() token {
	b.budget.charge(1)
	if n := len(b.unread); n > 0 {
		t := b.unread[n-1]
		b.unread = b.unread[:n-1]
		return t
	}

	// Find first non-space, non-comment byte.
	c := b.readByte()
	for {
		if isSpace(c) {
			if b.eof {
				return io.EOF
			}
			c = b.readByte()
		} else if c == '%' {
			for c != '\r' && c != '\n' {
				c = b.readByte()
			}
		} else {
			break
		}
	}

	switch c {
	case '<':
		if b.readByte() == '<' {
			return keyword("<<")
		}
		b.unreadByte()
		return b.readHexString()

	case '(':
		return b.readLiteralString()

	case '[', ']', '{', '}':
		return keyword(string(c))

	case '/':
		return b.readName()

	case '>':
		if b.readByte() == '>' {
			return keyword(">>")
		}
		b.unreadByte()
		fallthrough

	default:
		if isDelim(c) {
			fail(errMalformed)
			return nil
		}
		b.unreadByte()
		return b.readKeyword()
	}
}

func (b *buffer) readHexString() token {
	tmp := b.tmp[:0]
	high := -1
	for {
		c := b.readByte()
		if b.eof {
			fail(errMalformed)
		}
		if c == '>' {
			if high >= 0 {
				tmp = b.appendBytes(tmp, byte(high<<4))
			}
			break
		}
		if isSpace(c) {
			continue
		}
		n := unhex(c)
		if n < 0 {
			fail(errMalformed)
		}
		if high < 0 {
			high = n
		} else {
			tmp = b.appendBytes(tmp, byte(high<<4|n))
			high = -1
		}
	}
	b.tmp = tmp
	b.budget.retain(int64(len(tmp)))
	return string(tmp)
}
func (b *buffer) appendBytes(p []byte, x ...byte) []byte {
	if len(x) > maxToken-len(p) {
		fail(errLimit)
	}
	b.budget.retain(int64(len(x)))
	if len(x) > cap(p)-len(p) {
		n := cap(p) * 2
		if n < 16 {
			n = 16
		}
		if n > maxToken {
			n = maxToken
		}
		if n < len(p)+len(x) {
			n = len(p) + len(x)
		}
		b.budget.retain(int64(n))
		b.budget.charge(int64(len(p)))
		next := make([]byte, len(p), n)
		copy(next, p)
		p = next
	}
	return append(p, x...)
}

func unhex(b byte) int {
	switch {
	case '0' <= b && b <= '9':
		return int(b) - '0'
	case 'a' <= b && b <= 'f':
		return int(b) - 'a' + 10
	case 'A' <= b && b <= 'F':
		return int(b) - 'A' + 10
	}
	return -1
}

func (b *buffer) readLiteralString() token {
	tmp := b.tmp[:0]
	depth := 1
Loop:
	for !b.eof {
		c := b.readByte()
		switch c {
		default:
			tmp = b.appendBytes(tmp, c)
		case '(':
			depth++
			if depth > maxDepth {
				fail(errLimit)
			}
			tmp = b.appendBytes(tmp, c)
		case ')':
			if depth--; depth == 0 {
				break Loop
			}
			tmp = b.appendBytes(tmp, c)
		case '\\':
			switch c = b.readByte(); c {
			default:
				fail(errMalformed)
				tmp = b.appendBytes(tmp, '\\', c)
			case 'n':
				tmp = b.appendBytes(tmp, '\n')
			case 'r':
				tmp = b.appendBytes(tmp, '\r')
			case 'b':
				tmp = b.appendBytes(tmp, '\b')
			case 't':
				tmp = b.appendBytes(tmp, '\t')
			case 'f':
				tmp = b.appendBytes(tmp, '\f')
			case '(', ')', '\\':
				tmp = b.appendBytes(tmp, c)
			case '\r':
				if b.readByte() != '\n' {
					b.unreadByte()
				}
				fallthrough
			case '\n':
				// no append
			case '0', '1', '2', '3', '4', '5', '6', '7':
				x := int(c - '0')
				for i := 0; i < 2; i++ {
					c = b.readByte()
					if c < '0' || c > '7' {
						b.unreadByte()
						break
					}
					x = x*8 + int(c-'0')
				}
				if x > 255 {
					fail(errMalformed)
				}
				tmp = b.appendBytes(tmp, byte(x))
			}
		}
	}
	if depth != 0 {
		fail(errMalformed)
	}
	b.tmp = tmp
	b.budget.retain(int64(len(tmp)))
	return string(tmp)
}

func (b *buffer) readName() token {
	tmp := b.tmp[:0]
	for {
		c := b.readByte()
		if isDelim(c) || isSpace(c) {
			b.unreadByte()
			break
		}
		if c == '#' {
			x := unhex(b.readByte())<<4 | unhex(b.readByte())
			if x < 0 {
				fail(errMalformed)
			}
			tmp = b.appendBytes(tmp, byte(x))
			continue
		}
		tmp = b.appendBytes(tmp, c)
	}
	b.tmp = tmp
	b.budget.retain(int64(len(tmp)))
	return name(string(tmp))
}

func (b *buffer) readKeyword() token {
	tmp := b.tmp[:0]
	for {
		c := b.readByte()
		if isDelim(c) || isSpace(c) {
			b.unreadByte()
			break
		}
		tmp = b.appendBytes(tmp, c)
	}
	b.tmp = tmp
	b.budget.retain(int64(len(tmp)))
	s := string(tmp)
	switch {
	case s == "true":
		return true
	case s == "false":
		return false
	case isInteger(s):
		x, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			fail(errMalformed)
		}
		return x
	case isReal(s):
		x, err := strconv.ParseFloat(s, 64)
		if err != nil {
			fail(errMalformed)
		}
		return x
	}
	return keyword(string(tmp))
}

func isInteger(s string) bool {
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || '9' < c {
			return false
		}
	}
	return true
}

func isReal(s string) bool {
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	if len(s) == 0 {
		return false
	}
	ndot := 0
	for _, c := range s {
		if c == '.' {
			ndot++
			continue
		}
		if c < '0' || '9' < c {
			return false
		}
	}
	return ndot == 1
}

// An object is a PDF syntax object, one of the following Go types:
//
//	bool, a PDF boolean
//	int64, a PDF integer
//	float64, a PDF real
//	string, a PDF string literal
//	name, a PDF name without the leading slash
//	dict, a PDF dictionary
//	array, a PDF array
//	stream, a PDF stream
//	objptr, a PDF object reference
//	objdef, a PDF object definition
//
// An object may also be nil, to represent the PDF null.
type object interface{}

type dict map[name]object

type array []object

type stream struct {
	hdr    dict
	ptr    objptr
	offset int64
}

type objptr struct {
	id  uint32
	gen uint16
}

type objdef struct {
	ptr objptr
	obj object
}

func (b *buffer) readObject() object {
	b.budget.enter()
	defer b.budget.leave()
	b.budget.node(1)

	tok := b.readToken()
	if kw, ok := tok.(keyword); ok {
		switch kw {
		case "null":
			return nil
		case "<<":
			return b.readDict()
		case "[":
			return b.readArray()
		case ">>", "]":
			// Owning array/dictionary loops consume their legal closers. A
			// closer where an object value is expected is always malformed.
			fail(errMalformed)
		}
		fail(errMalformed)
		return nil
	}

	if !b.allowObjptr {
		return tok
	}

	if t1, ok := tok.(int64); ok && t1 >= 0 && t1 < maxNodes {
		tok2 := b.readToken()
		if t2, ok := tok2.(int64); ok && t2 >= 0 && t2 <= 65535 {
			tok3 := b.readToken()
			switch tok3 {
			case keyword("R"):
				return objptr{uint32(t1), uint16(t2)}
			case keyword("obj"):
				old := b.objptr
				b.objptr = objptr{uint32(t1), uint16(t2)}
				obj := b.readObject()
				if _, ok := obj.(stream); !ok {
					tok4 := b.readToken()
					if tok4 != keyword("endobj") {
						fail(errMalformed)
						b.unreadToken(tok4)
					}
				}
				b.objptr = old
				return objdef{objptr{uint32(t1), uint16(t2)}, obj}
			}
			b.unreadToken(tok3)
		}
		b.unreadToken(tok2)
	}
	return tok
}

func (b *buffer) readArray() object {
	var x array
	for {
		tok := b.readToken()
		// Break on io.EOF as well (readToken returns io.EOF as a token value
		// once the input is exhausted, and readDict already guards for it):
		// otherwise an array that is never closed, e.g. in a truncated
		// content stream, loops forever appending io.EOF objects and
		// allocates memory without bound.
		if tok == nil || tok == io.EOF {
			fail(errMalformed)
		}
		if tok == keyword("]") {
			break
		}
		b.unreadToken(tok)
		b.budget.node(1)
		b.budget.retain(16)
		if len(x) == cap(x) {
			n := cap(x) * 2
			if n == 0 {
				n = 1
			}
			if n > maxNodes {
				fail(errLimit)
			}
			b.budget.retain(int64(n) * 16)
			b.budget.charge(int64(len(x)))
			next := make(array, len(x), n)
			copy(next, x)
			x = next
		}
		x = append(x, b.readObject())
	}
	return x
}

func (b *buffer) readDict() object {
	b.budget.retain(48)
	x := make(dict)
	for {
		tok := b.readToken()
		if tok == nil {
			fail(errMalformed)
		}
		if tok == keyword(">>") {
			break
		}
		if tok == io.EOF {
			fail(errMalformed)
		}
		n, ok := tok.(name)
		if !ok {

			fail(errMalformed)
			continue
		}
		if _, exists := x[n]; exists {
			fail(errMalformed)
		}
		b.budget.node(1)
		b.budget.retain(32)
		x[n] = b.readObject()
	}

	if !b.allowStream {
		return x
	}

	tok := b.readToken()
	if tok != keyword("stream") {
		b.unreadToken(tok)
		return x
	}

	switch b.readByte() {
	case '\r':
		if b.readByte() != '\n' {
			b.unreadByte()
		}
	case '\n':
		// ok
	default:
		fail(errMalformed)
	}

	return stream{x, b.objptr, b.readOffset()}
}

func isSpace(b byte) bool {
	switch b {
	case '\x00', '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

func isDelim(b byte) bool {
	switch b {
	case '<', '>', '(', ')', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}
