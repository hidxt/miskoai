// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdfsafe

import "io"

type stack struct {
	values []value
	b      *budget
}

func (s *stack) len() int { return len(s.values) }
func (s *stack) push(v value) {
	s.b.charge(1)
	if len(s.values) >= maxOperands {
		fail(errLimit)
	}
	s.b.retain(32)
	if len(s.values) == cap(s.values) {
		n := cap(s.values) * 2
		if n == 0 {
			n = 1
		}
		if n > maxOperands {
			n = maxOperands
		}
		s.b.retain(int64(n) * 32)
		s.b.charge(int64(len(s.values)))
		next := make([]value, len(s.values), n)
		copy(next, s.values)
		s.values = next
	}
	s.values = append(s.values, v)
}
func (s *stack) pop() value {
	s.b.charge(1)
	n := len(s.values)
	if n == 0 {
		fail(errMalformed)
	}
	v := s.values[n-1]
	s.values[n-1] = value{}
	s.values = s.values[:n-1]
	return v
}

// interpret implements only dictionary/operand mechanics needed by embedded
// PDF content and CMaps. It never recovers malformed objects or safe faults.
func interpret(strm value, do func(stk *stack, op string)) {
	if strm.r == nil {
		fail(errMalformed)
	}
	r := strm.r
	b := r.b
	stk := stack{b: b}
	var dicts []dict
	var opened []*streamReader
	defer func() {
		pending := recover()
		var cleanup error
		for _, s := range opened {
			if s.Close() != nil {
				cleanup = errMalformed
			}
		}
		if pending != nil {
			panic(pending)
		}
		if cleanup != nil {
			fail(cleanup)
		}
	}()
	var rd io.Reader
	if strm.kind() == arrayKind {
		n := strm.len()
		if n > maxStreams {
			fail(errLimit)
		}
		b.retain(int64(n) * 24)
		readers := make([]io.Reader, 0, n)
		opened = make([]*streamReader, 0, n)
		for i := 0; i < n; i++ {
			s := strm.index(i).reader()
			opened = append(opened, s)
			readers = append(readers, s)
		}
		rd = io.MultiReader(readers...)
	} else {
		b.retain(8)
		s := strm.reader()
		opened = append(opened, s)
		rd = s
	}
	buf := newBuffer(rd, 0, b)
	buf.allowEOF = true
	buf.allowObjptr = false
	buf.allowStream = false
Reading:
	for {
		b.charge(1)
		tok := buf.readToken()
		if tok == io.EOF {
			break
		}
		if kw, ok := tok.(keyword); ok {
			switch kw {
			case "null", "[", "]", "<<", ">>":
			case "dict":
				count := stk.pop()
				if count.kind() != integerKind {
					fail(errMalformed)
				}
				boundedCount(count.data)
				b.retain(48)
				stk.push(value{r, objptr{}, make(dict)})
				continue
			case "currentdict":
				if len(dicts) == 0 {
					fail(errMalformed)
				}
				stk.push(value{r, objptr{}, dicts[len(dicts)-1]})
				continue
			case "begin":
				d := stk.pop()
				if d.kind() != dictKind {
					fail(errMalformed)
				}
				if len(dicts) >= maxDictScopes {
					fail(errLimit)
				}
				b.retain(8)
				if len(dicts) == cap(dicts) {
					n := cap(dicts) * 2
					if n == 0 {
						n = 1
					}
					if n > maxDictScopes {
						n = maxDictScopes
					}
					b.retain(int64(n) * 8)
					b.charge(int64(len(dicts)))
					next := make([]dict, len(dicts), n)
					copy(next, dicts)
					dicts = next
				}
				dicts = append(dicts, d.data.(dict))
				continue
			case "end":
				if len(dicts) == 0 {
					fail(errMalformed)
				}
				dicts = dicts[:len(dicts)-1]
				continue
			case "def":
				v := stk.pop()
				k := stk.pop()
				if len(dicts) == 0 || k.kind() != nameKind {
					fail(errMalformed)
				}
				d := dicts[len(dicts)-1]
				if _, ok := d[name(k.name())]; !ok {
					b.node(1)
					b.retain(32)
				}
				d[name(k.name())] = v.data
				continue
			case "pop":
				stk.pop()
				continue
			default:
				for i := len(dicts) - 1; i >= 0; i-- {
					b.charge(1)
					if v, ok := dicts[i][name(kw)]; ok {
						stk.push(value{r, objptr{}, v})
						continue Reading
					}
				}
				do(&stk, string(kw))
				continue
			}
		}
		buf.unreadToken(tok)
		obj, _ := readObjectRecover(buf)
		stk.push(value{r, objptr{}, obj})
	}
	if len(dicts) != 0 {
		fail(errMalformed)
	}
}

// Kept as a narrow dependent-task seam, without recover-and-continue semantics.
func readObjectRecover(b *buffer) (object, bool) { return b.readObject(), true }
