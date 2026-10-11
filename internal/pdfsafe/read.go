// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pdfsafe contains a private, restricted PDF syntax reader.
package pdfsafe

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"io"
)

type reader struct {
	f          io.ReaderAt
	end        int64
	xref       []xref
	trailer    dict
	trailerptr objptr
	b          *budget
}
type xref struct {
	ptr      objptr
	inStream bool
	stream   objptr
	offset   int64
}

func parse(ctx context.Context, input io.ReaderAt, size int64) (r *reader, err error) {
	var b *budget
	defer func() {
		if p := recover(); p != nil {
			r = nil
			if f, ok := p.(fault); ok {
				err = f.err
			} else {
				err = errMalformed
			}
		}
		if b != nil {
			if e := b.closeStreams(); e != nil && err == nil {
				r = nil
				err = e
			}
		}
	}()
	if size < 10 || size > maxInput || input == nil {
		return nil, errLimit
	}
	b = newBudget(ctx)
	b.check()
	f := &checkedReaderAt{input, b, size}
	b.retain(10)
	header := make([]byte, 10)
	if _, e := f.ReadAt(header, 0); e != nil {
		return nil, errMalformed
	}
	if !bytes.HasPrefix(header, []byte("%PDF-1.")) || header[7] < '0' || header[7] > '7' || header[8] != '\r' && header[8] != '\n' {
		return nil, errMalformed
	}
	n := int64(100)
	if size < n {
		n = size
	}
	b.retain(n)
	tail := make([]byte, n)
	if _, e := f.ReadAt(tail, size-n); e != nil {
		return nil, errMalformed
	}
	tail = bytes.TrimRight(tail, "\r\n\t ")
	if !bytes.HasSuffix(tail, []byte("%%EOF")) {
		return nil, errMalformed
	}
	i := findLastLine(tail, "startxref")
	if i < 0 {
		return nil, errMalformed
	}
	r = &reader{f: f, end: size, b: b}
	pos := size - n + int64(i)
	buf := r.bufferAt(pos)
	if buf.readToken() != keyword("startxref") {
		return nil, errMalformed
	}
	off, ok := buf.readToken().(int64)
	if !ok {
		return nil, errMalformed
	}
	r.xref, r.trailerptr, r.trailer = r.readXrefs(off)
	if r.trailer["Encrypt"] != nil {
		return nil, errUnsupported
	}
	return r, nil
}
func (r *reader) bufferAt(off int64) *buffer {
	r.b.charge(1)
	if off < 0 || off >= r.end {
		fail(errMalformed)
	}
	return newBuffer(io.NewSectionReader(r.f, off, r.end-off), off, r.b)
}
func findLastLine(buf []byte, s string) int {
	max := len(buf)
	for {
		i := bytes.LastIndex(buf[:max], []byte(s))
		if i <= 0 || i+len(s) >= len(buf) {
			return -1
		}
		if (buf[i-1] == '\n' || buf[i-1] == '\r') && (buf[i+len(s)] == '\n' || buf[i+len(s)] == '\r') {
			return i
		}
		max = i
	}
}
func boundedCount(x object) int64 {
	n, ok := x.(int64)
	if !ok || n < 0 {
		fail(errMalformed)
	}
	if n > maxNodes {
		fail(errLimit)
	}
	return n
}
func (r *reader) grow(table []xref, n int64) []xref {
	if n < 0 || n > maxNodes {
		fail(errLimit)
	}
	if n <= int64(len(table)) {
		return table
	}
	delta := n - int64(len(table))
	r.b.node(delta)
	// Every replacement allocation is admitted, including the copied prefix.
	r.b.retain(n * 32)
	r.b.charge(int64(len(table)))
	next := make([]xref, int(n))
	copy(next, table)
	return next
}
func (r *reader) readXrefs(off int64) (table []xref, ptr objptr, trailer dict) {
	var latestSize int64
	for {
		r.b.revision(off, r.end)
		buf := r.bufferAt(off)
		tok := buf.readToken()
		var hdr dict
		var current objptr
		if tok == keyword("xref") {
			var sectionEnd int64
			for {
				tok = buf.readToken()
				if tok == keyword("trailer") {
					break
				}
				start, ok := tok.(int64)
				if !ok {
					fail(errMalformed)
				}
				count := boundedCount(buf.readToken())
				if start < 0 || start > maxNodes-count {
					fail(errLimit)
				}
				if trailer != nil && start+count > latestSize {
					fail(errMalformed)
				}
				if start+count > sectionEnd {
					sectionEnd = start + count
				}
				table = r.grow(table, start+count)
				for i := int64(0); i < count; i++ {
					r.b.node(1)
					o, ok1 := buf.readToken().(int64)
					g, ok2 := buf.readToken().(int64)
					a, ok3 := buf.readToken().(keyword)
					if !ok1 || !ok2 || !ok3 || g < 0 || g > 65535 || o < 0 || (a != "f" && a != "n") {
						fail(errMalformed)
					}
					if a == "n" && o >= r.end {
						fail(errMalformed)
					}
					x := start + i
					if table[x].ptr == (objptr{}) {
						if a == "n" {
							table[x] = xref{ptr: objptr{uint32(x), uint16(g)}, offset: o}
						} else {
							table[x] = xref{ptr: objptr{0, 65535}}
						}
					}
				}
			}
			var ok bool
			hdr, ok = buf.readObject().(dict)
			if !ok {
				fail(errMalformed)
			}
			size := boundedCount(hdr["Size"])
			if size == 0 || sectionEnd > size || trailer != nil && size > latestSize {
				fail(errMalformed)
			}
			table = r.grow(table, size)
			if trailer == nil {
				latestSize = size
			}
		} else if _, ok := tok.(int64); ok {
			buf.unreadToken(tok)
			def, ok := buf.readObject().(objdef)
			if !ok {
				fail(errMalformed)
			}
			s, ok := def.obj.(stream)
			if !ok || s.hdr["Type"] != name("XRef") {
				fail(errMalformed)
			}
			hdr = s.hdr
			current = def.ptr
			size := boundedCount(hdr["Size"])
			if size == 0 || trailer != nil && size > latestSize || int64(current.id) >= size {
				fail(errMalformed)
			}
			table = r.grow(table, size)
			r.readXrefStreamData(s, table, size)
			if trailer == nil {
				latestSize = size
			}
		} else {
			fail(errMalformed)
		}
		if trailer == nil {
			trailer = hdr
			ptr = current
		}
		if hdr["Encrypt"] != nil {
			fail(errUnsupported)
		}
		if hdr["XRefStm"] != nil {
			fail(errUnsupported)
		}
		prev := hdr["Prev"]
		if prev == nil {
			break
		}
		var ok bool
		off, ok = prev.(int64)
		if !ok {
			fail(errMalformed)
		}
	}
	return
}
func decodeInt(p []byte) uint64 {
	var n uint64
	for _, c := range p {
		n = n<<8 | uint64(c)
	}
	return n
}
func (r *reader) readXrefStreamData(s stream, table []xref, size int64) {
	widths, ok := s.hdr["W"].(array)
	if !ok || len(widths) != 3 {
		fail(errMalformed)
	}
	var w [3]int
	total := 0
	for i, x := range widths {
		n, ok := x.(int64)
		if !ok || n < 0 || n > 8 {
			fail(errMalformed)
		}
		w[i] = int(n)
		total += w[i]
	}
	if total == 0 {
		fail(errMalformed)
	}
	idx, ok := s.hdr["Index"].(array)
	if !ok {
		if s.hdr["Index"] != nil {
			fail(errMalformed)
		}
		r.b.retain(32)
		idx = array{int64(0), size}
	}
	if len(idx)%2 != 0 {
		fail(errMalformed)
	}
	for i := 0; i < len(idx); i += 2 {
		start, ok := idx[i].(int64)
		n := boundedCount(idx[i+1])
		if !ok || start < 0 || start > size || n > size-start {
			fail(errMalformed)
		}
	}
	data := (value{r, objptr{}, s}).reader()
	defer data.Close()
	r.b.retain(int64(total))
	buf := make([]byte, total)
	for i := 0; i < len(idx); i += 2 {
		start := idx[i].(int64)
		n := idx[i+1].(int64)
		for j := int64(0); j < n; j++ {
			r.b.node(1)
			if _, e := io.ReadFull(data, buf); e != nil {
				fail(errMalformed)
			}
			a := decodeInt(buf[:w[0]])
			if w[0] == 0 {
				a = 1
			}
			v := decodeInt(buf[w[0] : w[0]+w[1]])
			g := decodeInt(buf[w[0]+w[1]:])
			x := start + j
			if table[x].ptr != (objptr{}) {
				continue
			}
			switch a {
			case 0:
				table[x] = xref{ptr: objptr{0, 65535}}
			case 1:
				if v >= uint64(r.end) || g > 65535 {
					fail(errMalformed)
				}
				table[x] = xref{ptr: objptr{uint32(x), uint16(g)}, offset: int64(v)}
			case 2:
				if v >= uint64(size) || g >= maxNodes {
					fail(errMalformed)
				}
				table[x] = xref{ptr: objptr{uint32(x), 0}, inStream: true, stream: objptr{uint32(v), 0}, offset: int64(g)}
			default:
				fail(errMalformed)
			}
		}
	}
	var probe [1]byte
	n, e := data.Read(probe[:])
	if n != 0 || e != io.EOF {
		fail(errMalformed)
	}
	if data.Close() != nil {
		fail(errMalformed)
	}
}

type value struct {
	r    *reader
	ptr  objptr
	data interface{}
}
type valueKind int

const (
	nullKind valueKind = iota
	boolKind
	integerKind
	realKind
	stringKind
	nameKind
	dictKind
	arrayKind
	streamKind
)

func (v value) kind() valueKind {
	switch v.data.(type) {
	case bool:
		return boolKind
	case int64:
		return integerKind
	case float64:
		return realKind
	case string:
		return stringKind
	case name:
		return nameKind
	case dict:
		return dictKind
	case array:
		return arrayKind
	case stream:
		return streamKind
	}
	return nullKind
}
func (v value) isNull() bool  { return v.data == nil }
func (v value) boolean() bool { x, _ := v.data.(bool); return x }
func (v value) int64() int64  { x, _ := v.data.(int64); return x }
func (v value) float64() float64 {
	if x, ok := v.data.(float64); ok {
		return x
	}
	return float64(v.int64())
}
func (v value) rawString() string { x, _ := v.data.(string); return x }
func (v value) name() string      { x, _ := v.data.(name); return string(x) }
func (v value) key(k string) value {
	d, ok := v.data.(dict)
	if !ok {
		s, ok := v.data.(stream)
		if !ok {
			return value{}
		}
		d = s.hdr
	}
	if v.r == nil {
		fail(errMalformed)
	}
	return v.r.resolve(v.ptr, d[name(k)])
}
func (v value) index(i int) value {
	a, ok := v.data.(array)
	if !ok || i < 0 || i >= len(a) {
		return value{}
	}
	return v.r.resolve(v.ptr, a[i])
}
func (v value) len() int              { a, _ := v.data.(array); return len(a) }
func (r *reader) trailerValue() value { return value{r, r.trailerptr, r.trailer} }
func (r *reader) resolve(parent objptr, x interface{}) value {
	r.b.enter()
	defer r.b.leave()
	if ptr, ok := x.(objptr); ok {
		if r.b.objects[ptr] {
			fail(errMalformed)
		}
		r.b.node(1)
		r.b.retain(16)
		r.b.objects[ptr] = true
		defer delete(r.b.objects, ptr)
		if ptr.id >= uint32(len(r.xref)) {
			fail(errMalformed)
		}
		entry := r.xref[ptr.id]
		if entry.ptr != ptr || !entry.inStream && entry.offset == 0 {
			fail(errMalformed)
		}
		if !entry.inStream {
			if r.b.offsets[entry.offset] {
				fail(errMalformed)
			}
			r.b.retain(16)
			r.b.offsets[entry.offset] = true
			defer delete(r.b.offsets, entry.offset)
		}
		if entry.inStream {
			strm := r.resolve(parent, entry.stream)
			r.b.retain(96)
			seen := make(map[objptr]bool)
			seenOffsets := make(map[int64]bool)
			for {
				r.b.charge(1)
				if strm.kind() != streamKind || strm.key("Type").name() != "ObjStm" {
					fail(errMalformed)
				}
				streamOffset := strm.data.(stream).offset
				if seen[strm.ptr] || seenOffsets[streamOffset] {
					fail(errMalformed)
				}
				r.b.node(1)
				if int64(len(seen)) >= maxDepth-r.b.depth {
					fail(errLimit)
				}
				r.b.retain(32)
				seen[strm.ptr] = true
				seenOffsets[streamOffset] = true
				decoded, found := r.objectStream(strm, ptr, entry.offset)
				if found {
					x = decoded
					break
				}
				strm = strm.key("Extends")
				if strm.kind() != streamKind {
					fail(errMalformed)
				}
			}
		} else {
			def, ok := r.bufferAt(entry.offset).readObject().(objdef)
			if !ok || def.ptr != ptr {
				fail(errMalformed)
			}
			x = def.obj
		}
		parent = ptr
		if _, ok := x.(objptr); ok {
			return r.resolve(parent, x)
		}
	}
	switch x.(type) {
	case nil, bool, int64, float64, string, name, dict, array, stream:
		return value{r, parent, x}
	}
	fail(errMalformed)
	return value{}
}

// objectStream validates actual decoder EOF before publishing any object. Each
// object's lexer receives only its declared span, never a neighboring object's
// bytes. All retained data, metadata and lexer allocations share the parse budget.
func (r *reader) objectStream(strm value, wanted objptr, index int64) (object, bool) {
	n := boundedCount(strm.key("N").data)
	first, ok := strm.key("First").data.(int64)
	if n == 0 || !ok || first <= 0 || first > maxDecoded {
		fail(errMalformed)
	}
	rd := strm.reader()
	defer rd.Close()
	r.b.retain(4096)
	scratch := make([]byte, 4096)
	var data []byte
	for {
		r.b.charge(1)
		count, err := rd.Read(scratch)
		if count < 0 || count > len(scratch) || count == 0 && err == nil {
			fail(errMalformed)
		}
		if count > maxDecoded-len(data) {
			fail(errLimit)
		}
		need := len(data) + count
		if need > cap(data) {
			capacity := cap(data) * 2
			if capacity < need {
				capacity = need
			}
			if capacity > maxDecoded {
				capacity = maxDecoded
			}
			r.b.retain(int64(capacity))
			r.b.charge(int64(len(data)))
			grown := make([]byte, len(data), capacity)
			copy(grown, data)
			data = grown
		}
		r.b.charge(int64(count))
		data = append(data, scratch[:count]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			fail(errMalformed)
		}
	}
	if rd.Close() != nil || first >= int64(len(data)) {
		fail(errMalformed)
	}
	type member struct {
		id     uint32
		offset int64
	}
	r.b.node(n)
	r.b.retain(n*32 + 64) // boundedCount admits at most maxNodes before multiplication.
	members := make([]member, int(n))
	ids := make(map[uint32]bool)
	h := newBuffer(bytes.NewReader(data[:int(first)]), 0, r.b)
	h.allowEOF, h.allowObjptr, h.allowStream = true, false, false
	payload := int64(len(data)) - first
	found := false
	for i := range members {
		id, okID := h.readToken().(int64)
		offset, okOffset := h.readToken().(int64)
		if !okID || !okOffset || id <= 0 || id >= maxNodes || offset < 0 || offset >= payload {
			fail(errMalformed)
		}
		if ids[uint32(id)] || i == 0 && offset != 0 || i > 0 && offset <= members[i-1].offset {
			fail(errMalformed)
		}
		if id < int64(len(r.xref)) {
			entry := r.xref[int(id)]
			if entry.inStream && entry.stream == strm.ptr && (entry.ptr != (objptr{uint32(id), 0}) || entry.offset != int64(i)) {
				fail(errMalformed)
			}
		}
		ids[uint32(id)] = true
		members[i] = member{uint32(id), offset}
		if uint32(id) == wanted.id {
			if int64(i) != index {
				fail(errMalformed)
			}
			found = true
		}
	}
	if h.readToken() != io.EOF {
		fail(errMalformed)
	}
	var result object
	for i, m := range members {
		end := payload
		if i+1 < len(members) {
			end = members[i+1].offset
		}
		// first and every offset are bounded by len(data) before addition/casts.
		b := newBuffer(bytes.NewReader(data[int(first+m.offset):int(first+end)]), 0, r.b)
		b.allowEOF, b.allowStream = true, false
		start := b.readToken()
		if start == io.EOF || start == keyword("]") || start == keyword(">>") {
			fail(errMalformed)
		}
		b.unreadToken(start)
		obj := b.readObject()
		r.objectStreamMember(obj)
		if b.readToken() != io.EOF {
			fail(errMalformed)
		}
		if m.id == wanted.id {
			result = obj
		}
	}
	return result, found
}

// Object streams contain ordinary objects, never definitions or nested streams.
func (r *reader) objectStreamMember(obj object) {
	r.b.enter()
	defer r.b.leave()
	switch x := obj.(type) {
	case nil, bool, int64, float64, string, name, objptr:
	case array:
		for _, item := range x {
			r.objectStreamMember(item)
		}
	case dict:
		for _, item := range x {
			r.objectStreamMember(item)
		}
	default:
		fail(errMalformed)
	}
}

// streamReader owns every actual decoder created for this stream. Reads close
// the complete chain on errors or panics; successful callers close it explicitly.
type streamReader struct {
	b        *budget
	r        io.Reader
	closers  []io.Closer
	closed   bool
	closeErr error
	terminal error
}

func (s *streamReader) Read(p []byte) (n int, e error) {
	if s.closed {
		s.b.charge(1)
		if s.terminal != nil {
			return 0, s.terminal
		}
		return 0, errMalformed
	}
	defer func() {
		if x := recover(); x != nil {
			_ = s.Close()
			panic(x)
		}
		if e != nil {
			s.terminal = e
			if ce := s.Close(); ce != nil && e == io.EOF {
				e = ce
				s.terminal = e
			}
		}
	}()
	s.b.charge(1)
	n, e = s.r.Read(p)
	if e != nil && e != io.EOF {
		e = errMalformed
	}
	return
}
func (s *streamReader) Close() error {
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	for i := len(s.closers) - 1; i >= 0; i-- {
		if s.closers[i].Close() != nil {
			s.closeErr = errMalformed
		}
	}
	delete(s.b.streams, s)
	return s.closeErr
}

type decodedReader struct {
	b *budget
	r io.Reader
}

func (d *decodedReader) Read(p []byte) (int, error) {
	d.b.charge(1)
	if len(p) == 0 {
		return 0, nil
	}
	remaining := maxDecoded - d.b.decoded
	if int64(len(p)) > remaining+1 {
		p = p[:remaining+1]
	}
	n, e := d.r.Read(p)
	if n < 0 || n > len(p) {
		fail(errMalformed)
	}
	d.b.charge(int64(n))
	// Inner stages have charged the same cumulative counter during Read.
	if int64(n) > maxDecoded-d.b.decoded {
		fail(errLimit)
	}
	d.b.decoded += int64(n)
	if n == 0 && e == nil {
		fail(errMalformed)
	}
	if e != nil && e != io.EOF {
		fail(errMalformed)
	}
	return n, e
}
func (v value) reader() (result *streamReader) {
	if v.r == nil {
		fail(errMalformed)
	}
	b := v.r.b
	b.charge(1)
	s, ok := v.data.(stream)
	if !ok {
		fail(errMalformed)
	}
	if len(b.streams) >= maxStreams {
		fail(errLimit)
	}
	lengthVal := v.key("Length")
	length, ok := lengthVal.data.(int64)
	if !ok || length < 0 || s.offset < 0 || s.offset > v.r.end || length > v.r.end-s.offset {
		fail(errMalformed)
	}
	b.retain(64)
	result = &streamReader{b: b}
	b.streams[result] = struct{}{}
	defer func() {
		if p := recover(); p != nil {
			_ = result.Close()
			panic(p)
		}
	}()
	rd := io.Reader(io.NewSectionReader(v.r.f, s.offset, length))
	filter := v.key("Filter")
	param := v.key("DecodeParms")
	var filters []value
	switch filter.kind() {
	case nullKind:
	case nameKind:
		b.retain(32)
		filters = []value{filter}
	case arrayKind:
		if filter.len() > maxFilters {
			fail(errLimit)
		}
		b.retain(int64(filter.len()) * 32)
		filters = make([]value, filter.len())
		for i := range filters {
			filters[i] = filter.index(i)
		}
	default:
		fail(errUnsupported)
	}
	if len(filters) == 0 {
		rd = &decodedReader{b, rd}
	}
	if param.kind() == arrayKind && param.len() != len(filters) {
		fail(errMalformed)
	}
	for i, f := range filters {
		p := param
		if param.kind() == arrayKind {
			p = param.index(i)
		} else if len(filters) > 1 && !param.isNull() {
			fail(errMalformed)
		}
		rd = applyFilter(rd, f.name(), p, result)
	}
	result.r = rd
	return
}
func applyFilter(rd io.Reader, n string, p value, chain *streamReader) io.Reader {
	b := chain.b
	b.charge(1)
	if !p.isNull() && p.kind() != dictKind {
		fail(errMalformed)
	}
	switch n {
	case "FlateDecode":
		predictor := int64(1)
		columns := int64(1)
		if !p.isNull() {
			if q := p.key("Predictor"); !q.isNull() {
				if q.kind() != integerKind {
					fail(errMalformed)
				}
				predictor = q.int64()
			}
			if q := p.key("Columns"); !q.isNull() {
				if q.kind() != integerKind {
					fail(errMalformed)
				}
				columns = q.int64()
			}
			for _, k := range []string{"Colors", "BitsPerComponent"} {
				if q := p.key(k); !q.isNull() && q.int64() != map[string]int64{"Colors": 1, "BitsPerComponent": 8}[k] {
					fail(errUnsupported)
				}
			}
		}
		if predictor != 1 && predictor != 12 {
			fail(errUnsupported)
		}
		if predictor == 12 && (columns <= 0 || columns > maxRow-1) {
			fail(errLimit)
		}
		// Reserve both decoder ownership and bounded standard-library scratch
		// before construction, which may read/decompress buffered input.
		if len(chain.closers) >= maxFilters {
			fail(errLimit)
		}
		b.retain(64 << 10)
		zr, e := zlib.NewReader(rd)
		if e != nil {
			fail(errMalformed)
		}
		chain.closers = append(chain.closers, zr)
		var out io.Reader = &decodedReader{b, zr}
		if predictor == 12 {
			row := int(columns + 1)
			b.retain(int64(row) * 2)
			out = &decodedReader{b, &pngUpReader{r: out, hist: make([]byte, row), tmp: make([]byte, row)}}
		}
		return out
	case "ASCII85Decode":
		if !p.isNull() && len(p.data.(dict)) > 0 {
			fail(errUnsupported)
		}
		b.retain(4096)
		return &decodedReader{b, ascii85.NewDecoder(newAlphaReader(rd, b))}
	default:
		fail(errUnsupported)
	}
	return nil
}

type pngUpReader struct {
	r               io.Reader
	hist, tmp, pend []byte
}

func (r *pngUpReader) Read(p []byte) (n int, err error) {
	for len(p) > 0 {
		if len(r.pend) > 0 {
			m := copy(p, r.pend)
			n += m
			p = p[m:]
			r.pend = r.pend[m:]
			continue
		}
		got, e := io.ReadFull(r.r, r.tmp)
		if e != nil {
			if e == io.EOF && got == 0 {
				return n, io.EOF
			}
			return n, errMalformed
		}
		if r.tmp[0] != 2 {
			return n, errMalformed
		}
		for i := 1; i < len(r.tmp); i++ {
			r.hist[i] += r.tmp[i]
		}
		r.pend = r.hist[1:]
	}
	return n, nil
}
