package document

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"hash/crc32"
	"io"
)

func validateZIPMembers(ctx context.Context, input io.ReaderAt, manifest *zipManifest, selected map[string]bool) (map[string][]byte, error) {
	if ctx == nil || nilReader(input) || manifest == nil {
		return nil, ErrInvalid
	}
	for name := range selected {
		if name != "[Content_Types].xml" && name != "_rels/.rels" && name != "word/document.xml" {
			return nil, ErrInvalid
		}
	}
	if err := checkWork(ctx); err != nil {
		return nil, err
	}
	// Only this boundary invokes the standard parser, after the complete bounded
	// manifest, all local records, and descriptors have been admitted.
	r := zipContextReaderAt{ctx: ctx, reader: checkedReaderAt{input: input, size: manifest.size}}
	zr, err := zip.NewReader(r, manifest.size)
	if workErr := checkWork(ctx); workErr != nil {
		return nil, workErr
	}
	if err != nil || len(zr.File) != len(manifest.members) || zr.Comment != manifest.comment {
		return nil, ErrInvalid
	}
	for i, f := range zr.File {
		if err := crosscheckZIPFile(ctx, f, &manifest.members[i], r.reader, manifest.central); err != nil {
			return nil, err
		}
	}
	parts := make(map[string][]byte, 3)
	total := uint64(0)
	for i, f := range zr.File {
		m := &manifest.members[i]
		part, err := decodeZIPMember(ctx, f, m, selected[m.name], &total)
		if err != nil {
			return nil, err
		}
		if selected[m.name] {
			parts[m.name] = part
		}
	}
	if err := checkWork(ctx); err != nil {
		return nil, err
	}
	return parts, nil
}

// The standard ZIP reader gets the same safe bounded synchronous I/O contract
// as preflight. No reader error text or partial backing read escapes it.
type zipContextReaderAt struct {
	ctx    context.Context
	reader checkedReaderAt
}

func (r zipContextReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.reader.read(r.ctx, p, offset); err != nil {
		return 0, err
	}
	return len(p), nil
}

func crosscheckZIPFile(ctx context.Context, f *zip.File, m *zipMember, reader checkedReaderAt, central int64) error {
	if err := checkWork(ctx); err != nil {
		return err
	}
	if f.Name != m.name || f.Comment != m.comment || !bytes.Equal(f.Extra, m.extra) ||
		f.CreatorVersion != m.creator || f.ReaderVersion != m.version || f.Flags != m.flags || f.Method != m.method ||
		f.ModifiedTime != m.modifiedTime || f.ModifiedDate != m.modifiedDate || f.ExternalAttrs != m.attrs ||
		f.CRC32 != m.crc || f.CompressedSize != m.compressed || f.UncompressedSize != m.decoded ||
		f.CompressedSize64 != uint64(m.compressed) || f.UncompressedSize64 != uint64(m.decoded) {
		return ErrInvalid
	}
	offset, err := f.DataOffset()
	if workErr := checkWork(ctx); workErr != nil {
		return workErr
	}
	if err != nil || offset != m.body {
		return ErrInvalid
	}
	// DataOffset only reads lengths. Reconcile the local header again rather
	// than treating stdlib's minimal local-header check as admission evidence.
	local := *m
	if err := admitZIPLocal(ctx, reader, central, &local); err != nil {
		return err
	}
	if local.body != m.body || local.end != m.end {
		return ErrInvalid
	}
	return nil
}

// flate receives an io.ByteReader so its own buffering cannot conceal bytes
// after the final block. consumed counts bytes actually requested by flate,
// not bytes prefetched into an application buffer.
type zipRawReader struct {
	ctx             context.Context
	input           io.Reader
	limit, consumed uint64
}

func (r *zipRawReader) Read(p []byte) (int, error) {
	if err := checkWork(r.ctx); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.consumed == r.limit {
		return 0, io.EOF
	}
	if len(p) > 4096 {
		p = p[:4096]
	}
	if uint64(len(p)) > r.limit-r.consumed {
		p = p[:int(r.limit-r.consumed)]
	}
	n, err := r.input.Read(p)
	if workErr := checkWork(r.ctx); workErr != nil {
		return 0, workErr
	}
	if n < 0 || n > len(p) || (err != nil && err != io.EOF) || (n == 0 && err == nil) {
		return 0, ErrInvalid
	}
	r.consumed += uint64(n)
	return n, err
}

func (r *zipRawReader) ReadByte() (byte, error) {
	var b [1]byte
	n, err := r.Read(b[:])
	if n == 1 {
		return b[0], nil
	}
	return 0, err
}

func decodeZIPMember(ctx context.Context, f *zip.File, m *zipMember, retain bool, total *uint64) (part []byte, resultErr error) {
	if err := checkWork(ctx); err != nil {
		return nil, err
	}
	raw, err := f.OpenRaw()
	if workErr := checkWork(ctx); workErr != nil {
		return nil, workErr
	}
	if err != nil {
		return nil, ErrInvalid
	}
	bounded := &zipRawReader{ctx: ctx, input: raw, limit: uint64(m.compressed)}
	var decoded io.Reader = bounded
	if m.method == zip.Deflate {
		decoder := flate.NewReader(bounded)
		defer func() {
			closeErr := decoder.Close()
			if resultErr == nil && closeErr != nil {
				part, resultErr = nil, ErrInvalid
			}
		}()
		decoded = decoder
	}
	var scratch [4096]byte
	length := uint64(0)
	crc := uint32(0)
	for {
		if err := checkWork(ctx); err != nil {
			return nil, err
		}
		window := scratch[:]
		remaining := uint64(maxZIPDecoded) - *total
		if remaining < uint64(len(window)) {
			// Probe at most one byte beyond the remaining aggregate so exact
			// quota EOF can succeed without decoding another full chunk.
			window = window[:int(remaining)+1]
		}
		n, readErr := decoded.Read(window)
		if err := checkWork(ctx); err != nil {
			return nil, err
		}
		if n < 0 || n > len(window) {
			return nil, ErrInvalid
		}
		if uint64(n) > maxZIPDecoded-*total {
			return nil, ErrLimit
		}
		*total += uint64(n)
		length += uint64(n)
		crc = crc32.Update(crc, crc32.IEEETable, scratch[:n])
		if retain && n > 0 {
			// Actual aggregate admission precedes every append, never truncating
			// a successful part. Unselected members use only the fixed scratch.
			part = append(part, scratch[:n]...)
		}
		if readErr != nil {
			if readErr != io.EOF {
				return nil, ErrInvalid
			}
			break
		}
		if n == 0 {
			return nil, ErrInvalid
		}
	}
	if length != uint64(m.decoded) || crc != m.crc || bounded.consumed != uint64(m.compressed) {
		return nil, ErrInvalid
	}
	return part, nil
}
