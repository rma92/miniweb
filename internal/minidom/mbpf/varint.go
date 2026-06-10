package mbpf

import (
	"fmt"
	"io"
)

// AppendVarint appends an unsigned LEB-128 varint to buf and returns the result.
func AppendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// ReadVarint reads a single unsigned LEB-128 varint from r.
func ReadVarint(r io.ByteReader) (uint64, error) {
	var v uint64
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		v |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return v, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, fmt.Errorf("varint overflow")
		}
	}
}

// VarintSize returns how many bytes the varint encoding of v requires.
func VarintSize(v uint64) int {
	n := 1
	for v >= 0x80 {
		n++
		v >>= 7
	}
	return n
}
