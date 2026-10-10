package media

import (
	"encoding/binary"
	"hash/crc32"
)

// scanPNG uses fixed scratch; chunk length never controls an allocation.
func scanPNG(s *imageScanner) (ImageInfo, error) {
	info := ImageInfo{Format: "png", Frames: 1}
	var sig [8]byte
	if err := s.read(sig[:]); err != nil {
		return ImageInfo{}, err
	}
	haveIDAT, endedIDAT := false, false
	for chunks := 0; chunks < 4096; chunks++ {
		var hdr [8]byte
		if err := s.read(hdr[:]); err != nil {
			return ImageInfo{}, err
		}
		n := int64(binary.BigEndian.Uint32(hdr[:4]))
		kind := string(hdr[4:])
		if n > s.remaining-4 {
			return ImageInfo{}, ErrInvalid
		}
		for _, b := range hdr[4:] {
			if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z') {
				return ImageInfo{}, ErrInvalid
			}
		}
		if hdr[6]&32 != 0 {
			return ImageInfo{}, ErrInvalid
		}
		if chunks == 0 && (kind != "IHDR" || n != 13) {
			return ImageInfo{}, ErrInvalid
		}
		if chunks > 0 && kind == "IHDR" {
			return ImageInfo{}, ErrInvalid
		}
		if kind == "acTL" || kind == "fcTL" || kind == "fdAT" {
			return ImageInfo{}, ErrUnsupported
		}
		crc := crc32.NewIEEE()
		_, _ = crc.Write(hdr[4:])
		if kind == "IHDR" {
			var p [13]byte
			if err := s.read(p[:]); err != nil {
				return ImageInfo{}, err
			}
			_, _ = crc.Write(p[:])
			w, h := int64(binary.BigEndian.Uint32(p[:4])), int64(binary.BigEndian.Uint32(p[4:8]))
			if err := imageDimensions(w, h); err != nil {
				return ImageInfo{}, err
			}
			legal := false
			switch p[9] {
			case 0:
				legal = p[8] == 1 || p[8] == 2 || p[8] == 4 || p[8] == 8 || p[8] == 16
			case 2, 4, 6:
				legal = p[8] == 8 || p[8] == 16
			case 3:
				legal = p[8] == 1 || p[8] == 2 || p[8] == 4 || p[8] == 8
			}
			if !legal || p[10] != 0 || p[11] != 0 || p[12] > 1 {
				return ImageInfo{}, ErrUnsupported
			}
			pixels, err := imageProduct(16, w, h)
			if err != nil {
				return ImageInfo{}, err
			}
			row, err := imageProduct(8, w)
			if err != nil {
				return ImageInfo{}, err
			}
			row, err = imageAdd(row, 1)
			if err != nil {
				return ImageInfo{}, err
			}
			rows, err := imageMul(2, row)
			if err != nil {
				return ImageInfo{}, err
			}
			est, err := imageEstimate(pixels, rows, imageFixedBytes)
			if err != nil {
				return ImageInfo{}, err
			}
			info.Width = int(w)
			info.Height = int(h)
			info.EstimatedDecodeBytes = est
		} else {
			if kind == "IDAT" {
				if endedIDAT {
					return ImageInfo{}, ErrInvalid
				}
				haveIDAT = true
			} else if haveIDAT {
				endedIDAT = true
			}
			if kind == "IEND" && (n != 0 || !haveIDAT) {
				return ImageInfo{}, ErrInvalid
			}
			if hdr[4]&32 == 0 && kind != "PLTE" && kind != "IDAT" && kind != "IEND" {
				return ImageInfo{}, ErrUnsupported
			}
			var scratch [4096]byte
			for n > 0 {
				k := min(n, int64(len(scratch)))
				if err := s.read(scratch[:k]); err != nil {
					return ImageInfo{}, err
				}
				_, _ = crc.Write(scratch[:k])
				n -= k
			}
		}
		var checksum [4]byte
		if err := s.read(checksum[:]); err != nil {
			return ImageInfo{}, err
		}
		if binary.BigEndian.Uint32(checksum[:]) != crc.Sum32() {
			return ImageInfo{}, ErrInvalid
		}
		if kind == "IEND" {
			if s.remaining != 0 {
				return ImageInfo{}, ErrInvalid
			}
			return info, nil
		}
	}
	return ImageInfo{}, ErrLimit
}
