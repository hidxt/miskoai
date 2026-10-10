package media

import "encoding/binary"

func scanJPEG(s *imageScanner) (ImageInfo, error) {
	info := ImageInfo{Format: "jpeg", Frames: 1}
	var soi [2]byte
	if err := s.read(soi[:]); err != nil {
		return ImageInfo{}, err
	}
	sof, sos, entropy := false, false, false
	for markers := 0; markers < 4096; markers++ {
		var marker byte
		for {
			b, err := s.byte()
			if err != nil {
				return ImageInfo{}, err
			}
			if b != 255 {
				if entropy {
					continue
				}
				return ImageInfo{}, ErrInvalid
			}
			for {
				marker, err = s.byte()
				if err != nil {
					return ImageInfo{}, err
				}
				if marker != 255 {
					break
				}
			}
			if marker == 0 {
				if entropy {
					continue
				}
				return ImageInfo{}, ErrInvalid
			}
			break
		}
		if marker >= 0xd0 && marker <= 0xd7 {
			if !entropy {
				return ImageInfo{}, ErrInvalid
			}
			continue
		}
		entropy = false
		if marker == 0xd9 {
			if !sof || !sos || s.remaining != 0 {
				return ImageInfo{}, ErrInvalid
			}
			return info, nil
		}
		if marker == 0xd8 || marker == 1 {
			return ImageInfo{}, ErrUnsupported
		}
		var lenbuf [2]byte
		if err := s.read(lenbuf[:]); err != nil {
			return ImageInfo{}, err
		}
		n := int64(binary.BigEndian.Uint16(lenbuf[:])) - 2
		if n < 0 || n > s.remaining {
			return ImageInfo{}, ErrInvalid
		}
		if marker >= 0xc0 && marker <= 0xcf && marker != 0xc4 && marker != 0xc8 && marker != 0xcc {
			if sof {
				return ImageInfo{}, ErrInvalid
			}
			if marker != 0xc0 && marker != 0xc1 && marker != 0xc2 {
				return ImageInfo{}, ErrUnsupported
			}
			if n < 6 {
				return ImageInfo{}, ErrInvalid
			}
			var p [18]byte
			if err := s.read(p[:6]); err != nil {
				return ImageInfo{}, err
			}
			nc := int(p[5])
			if nc != 1 && nc != 3 && nc != 4 {
				return ImageInfo{}, ErrUnsupported
			}
			if n != int64(6+3*nc) {
				return ImageInfo{}, ErrInvalid
			}
			if p[0] != 8 {
				return ImageInfo{}, ErrUnsupported
			}
			h, w := int64(binary.BigEndian.Uint16(p[1:3])), int64(binary.BigEndian.Uint16(p[3:5]))
			if err := imageDimensions(w, h); err != nil {
				return ImageInfo{}, err
			}
			if err := s.read(p[6 : 6+3*nc]); err != nil {
				return ImageInfo{}, err
			}
			var maxH, maxV, sumHV int64
			for i := 0; i < nc; i++ {
				id, hv := p[6+3*i], p[7+3*i]
				hi, vi := int64(hv>>4), int64(hv&15)
				if hi < 1 || hi > 4 || vi < 1 || vi > 4 {
					return ImageInfo{}, ErrUnsupported
				}
				if p[8+3*i] > 3 {
					return ImageInfo{}, ErrInvalid
				}
				for j := 0; j < i; j++ {
					if id == p[6+3*j] {
						return ImageInfo{}, ErrInvalid
					}
				}
				maxH = max(maxH, hi)
				maxV = max(maxV, vi)
				sumHV += hi * vi
			}
			mx, my := (w+8*maxH-1)/(8*maxH), (h+8*maxV-1)/(8*maxV)
			// Dimensions <=8192 and sampling <=4 were checked above, so MCU
			// rounding additions cannot overflow. Check every buffer product.
			planes, err := imageProduct(4, 8, maxH, mx, 8, maxV, my)
			if err != nil {
				return ImageInfo{}, err
			}
			conversion, err := imageProduct(4, w, h)
			if err != nil {
				return ImageInfo{}, err
			}
			coeff := int64(0)
			if marker == 0xc2 {
				coeff, err = imageProduct(256, mx, my, sumHV)
				if err != nil {
					return ImageInfo{}, err
				}
			}
			est, err := imageEstimate(planes, conversion, coeff, imageFixedBytes)
			if err != nil {
				return ImageInfo{}, err
			}
			info.Width = int(w)
			info.Height = int(h)
			info.EstimatedDecodeBytes = est
			sof = true
		} else {
			if marker == 0xda {
				if !sof {
					return ImageInfo{}, ErrInvalid
				}
				sos = true
				entropy = true
			}
			// Only the decoder's known, bounded marker families are admitted.
			if marker != 0xda && marker != 0xc4 && marker != 0xdb && marker != 0xdd && marker != 0xfe && !(marker >= 0xe0 && marker <= 0xef) {
				return ImageInfo{}, ErrUnsupported
			}
			if err := s.skip(n); err != nil {
				return ImageInfo{}, err
			}
		}
	}
	return ImageInfo{}, ErrLimit
}
