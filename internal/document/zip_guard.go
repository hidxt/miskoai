package document

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxZIPEntries = 256
)

const (
	maxZIPCentral = 256 * 1024
	maxZIPName    = 512
	maxZIPExtra   = 4096
	maxZIPComment = 1024
	maxZIPDecoded = 8 * 1024 * 1024
	maxZIPTail    = 65557
)

type zipMember struct {
	name, comment                                               string
	extra                                                       []byte
	creator, version, flags, method, modifiedTime, modifiedDate uint16
	crc, compressed, decoded, attrs                             uint32
	local, body, end                                            int64
	directory                                                   bool
}

// The manifest is private admission evidence, never a filesystem path API.
// Its input must remain the same immutable completed spool through validation.
type zipManifest struct {
	size, central int64
	comment       string
	members       []zipMember
}

func admitZIP(ctx context.Context, input io.ReaderAt, size int64) (*zipManifest, error) {
	if ctx == nil || nilReader(input) || size < 0 {
		return nil, ErrInvalid
	}
	if size > maxInputBytes {
		return nil, ErrLimit
	}
	if err := checkWork(ctx); err != nil {
		return nil, err
	}
	if size < 22 {
		return nil, ErrInvalid
	}
	r := checkedReaderAt{input: input, size: size}
	tailSize := size
	if tailSize > maxZIPTail {
		tailSize = maxZIPTail
	}
	tail := make([]byte, int(tailSize))
	if err := r.read(ctx, tail, size-tailSize); err != nil {
		return nil, err
	}
	// Enumerate candidates once. stdlib chooses the latest signature, including
	// candidates ending before EOF; refuse those ambiguities before its parser.
	candidate, latest, candidates := -1, -1, 0
	for i := 0; i <= len(tail)-22; i++ {
		if binary.LittleEndian.Uint32(tail[i:]) != 0x06054b50 {
			continue
		}
		latest = i
		end := i + 22 + int(binary.LittleEndian.Uint16(tail[i+20:]))
		if end <= len(tail) {
			candidates++
			if end == len(tail) {
				candidate = i
			}
		}
	}
	if candidates != 1 || candidate < 0 || latest != candidate {
		return nil, ErrInvalid
	}
	if err := checkWork(ctx); err != nil {
		return nil, err
	}
	e := tail[candidate:]
	u16 := func(at int) uint16 { return binary.LittleEndian.Uint16(e[at:]) }
	u32 := func(at int) uint32 { return binary.LittleEndian.Uint32(e[at:]) }
	count, centralSize, central := u16(10), u32(12), int64(u32(16))
	if u16(4) != 0 || u16(6) != 0 || count == 0xffff || u16(8) == 0xffff || centralSize == 0xffffffff || u32(16) == 0xffffffff {
		return nil, ErrUnsupported
	}
	if u16(8) != count {
		return nil, ErrInvalid
	}
	if count > maxZIPEntries || centralSize > maxZIPCentral {
		return nil, ErrLimit
	}
	eocd := size - tailSize + int64(candidate)
	if central > eocd || int64(centralSize) != eocd-central || int64(count)*46 > int64(centralSize) {
		return nil, ErrInvalid
	}
	manifest := &zipManifest{size: size, central: central, comment: string(e[22:]), members: make([]zipMember, 0, int(count))}
	seen := make(map[string]bool, int(count))
	offset, total := central, uint64(0)
	for i := 0; i < int(count); i++ {
		if err := checkWork(ctx); err != nil {
			return nil, err
		}
		var h [46]byte
		if offset > eocd || int64(len(h)) > eocd-offset {
			return nil, ErrInvalid
		}
		if err := r.read(ctx, h[:], offset); err != nil {
			return nil, err
		}
		if binary.LittleEndian.Uint32(h[:]) != 0x02014b50 {
			return nil, ErrInvalid
		}
		m := zipMember{
			creator: binary.LittleEndian.Uint16(h[4:]), version: binary.LittleEndian.Uint16(h[6:]), flags: binary.LittleEndian.Uint16(h[8:]), method: binary.LittleEndian.Uint16(h[10:]),
			modifiedTime: binary.LittleEndian.Uint16(h[12:]), modifiedDate: binary.LittleEndian.Uint16(h[14:]),
			crc: binary.LittleEndian.Uint32(h[16:]), compressed: binary.LittleEndian.Uint32(h[20:]), decoded: binary.LittleEndian.Uint32(h[24:]),
			attrs: binary.LittleEndian.Uint32(h[38:]), local: int64(binary.LittleEndian.Uint32(h[42:])),
		}
		if m.compressed == 0xffffffff || m.decoded == 0xffffffff || m.local == 0xffffffff || binary.LittleEndian.Uint16(h[34:]) != 0 {
			return nil, ErrUnsupported
		}
		nameLen, extraLen, commentLen := int(binary.LittleEndian.Uint16(h[28:])), int(binary.LittleEndian.Uint16(h[30:])), int(binary.LittleEndian.Uint16(h[32:]))
		if nameLen > maxZIPName || extraLen > maxZIPExtra || commentLen > maxZIPComment {
			return nil, ErrLimit
		}
		fieldSize := nameLen + extraLen + commentLen
		offset += 46
		if int64(fieldSize) > eocd-offset {
			return nil, ErrInvalid
		}
		fields := make([]byte, fieldSize)
		if fieldSize > 0 {
			if err := r.read(ctx, fields, offset); err != nil {
				return nil, err
			}
		}
		offset += int64(fieldSize)
		m.name = string(fields[:nameLen])
		m.extra = fields[nameLen : nameLen+extraLen]
		m.comment = string(fields[nameLen+extraLen:])
		if err := canonicalZIPName(m.name); err != nil {
			return nil, err
		}
		canonical := strings.TrimSuffix(m.name, "/")
		if seen[canonical] {
			return nil, ErrInvalid
		}
		seen[canonical] = true
		if err := zipExtra(m.extra); err != nil {
			return nil, err
		}
		if err := zipFlags(m.method, m.flags); err != nil {
			return nil, err
		}
		if m.method == zip.Store && m.compressed != m.decoded {
			return nil, ErrInvalid
		}
		if err := zipType(&m); err != nil {
			return nil, err
		}
		if uint64(m.decoded) > maxZIPDecoded-total {
			return nil, ErrLimit
		}
		total += uint64(m.decoded)
		manifest.members = append(manifest.members, m)
	}
	if offset != eocd {
		return nil, ErrInvalid
	}
	for i := range manifest.members {
		if err := admitZIPLocal(ctx, r, central, &manifest.members[i]); err != nil {
			return nil, err
		}
	}
	// This restricted subset has no prepended bytes or unclaimed inter-record
	// gaps. It excludes archive envelopes and hidden records as well as overlap.
	order := make([]int, len(manifest.members))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return manifest.members[order[i]].local < manifest.members[order[j]].local })
	next := int64(0)
	for _, i := range order {
		if manifest.members[i].local != next {
			return nil, ErrInvalid
		}
		next = manifest.members[i].end
	}
	if next != central {
		return nil, ErrInvalid
	}
	if err := checkWork(ctx); err != nil {
		return nil, err
	}
	return manifest, nil
}

func canonicalZIPName(name string) error {
	if name == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\\\x00:") || strings.HasPrefix(name, "/") {
		return ErrInvalid
	}
	trimmed := strings.TrimSuffix(name, "/")
	for _, component := range strings.Split(trimmed, "/") {
		if component == "" || component == "." || component == ".." {
			return ErrInvalid
		}
	}
	return nil
}

func zipExtra(extra []byte) error {
	for len(extra) > 0 {
		if len(extra) < 4 {
			return ErrInvalid
		}
		id, n := binary.LittleEndian.Uint16(extra), int(binary.LittleEndian.Uint16(extra[2:]))
		if id == 1 {
			return ErrUnsupported
		}
		extra = extra[4:]
		if n > len(extra) {
			return ErrInvalid
		}
		extra = extra[n:]
	}
	return nil
}

func zipFlags(method, flags uint16) error {
	allowed := uint16(0x808)
	if method == zip.Deflate {
		allowed |= 6
	} else if method != zip.Store {
		return ErrUnsupported
	}
	if flags & ^allowed != 0 {
		return ErrUnsupported
	}
	return nil
}

func zipType(m *zipMember) error {
	creator := m.creator >> 8
	if creator == 3 || creator == 19 {
		typeBits := (m.attrs >> 16) & 0170000
		if typeBits != 0 && typeBits != 0100000 && typeBits != 0040000 {
			return ErrUnsupported
		}
	}
	if (creator == 0 || creator == 10 || creator == 14) && m.attrs&8 != 0 {
		return ErrUnsupported
	}
	header := zip.FileHeader{Name: m.name, CreatorVersion: m.creator, ExternalAttrs: m.attrs}
	mode := header.Mode()
	if mode.Type() != 0 && mode.Type() != fs.ModeDir {
		return ErrUnsupported
	}
	m.directory = strings.HasSuffix(m.name, "/")
	if mode.IsDir() != m.directory {
		return ErrInvalid
	}
	if m.directory && (m.decoded != 0 || m.crc != 0) {
		return ErrInvalid
	}
	return nil
}

func admitZIPLocal(ctx context.Context, r checkedReaderAt, central int64, m *zipMember) error {
	var h [30]byte
	if m.local > central || int64(len(h)) > central-m.local {
		return ErrInvalid
	}
	if err := r.read(ctx, h[:], m.local); err != nil {
		return err
	}
	if binary.LittleEndian.Uint32(h[:]) != 0x04034b50 {
		return ErrInvalid
	}
	if binary.LittleEndian.Uint16(h[4:]) != m.version || binary.LittleEndian.Uint16(h[6:]) != m.flags || binary.LittleEndian.Uint16(h[8:]) != m.method || binary.LittleEndian.Uint16(h[10:]) != m.modifiedTime || binary.LittleEndian.Uint16(h[12:]) != m.modifiedDate {
		return ErrInvalid
	}
	crc, compressed, decoded := binary.LittleEndian.Uint32(h[14:]), binary.LittleEndian.Uint32(h[18:]), binary.LittleEndian.Uint32(h[22:])
	if compressed == 0xffffffff || decoded == 0xffffffff {
		return ErrUnsupported
	}
	if m.flags&8 == 0 {
		if crc != m.crc || compressed != m.compressed || decoded != m.decoded {
			return ErrInvalid
		}
	} else if (crc != 0 && crc != m.crc) || (compressed != 0 && compressed != m.compressed) || (decoded != 0 && decoded != m.decoded) {
		return ErrInvalid
	}
	nameLen, extraLen := int(binary.LittleEndian.Uint16(h[26:])), int(binary.LittleEndian.Uint16(h[28:]))
	if nameLen > maxZIPName || extraLen > maxZIPExtra {
		return ErrLimit
	}
	fieldsStart := m.local + 30
	if int64(nameLen+extraLen) > central-fieldsStart {
		return ErrInvalid
	}
	fields := make([]byte, nameLen+extraLen)
	if len(fields) > 0 {
		if err := r.read(ctx, fields, fieldsStart); err != nil {
			return err
		}
	}
	if !bytes.Equal(fields[:nameLen], []byte(m.name)) {
		return ErrInvalid
	}
	if err := zipExtra(fields[nameLen:]); err != nil {
		return err
	}
	m.body = fieldsStart + int64(len(fields))
	if int64(m.compressed) > central-m.body {
		return ErrInvalid
	}
	m.end = m.body + int64(m.compressed)
	if m.flags&8 != 0 {
		var d [16]byte
		if 12 > central-m.end {
			return ErrInvalid
		}
		if err := r.read(ctx, d[:12], m.end); err != nil {
			return err
		}
		start, length := 0, int64(12)
		if binary.LittleEndian.Uint32(d[:]) == 0x08074b50 {
			if 16 > central-m.end {
				return ErrInvalid
			}
			if err := r.read(ctx, d[12:], m.end+12); err != nil {
				return err
			}
			start, length = 4, 16
		}
		if binary.LittleEndian.Uint32(d[start:]) != m.crc || binary.LittleEndian.Uint32(d[start+4:]) != m.compressed || binary.LittleEndian.Uint32(d[start+8:]) != m.decoded {
			return ErrInvalid
		}
		m.end += length
	}
	return checkWork(ctx)
}
