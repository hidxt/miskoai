// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// Encoding tables are retained from the verified pin; CMap/Unicode decoding is
// replaced with strict, cumulative-budget parsing.
package pdfsafe

import (
	"io"
	"unicode/utf8"
)

type codeSpace struct {
	lo, hi uint32
	n      int
}
type textFont struct {
	encoding string
	mappings map[string]string
	spaces   []codeSpace
}

func (x *extraction) font(resources value, n string) *textFont {
	fonts := resources.key("Font")
	if fonts.kind() != dictKind {
		fail(errUnsupported)
	}
	raw := fonts.data.(dict)[name(n)]
	v := fonts.key(n)
	if v.kind() != dictKind || v.key("Type").name() != "Font" {
		fail(errUnsupported)
	}
	key := fontKey{ptr: v.ptr}
	if _, ok := raw.(objptr); !ok {
		key.scope = x.pages
		key.direct = n
		key.ptr = objptr{}
	}
	if f := x.fonts[key]; f != nil {
		return f
	}
	b := x.r.b
	b.node(1)
	b.retain(96)
	f := &textFont{}
	subtype := v.key("Subtype").name()
	if subtype != "Type0" && subtype != "Type1" && subtype != "TrueType" {
		fail(errUnsupported)
	}
	if subtype == "Type0" {
		descendants := v.key("DescendantFonts")
		if descendants.kind() != arrayKind || descendants.len() != 1 {
			fail(errMalformed)
		}
		d := descendants.index(0)
		if d.kind() != dictKind || d.key("Type").name() != "Font" {
			fail(errMalformed)
		}
		sub := d.key("Subtype").name()
		if sub != "CIDFontType0" && sub != "CIDFontType2" {
			fail(errUnsupported)
		}
		if d.key("BaseFont").kind() != nameKind {
			fail(errMalformed)
		}
		info := d.key("CIDSystemInfo")
		if info.kind() != dictKind || info.key("Registry").kind() != stringKind || info.key("Ordering").kind() != stringKind {
			fail(errMalformed)
		}
		supplement := info.key("Supplement")
		if supplement.kind() != integerKind || supplement.int64() < 0 {
			fail(errMalformed)
		}
	}
	enc := v.key("Encoding")
	if enc.kind() == dictKind {
		if !enc.key("Differences").isNull() {
			fail(errUnsupported)
		}
		enc = enc.key("BaseEncoding")
	}
	cmap := v.key("ToUnicode")
	if !cmap.isNull() {
		if cmap.kind() != streamKind {
			fail(errMalformed)
		}
		f = parseCMap(cmap)
	} else if subtype == "Type0" {
		if enc.kind() != nameKind || enc.name() != "UniGB-UCS2-H" {
			fail(errUnsupported)
		}
		f.encoding = "UniGB-UCS2-H"
	} else {
		if enc.isNull() {
			base := v.key("BaseFont").name()
			switch base {
			case "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique", "Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic", "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique":
				f.encoding = "StandardEncoding"
			default:
				fail(errUnsupported)
			}
		} else if enc.kind() == nameKind {
			switch enc.name() {
			case "WinAnsiEncoding", "MacRomanEncoding", "StandardEncoding":
				f.encoding = enc.name()
			default:
				fail(errUnsupported)
			}
		} else {
			fail(errUnsupported)
		}
	}
	x.fonts[key] = f
	return f
}
func (f *textFont) decode(x *extraction, s string) {
	b := x.r.b
	if f.mappings != nil {
		for len(s) > 0 {
			b.charge(1)
			n := 0
			for _, space := range f.spaces {
				b.charge(1)
				if len(s) >= space.n {
					code := codeNumber(s[:space.n])
					if code >= space.lo && code <= space.hi {
						if n != 0 {
							fail(errMalformed)
						}
						n = space.n
					}
				}
			}
			if n == 0 {
				fail(errMalformed)
			}
			mapped, ok := f.mappings[s[:n]]
			if !ok {
				fail(errMalformed)
			}
			x.append(mapped)
			s = s[n:]
		}
		return
	}
	if f.encoding == "UniGB-UCS2-H" {
		decodeUTF16(b, s, x.append)
		return
	}
	for i := 0; i < len(s); i++ {
		b.charge(1)
		c := s[i]
		var r rune
		switch f.encoding {
		case "WinAnsiEncoding":
			r = winAnsiEncoding[c]
		case "MacRomanEncoding":
			r = macRomanEncoding[c]
		case "StandardEncoding":
			r = standardEncoding(c)
		default:
			fail(errUnsupported)
		}
		if r == noRune || !utf8.ValidRune(r) {
			fail(errUnsupported)
		}
		b.retain(int64(utf8.RuneLen(r)))
		x.append(string(r))
	}
}

// StandardEncoding maps the Adobe standard byte encoding, not PDFDocEncoding.
// Unsupported glyph bytes refuse rather than using the excluded name.go table.
func standardEncoding(c byte) rune {
	if c >= 32 && c <= 126 {
		switch c {
		case 39:
			return '\u2019'
		case 96:
			return '\u2018'
		}
		return rune(c)
	}
	switch c {
	case 161:
		return 0xa1
	case 162:
		return 0xa2
	case 163:
		return 0xa3
	case 164:
		return 0x2044
	case 165:
		return 0xa5
	case 166:
		return 0x192
	case 167:
		return 0xa7
	case 168:
		return 0xa4
	case 169:
		return 0x27
	case 170:
		return 0x201c
	case 171:
		return 0xab
	case 172:
		return 0x2039
	case 173:
		return 0x203a
	case 174:
		return 0xfb01
	case 175:
		return 0xfb02
	case 177:
		return 0x2013
	case 178:
		return 0x2020
	case 179:
		return 0x2021
	case 180:
		return 0xb7
	case 182:
		return 0xb6
	case 183:
		return 0x2022
	case 184:
		return 0x201a
	case 185:
		return 0x201e
	case 186:
		return 0x201d
	case 187:
		return 0xbb
	case 188:
		return 0x2026
	case 189:
		return 0x2030
	case 191:
		return 0xbf
	case 193:
		return 0x60
	case 194:
		return 0xb4
	case 195:
		return 0x2c6
	case 196:
		return 0x2dc
	case 197:
		return 0xaf
	case 198:
		return 0x2d8
	case 199:
		return 0x2d9
	case 200:
		return 0xa8
	case 202:
		return 0x2da
	case 203:
		return 0xb8
	case 205:
		return 0x2dd
	case 206:
		return 0x2db
	case 207:
		return 0x2c7
	case 208:
		return 0x2014
	case 225:
		return 0xc6
	case 227:
		return 0xaa
	case 232:
		return 0x141
	case 233:
		return 0xd8
	case 234:
		return 0x152
	case 235:
		return 0xba
	case 241:
		return 0xe6
	case 245:
		return 0x131
	case 248:
		return 0x142
	case 249:
		return 0xf8
	case 250:
		return 0x153
	case 251:
		return 0xdf
	}
	return noRune
}
func decodeUTF16(b *budget, s string, emit func(string)) {
	if len(s)%2 != 0 {
		fail(errMalformed)
	}
	for i := 0; i < len(s); i += 2 {
		b.charge(1)
		r := rune(s[i])<<8 | rune(s[i+1])
		if r >= 0xd800 && r <= 0xdbff {
			if i+3 >= len(s) {
				fail(errMalformed)
			}
			low := rune(s[i+2])<<8 | rune(s[i+3])
			if low < 0xdc00 || low > 0xdfff {
				fail(errMalformed)
			}
			r = 0x10000 + (r-0xd800)*1024 + low - 0xdc00
			i += 2
		} else if r >= 0xdc00 && r <= 0xdfff {
			fail(errMalformed)
		}
		if !utf8.ValidRune(r) {
			fail(errMalformed)
		}
		b.retain(int64(utf8.RuneLen(r)))
		emit(string(r))
	}
}
func codeNumber(s string) uint32 {
	var n uint32
	for i := 0; i < len(s); i++ {
		n = n<<8 | uint32(s[i])
	}
	return n
}
func codeString(b *budget, n uint32, size int) string {
	b.retain(int64(size))
	b.charge(int64(size))
	var p [4]byte
	for i := size - 1; i >= 0; i-- {
		p[i] = byte(n & 255)
		n >>= 8
	}
	return string(p[:size])
}
func cmapCode(o object) string {
	s, ok := o.(string)
	if !ok || len(s) < 1 || len(s) > maxCodeBytes {
		fail(errMalformed)
	}
	return s
}
func (f *textFont) addSpace(b *budget, lo, hi string) {
	b.cmapSpaceRoom(1)
	if len(lo) != len(hi) {
		fail(errMalformed)
	}
	s := codeSpace{codeNumber(lo), codeNumber(hi), len(lo)}
	if s.lo > s.hi {
		fail(errMalformed)
	}
	for _, a := range f.spaces {
		b.charge(1)
		aa, zz := a.lo, a.hi
		ss, tt := s.lo, s.hi
		if a.n > s.n {
			shift := uint(8 * (a.n - s.n))
			aa >>= shift
			zz >>= shift
		} else if s.n > a.n {
			shift := uint(8 * (s.n - a.n))
			ss >>= shift
			tt >>= shift
		}
		if aa <= tt && ss <= zz {
			fail(errMalformed)
		}
	}
	if len(f.spaces) >= maxCMapEntries {
		fail(errLimit)
	}
	b.retain(16)
	if len(f.spaces) == cap(f.spaces) {
		n := cap(f.spaces) * 2
		if n == 0 {
			n = 1
		}
		if n > maxCMapEntries {
			n = maxCMapEntries
		}
		b.retain(int64(n) * 16)
		b.charge(int64(len(f.spaces)))
		next := make([]codeSpace, len(f.spaces), n)
		copy(next, f.spaces)
		f.spaces = next
	}
	b.cmapSpaces++
	f.spaces = append(f.spaces, s)
}
func (f *textFont) validCode(b *budget, s string) bool {
	n := codeNumber(s)
	for _, a := range f.spaces {
		b.charge(1)
		if len(s) == a.n && n >= a.lo && n <= a.hi {
			return true
		}
	}
	return false
}
func (f *textFont) addMapping(b *budget, code, replacement string) {
	b.cmapMappingRoom(1)
	if !f.validCode(b, code) {
		fail(errMalformed)
	}
	if len(replacement) > maxReplacementBytes {
		fail(errLimit)
	}
	if len(replacement) == 0 || len(replacement)%2 != 0 {
		fail(errMalformed)
	}
	if len(f.mappings) >= maxCMapEntries {
		fail(errLimit)
	}
	if _, ok := f.mappings[code]; ok {
		fail(errMalformed)
	}
	b.node(1)
	b.retain(int64(len(code)) + int64(len(replacement))*4 + 64)
	b.cmapMappings++
	out := make([]byte, 0, len(replacement)*2)
	decodeUTF16(b, replacement, func(s string) { out = append(out, s...) })
	f.mappings[code] = string(out)
}
func parseCMap(v value) *textFont {
	b := v.r.b
	b.retain(64)
	f := &textFont{mappings: make(map[string]string)}
	rd := v.reader()
	defer rd.Close()
	buf := newBuffer(rd, 0, b)
	buf.allowEOF = true
	buf.allowObjptr = false
	buf.allowStream = false
	var operands []object
	push := func(o object) {
		if len(operands) >= maxOperands {
			fail(errLimit)
		}
		b.retain(16)
		if len(operands) == cap(operands) {
			n := cap(operands) * 2
			if n == 0 {
				n = 1
			}
			if n > maxOperands {
				n = maxOperands
			}
			b.retain(int64(n) * 16)
			b.charge(int64(len(operands)))
			next := make([]object, len(operands), n)
			copy(next, operands)
			operands = next
		}
		operands = append(operands, o)
	}
	begun, ended := false, false
	depth := 0
	read := func() object { return buf.readObject() }
	for {
		b.charge(1)
		tok := buf.readToken()
		if tok == io.EOF {
			break
		}
		kw, ok := tok.(keyword)
		if !ok {
			buf.unreadToken(tok)
			if len(operands) >= maxOperands {
				fail(errLimit)
			}
			push(read())
			continue
		}
		switch kw {
		case "<<", "[", "null":
			buf.unreadToken(tok)
			if len(operands) >= maxOperands {
				fail(errLimit)
			}
			push(read())
			continue
		case "begincmap":
			if begun || ended || len(operands) != 0 {
				fail(errMalformed)
			}
			begun = true
		case "endcmap":
			if !begun || ended || len(operands) != 0 {
				fail(errMalformed)
			}
			ended = true
		case "begincodespacerange", "beginbfchar", "beginbfrange":
			if !begun || ended || len(operands) != 1 {
				fail(errMalformed)
			}
			count, ok := operands[0].(int64)
			if !ok || count < 0 {
				fail(errMalformed)
			}
			if count > maxCMapEntries {
				fail(errLimit)
			}
			// A bfchar block declares its actual mapping count. A bfrange
			// block counts ranges; each range's expansion is checked below.
			if kw == "beginbfchar" {
				b.cmapMappingRoom(uint64(count))
			}
			if kw == "begincodespacerange" {
				b.cmapSpaceRoom(uint64(count))
			}
			operands = nil
			for i := int64(0); i < count; i++ {
				b.charge(1)
				lo := cmapCode(read())
				switch kw {
				case "begincodespacerange":
					f.addSpace(b, lo, cmapCode(read()))
				case "beginbfchar":
					replacement, ok := read().(string)
					if !ok {
						fail(errMalformed)
					}
					f.addMapping(b, lo, replacement)
				case "beginbfrange":
					hi := cmapCode(read())
					if len(lo) != len(hi) || codeNumber(lo) > codeNumber(hi) {
						fail(errMalformed)
					}
					n := uint64(codeNumber(hi)) - uint64(codeNumber(lo)) + 1
					b.cmapMappingRoom(n)
					if n > uint64(maxCMapEntries-len(f.mappings)) {
						fail(errLimit)
					}
					replacement := read()
					switch dest := replacement.(type) {
					case string:
						if len(dest) == 0 || len(dest)%2 != 0 {
							fail(errMalformed)
						}
						if len(dest) > maxReplacementBytes {
							fail(errLimit)
						}
						b.retain(int64(len(dest)))
						next := []byte(dest)
						for j := uint64(0); j < n; j++ {
							b.charge(1)
							b.retain(int64(len(next)))
							f.addMapping(b, codeString(b, codeNumber(lo)+uint32(j), len(lo)), string(next))
							if j+1 < n {
								carry := true
								for k := len(next) - 1; k >= 0 && carry; k-- {
									b.charge(1)
									next[k]++
									carry = next[k] == 0
								}
								if carry {
									fail(errMalformed)
								}
							}
						}
					case array:
						if uint64(len(dest)) != n {
							fail(errMalformed)
						}
						for j, item := range dest {
							b.charge(1)
							s, ok := item.(string)
							if !ok {
								fail(errMalformed)
							}
							f.addMapping(b, codeString(b, codeNumber(lo)+uint32(j), len(lo)), s)
						}
					default:
						fail(errMalformed)
					}
				}
			}
			ending := keyword("endcodespacerange")
			if kw == "beginbfchar" {
				ending = "endbfchar"
			}
			if kw == "beginbfrange" {
				ending = "endbfrange"
			}
			if buf.readToken() != ending {
				fail(errMalformed)
			}
		case "def":
			if len(operands) != 2 {
				fail(errMalformed)
			}
			k, ok := operands[0].(name)
			if !ok {
				fail(errMalformed)
			}
			switch k {
			case "CIDSystemInfo":
				if _, ok := operands[1].(dict); !ok {
					fail(errMalformed)
				}
			case "CMapName":
				if _, ok := operands[1].(name); !ok {
					fail(errMalformed)
				}
			case "CMapType":
				if operands[1] != int64(2) {
					fail(errUnsupported)
				}
			case "WMode":
				if operands[1] != int64(0) {
					fail(errUnsupported)
				}
			default:
				fail(errUnsupported)
			}
		case "findresource":
			if len(operands) != 2 || operands[0] != name("CIDInit") || operands[1] != name("ProcSet") {
				fail(errUnsupported)
			}
		case "dict":
			if len(operands) != 1 {
				fail(errMalformed)
			}
			n, ok := operands[0].(int64)
			if !ok || n < 0 || n > maxNodes {
				fail(errMalformed)
			}
		case "begin":
			if len(operands) != 0 || depth >= maxDictScopes {
				fail(errMalformed)
			}
			depth++
		case "end":
			if len(operands) != 0 || depth == 0 {
				fail(errMalformed)
			}
			depth--
		case "CMapName", "currentdict":
			if !ended {
				fail(errMalformed)
			}
		case "defineresource":
			if !ended || len(operands) != 1 || operands[0] != name("CMap") {
				fail(errMalformed)
			}
		case "pop":
			if !ended || len(operands) != 0 {
				fail(errMalformed)
			}
		default:
			fail(errUnsupported)
		}
		operands = nil
	}
	if !begun || !ended || depth != 0 || len(operands) != 0 || len(f.spaces) == 0 {
		fail(errMalformed)
	}
	return f
}

const noRune = rune(0xfffd)

var winAnsiEncoding = [256]rune{
	0x0000, 0x0001, 0x0002, 0x0003, 0x0004, 0x0005, 0x0006, 0x0007,
	0x0008, 0x0009, 0x000a, 0x000b, 0x000c, 0x000d, 0x000e, 0x000f,
	0x0010, 0x0011, 0x0012, 0x0013, 0x0014, 0x0015, 0x0016, 0x0017,
	0x0018, 0x0019, 0x001a, 0x001b, 0x001c, 0x001d, 0x001e, 0x001f,
	0x0020, 0x0021, 0x0022, 0x0023, 0x0024, 0x0025, 0x0026, 0x0027,
	0x0028, 0x0029, 0x002a, 0x002b, 0x002c, 0x002d, 0x002e, 0x002f,
	0x0030, 0x0031, 0x0032, 0x0033, 0x0034, 0x0035, 0x0036, 0x0037,
	0x0038, 0x0039, 0x003a, 0x003b, 0x003c, 0x003d, 0x003e, 0x003f,
	0x0040, 0x0041, 0x0042, 0x0043, 0x0044, 0x0045, 0x0046, 0x0047,
	0x0048, 0x0049, 0x004a, 0x004b, 0x004c, 0x004d, 0x004e, 0x004f,
	0x0050, 0x0051, 0x0052, 0x0053, 0x0054, 0x0055, 0x0056, 0x0057,
	0x0058, 0x0059, 0x005a, 0x005b, 0x005c, 0x005d, 0x005e, 0x005f,
	0x0060, 0x0061, 0x0062, 0x0063, 0x0064, 0x0065, 0x0066, 0x0067,
	0x0068, 0x0069, 0x006a, 0x006b, 0x006c, 0x006d, 0x006e, 0x006f,
	0x0070, 0x0071, 0x0072, 0x0073, 0x0074, 0x0075, 0x0076, 0x0077,
	0x0078, 0x0079, 0x007a, 0x007b, 0x007c, 0x007d, 0x007e, 0x007f,
	0x20ac, noRune, 0x201a, 0x0192, 0x201e, 0x2026, 0x2020, 0x2021,
	0x02c6, 0x2030, 0x0160, 0x2039, 0x0152, noRune, 0x017d, noRune,
	noRune, 0x2018, 0x2019, 0x201c, 0x201d, 0x2022, 0x2013, 0x2014,
	0x02dc, 0x2122, 0x0161, 0x203a, 0x0153, noRune, 0x017e, 0x0178,
	0x00a0, 0x00a1, 0x00a2, 0x00a3, 0x00a4, 0x00a5, 0x00a6, 0x00a7,
	0x00a8, 0x00a9, 0x00aa, 0x00ab, 0x00ac, 0x00ad, 0x00ae, 0x00af,
	0x00b0, 0x00b1, 0x00b2, 0x00b3, 0x00b4, 0x00b5, 0x00b6, 0x00b7,
	0x00b8, 0x00b9, 0x00ba, 0x00bb, 0x00bc, 0x00bd, 0x00be, 0x00bf,
	0x00c0, 0x00c1, 0x00c2, 0x00c3, 0x00c4, 0x00c5, 0x00c6, 0x00c7,
	0x00c8, 0x00c9, 0x00ca, 0x00cb, 0x00cc, 0x00cd, 0x00ce, 0x00cf,
	0x00d0, 0x00d1, 0x00d2, 0x00d3, 0x00d4, 0x00d5, 0x00d6, 0x00d7,
	0x00d8, 0x00d9, 0x00da, 0x00db, 0x00dc, 0x00dd, 0x00de, 0x00df,
	0x00e0, 0x00e1, 0x00e2, 0x00e3, 0x00e4, 0x00e5, 0x00e6, 0x00e7,
	0x00e8, 0x00e9, 0x00ea, 0x00eb, 0x00ec, 0x00ed, 0x00ee, 0x00ef,
	0x00f0, 0x00f1, 0x00f2, 0x00f3, 0x00f4, 0x00f5, 0x00f6, 0x00f7,
	0x00f8, 0x00f9, 0x00fa, 0x00fb, 0x00fc, 0x00fd, 0x00fe, 0x00ff,
}

var macRomanEncoding = [256]rune{
	0x0000, 0x0001, 0x0002, 0x0003, 0x0004, 0x0005, 0x0006, 0x0007,
	0x0008, 0x0009, 0x000a, 0x000b, 0x000c, 0x000d, 0x000e, 0x000f,
	0x0010, 0x0011, 0x0012, 0x0013, 0x0014, 0x0015, 0x0016, 0x0017,
	0x0018, 0x0019, 0x001a, 0x001b, 0x001c, 0x001d, 0x001e, 0x001f,
	0x0020, 0x0021, 0x0022, 0x0023, 0x0024, 0x0025, 0x0026, 0x0027,
	0x0028, 0x0029, 0x002a, 0x002b, 0x002c, 0x002d, 0x002e, 0x002f,
	0x0030, 0x0031, 0x0032, 0x0033, 0x0034, 0x0035, 0x0036, 0x0037,
	0x0038, 0x0039, 0x003a, 0x003b, 0x003c, 0x003d, 0x003e, 0x003f,
	0x0040, 0x0041, 0x0042, 0x0043, 0x0044, 0x0045, 0x0046, 0x0047,
	0x0048, 0x0049, 0x004a, 0x004b, 0x004c, 0x004d, 0x004e, 0x004f,
	0x0050, 0x0051, 0x0052, 0x0053, 0x0054, 0x0055, 0x0056, 0x0057,
	0x0058, 0x0059, 0x005a, 0x005b, 0x005c, 0x005d, 0x005e, 0x005f,
	0x0060, 0x0061, 0x0062, 0x0063, 0x0064, 0x0065, 0x0066, 0x0067,
	0x0068, 0x0069, 0x006a, 0x006b, 0x006c, 0x006d, 0x006e, 0x006f,
	0x0070, 0x0071, 0x0072, 0x0073, 0x0074, 0x0075, 0x0076, 0x0077,
	0x0078, 0x0079, 0x007a, 0x007b, 0x007c, 0x007d, 0x007e, 0x007f,
	0x00c4, 0x00c5, 0x00c7, 0x00c9, 0x00d1, 0x00d6, 0x00dc, 0x00e1,
	0x00e0, 0x00e2, 0x00e4, 0x00e3, 0x00e5, 0x00e7, 0x00e9, 0x00e8,
	0x00ea, 0x00eb, 0x00ed, 0x00ec, 0x00ee, 0x00ef, 0x00f1, 0x00f3,
	0x00f2, 0x00f4, 0x00f6, 0x00f5, 0x00fa, 0x00f9, 0x00fb, 0x00fc,
	0x2020, 0x00b0, 0x00a2, 0x00a3, 0x00a7, 0x2022, 0x00b6, 0x00df,
	0x00ae, 0x00a9, 0x2122, 0x00b4, 0x00a8, 0x2260, 0x00c6, 0x00d8,
	0x221e, 0x00b1, 0x2264, 0x2265, 0x00a5, 0x00b5, 0x2202, 0x2211,
	0x220f, 0x03c0, 0x222b, 0x00aa, 0x00ba, 0x03a9, 0x00e6, 0x00f8,
	0x00bf, 0x00a1, 0x00ac, 0x221a, 0x0192, 0x2248, 0x2206, 0x00ab,
	0x00bb, 0x2026, 0x00a0, 0x00c0, 0x00c3, 0x00d5, 0x0152, 0x0153,
	0x2013, 0x2014, 0x201c, 0x201d, 0x2018, 0x2019, 0x00f7, 0x25ca,
	0x00ff, 0x0178, 0x2044, 0x20ac, 0x2039, 0x203a, 0xfb01, 0xfb02,
	0x2021, 0x00b7, 0x201a, 0x201e, 0x2030, 0x00c2, 0x00ca, 0x00c1,
	0x00cb, 0x00c8, 0x00cd, 0x00ce, 0x00cf, 0x00cc, 0x00d3, 0x00d4,
	0xf8ff, 0x00d2, 0x00da, 0x00db, 0x00d9, 0x0131, 0x02c6, 0x02dc,
	0x00af, 0x02d8, 0x02d9, 0x02da, 0x00b8, 0x02dd, 0x02db, 0x02c7,
}
