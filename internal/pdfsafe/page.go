// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// Restricted traversal and source-order text semantics replace upstream layout.
package pdfsafe

import "math"

type fontKey struct {
	ptr    objptr
	scope  int
	direct string
}
type extraction struct {
	r       *reader
	output  []byte
	pages   int
	visited map[objptr]bool
	fonts   map[fontKey]*textFont
}

func newExtraction(r *reader) *extraction {
	r.b.retain(96)
	return &extraction{r: r, visited: make(map[objptr]bool), fonts: make(map[fontKey]*textFont)}
}
func (x *extraction) walkPages(v, parent value, depth int) {
	b := x.r.b
	b.charge(1)
	b.node(1)
	if depth >= maxDepth {
		fail(errLimit)
	}
	if v.kind() != dictKind || v.ptr == (objptr{}) {
		fail(errMalformed)
	}
	if x.visited[v.ptr] {
		fail(errMalformed)
	}
	b.retain(16)
	x.visited[v.ptr] = true
	pv := v.key("Parent")
	if parent.r != nil {
		if pv.kind() != dictKind || pv.ptr != parent.ptr {
			fail(errMalformed)
		}
	} else if !pv.isNull() {
		fail(errMalformed)
	}
	switch v.key("Type").name() {
	case "Pages":
		before := x.pages
		count := v.key("Count")
		if count.kind() != integerKind || count.int64() < 0 {
			fail(errMalformed)
		}
		kids := v.key("Kids")
		if kids.kind() != arrayKind {
			fail(errMalformed)
		}
		for i := 0; i < kids.len(); i++ {
			b.charge(1)
			x.walkPages(kids.index(i), v, depth+1)
		}
		if count.int64() != int64(x.pages-before) {
			fail(errMalformed)
		}
	case "Page":
		if x.pages >= maxPages {
			fail(errLimit)
		}
		x.pages++
		resources := x.inherited(v, "Resources")
		if resources.kind() != dictKind && !resources.isNull() {
			fail(errMalformed)
		}
		contents := v.key("Contents")
		if contents.isNull() {
			return
		}
		if contents.kind() != streamKind && contents.kind() != arrayKind {
			fail(errMalformed)
		}
		if x.pages > 1 && len(x.output) > 0 {
			x.append("\n")
		}
		x.content(contents, resources)
	default:
		fail(errMalformed)
	}
}
func (x *extraction) inherited(v value, k string) value {
	b := x.r.b
	b.retain(48)
	seen := make(map[objptr]bool)
	for depth := 0; !v.isNull(); depth++ {
		b.charge(1)
		if depth >= maxDepth {
			fail(errLimit)
		}
		if v.kind() != dictKind || v.ptr == (objptr{}) || seen[v.ptr] {
			fail(errMalformed)
		}
		b.retain(16)
		b.node(1)
		seen[v.ptr] = true
		a := v.key(k)
		if !a.isNull() {
			return a
		}
		v = v.key("Parent")
	}
	return value{}
}
func numeric(v value) bool {
	if v.kind() != integerKind && v.kind() != realKind {
		return false
	}
	f := v.float64()
	return !math.IsInf(f, 0) && !math.IsNaN(f)
}
func argsCount(args []value, n int) {
	if len(args) != n {
		fail(errMalformed)
	}
}
func numericArgs(args []value, n int) {
	argsCount(args, n)
	for _, a := range args {
		if !numeric(a) {
			fail(errMalformed)
		}
	}
}
func (x *extraction) content(contents, resources value) {
	var font *textFont
	inText := false
	rendering := int64(0)
	type state struct {
		font      *textFont
		rendering int64
	}
	var saved []state
	show := func(v value) {
		if !inText || font == nil || v.kind() != stringKind {
			fail(errMalformed)
		}
		if rendering == 3 {
			return
		}
		font.decode(x, v.rawString())
	}
	interpretContent(contents, func(s *stack, op string) {
		args := s.values
		s.values = nil
		switch op {
		case "BT":
			argsCount(args, 0)
			if inText {
				fail(errMalformed)
			}
			inText = true
		case "ET":
			argsCount(args, 0)
			if !inText {
				fail(errMalformed)
			}
			inText = false
		case "Tf":
			argsCount(args, 2)
			if !inText || args[0].kind() != nameKind || !numeric(args[1]) {
				fail(errMalformed)
			}
			font = x.font(resources, args[0].name())
		case "Tj":
			argsCount(args, 1)
			show(args[0])
		case "'":
			argsCount(args, 1)
			if len(x.output) > 0 {
				x.append("\n")
			}
			show(args[0])
		case "\"":
			argsCount(args, 3)
			if !numeric(args[0]) || !numeric(args[1]) {
				fail(errMalformed)
			}
			if len(x.output) > 0 {
				x.append("\n")
			}
			show(args[2])
		case "TJ":
			argsCount(args, 1)
			if args[0].kind() != arrayKind {
				fail(errMalformed)
			}
			for i := 0; i < args[0].len(); i++ {
				a := args[0].index(i)
				if a.kind() == stringKind {
					show(a)
				} else if !numeric(a) {
					fail(errMalformed)
				}
			}
		case "Td", "TD":
			numericArgs(args, 2)
			if !inText {
				fail(errMalformed)
			}
			if args[1].float64() != 0 && len(x.output) > 0 {
				x.append("\n")
			}
		case "T*":
			argsCount(args, 0)
			if !inText {
				fail(errMalformed)
			}
			if len(x.output) > 0 {
				x.append("\n")
			}
		case "Tm":
			numericArgs(args, 6)
			if !inText {
				fail(errMalformed)
			}
		case "Tc", "Tw", "Tz", "TL", "Ts":
			numericArgs(args, 1)
			if !inText {
				fail(errMalformed)
			}
		case "Tr":
			argsCount(args, 1)
			if !inText || args[0].kind() != integerKind || args[0].int64() < 0 || args[0].int64() > 7 {
				fail(errMalformed)
			}
			rendering = args[0].int64()
		case "q":
			argsCount(args, 0)
			if len(saved) >= maxDepth {
				fail(errLimit)
			}
			x.r.b.retain(16)
			if len(saved) == cap(saved) {
				n := cap(saved) * 2
				if n == 0 {
					n = 1
				}
				if n > maxDepth {
					n = maxDepth
				}
				x.r.b.retain(int64(n) * 16)
				x.r.b.charge(int64(len(saved)))
				next := make([]state, len(saved), n)
				copy(next, saved)
				saved = next
			}
			saved = append(saved, state{font, rendering})
		case "Q":
			argsCount(args, 0)
			if len(saved) == 0 {
				fail(errMalformed)
			}
			v := saved[len(saved)-1]
			saved = saved[:len(saved)-1]
			font = v.font
			rendering = v.rendering
		case "cm":
			numericArgs(args, 6)
		case "w", "J", "j", "M", "i", "G", "g":
			numericArgs(args, 1)
		case "m", "l":
			numericArgs(args, 2)
		case "c":
			numericArgs(args, 6)
		case "v", "y", "re", "K", "k":
			numericArgs(args, 4)
		case "RG", "rg":
			numericArgs(args, 3)
		case "h", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n", "W", "W*":
			argsCount(args, 0)
		case "d":
			argsCount(args, 2)
			if args[0].kind() != arrayKind || !numeric(args[1]) {
				fail(errMalformed)
			}
			for i := 0; i < args[0].len(); i++ {
				if !numeric(args[0].index(i)) {
					fail(errMalformed)
				}
			}
		case "Do": // Images have no text; forms may contain visible text and must refuse.
			argsCount(args, 1)
			if args[0].kind() != nameKind {
				fail(errMalformed)
			}
			v := resources.key("XObject").key(args[0].name())
			if v.kind() != streamKind {
				fail(errMalformed)
			}
			if v.key("Subtype").name() != "Image" {
				fail(errUnsupported)
			}
		default:
			fail(errUnsupported)
		}
	})
	if inText || len(saved) != 0 {
		fail(errMalformed)
	}
}
