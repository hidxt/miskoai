package media

import "encoding/binary"

func gifSubblocks(s *imageScanner) error {
	for {
		n, err := s.byte()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if err = s.skip(int64(n)); err != nil {
			return err
		}
	}
}
func scanGIF(s *imageScanner) (ImageInfo, error) {
	info := ImageInfo{Format: "gif"}
	var hdr [13]byte
	if err := s.read(hdr[:]); err != nil {
		return ImageInfo{}, err
	}
	w, h := int64(binary.LittleEndian.Uint16(hdr[6:8])), int64(binary.LittleEndian.Uint16(hdr[8:10]))
	if err := imageDimensions(w, h); err != nil {
		return ImageInfo{}, err
	}
	info.Width = int(w)
	info.Height = int(h)
	if hdr[10]&128 != 0 {
		if err := s.skip(3 * (1 << ((hdr[10] & 7) + 1))); err != nil {
			return ImageInfo{}, err
		}
	}
	var pixels, interlacedPixels int64
	for s.remaining > 0 {
		kind, err := s.byte()
		if err != nil {
			return ImageInfo{}, err
		}
		switch kind {
		case 0x3b:
			if info.Frames == 0 || s.remaining != 0 {
				return ImageInfo{}, ErrInvalid
			}
			info.Animated = info.Frames > 1
			return info, nil
		case 0x21:
			label, err := s.byte()
			if err != nil {
				return ImageInfo{}, err
			}
			switch label {
			case 0xf9:
				var p [6]byte
				if err = s.read(p[:]); err != nil {
					return ImageInfo{}, err
				}
				if p[0] != 4 || p[5] != 0 {
					return ImageInfo{}, ErrInvalid
				}
			case 0xff:
				n, e := s.byte()
				if e != nil {
					return ImageInfo{}, e
				}
				if n != 11 {
					return ImageInfo{}, ErrInvalid
				}
				if e = s.skip(11); e != nil {
					return ImageInfo{}, e
				}
				if e = gifSubblocks(s); e != nil {
					return ImageInfo{}, e
				}
			case 0xfe:
				if err = gifSubblocks(s); err != nil {
					return ImageInfo{}, err
				}
			default:
				return ImageInfo{}, ErrUnsupported
			}
		case 0x2c:
			info.Frames++
			if info.Frames > 32 {
				return ImageInfo{}, ErrLimit
			}
			var p [9]byte
			if err = s.read(p[:]); err != nil {
				return ImageInfo{}, err
			}
			x, y := int64(binary.LittleEndian.Uint16(p[:2])), int64(binary.LittleEndian.Uint16(p[2:4]))
			fw, fh := int64(binary.LittleEndian.Uint16(p[4:6])), int64(binary.LittleEndian.Uint16(p[6:8]))
			if fw <= 0 || fh <= 0 || x+fw > w || y+fh > h {
				return ImageInfo{}, ErrInvalid
			}
			if p[8]&0x18 != 0 {
				return ImageInfo{}, ErrUnsupported
			}
			fp, e := imageMul(fw, fh)
			if e != nil {
				return ImageInfo{}, e
			}
			pixels, e = imageAdd(pixels, fp)
			if e != nil {
				return ImageInfo{}, e
			}
			if p[8]&64 != 0 {
				interlacedPixels, e = imageAdd(interlacedPixels, fp)
				if e != nil {
					return ImageInfo{}, e
				}
			}
			info.EstimatedDecodeBytes, e = imageEstimate(pixels, interlacedPixels, imageFixedBytes, int64(info.Frames)*(16<<10))
			if e != nil {
				return ImageInfo{}, e
			}
			if p[8]&128 != 0 {
				if e = s.skip(3 * (1 << ((p[8] & 7) + 1))); e != nil {
					return ImageInfo{}, e
				}
			}
			litWidth, e := s.byte()
			if e != nil {
				return ImageInfo{}, e
			}
			if litWidth < 2 || litWidth > 8 {
				return ImageInfo{}, ErrInvalid
			}
			if e = gifSubblocks(s); e != nil {
				return ImageInfo{}, e
			}
		default:
			return ImageInfo{}, ErrUnsupported
		}
	}
	return ImageInfo{}, ErrInvalid
}
