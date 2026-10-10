package document

import (
	"bytes"
	"context"
	"unicode"
	"unicode/utf8"
)

const textChunkBytes = 4096

func extractText(ctx context.Context, input checkedReaderAt) (string, error) {
	var chunk [textChunkBytes + utf8.UTFMax - 1]byte
	var output []byte
	carry := 0
	nonspace := false
	for offset := int64(0); offset < input.size; {
		length := int64(textChunkBytes)
		if length > input.size-offset {
			length = input.size - offset
		}
		if err := input.read(ctx, chunk[carry:carry+int(length)], offset); err != nil {
			return "", err
		}
		data := chunk[:carry+int(length)]
		if offset == 0 && bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
			data = data[3:]
		}
		offset += length
		consumed := 0
		for consumed < len(data) {
			if !utf8.FullRune(data[consumed:]) {
				break
			}
			r, width := utf8.DecodeRune(data[consumed:])
			if (r == utf8.RuneError && width == 1) || r == 0 {
				return "", ErrInvalid
			}
			if !unicode.IsSpace(r) {
				nonspace = true
			}
			consumed += width
		}
		var err error
		output, err = appendOutput(ctx, output, data[:consumed])
		if err != nil {
			return "", err
		}
		carry = copy(chunk[:], data[consumed:])
		if carry > utf8.UTFMax-1 {
			return "", ErrInvalid
		}
	}
	if carry != 0 {
		return "", ErrInvalid
	}
	if !nonspace {
		return "", ErrEmpty
	}
	return string(output), nil
}
