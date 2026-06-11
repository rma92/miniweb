package compress

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// Algorithm names.
const (
	AlgoNone   = "none"
	AlgoGzip   = "gzip"
	AlgoBrotli = "brotli"
	AlgoZstd   = "zstd"
)

// Compress compresses data using the given algorithm.
// level is algorithm-specific: for gzip use 1-9, for brotli use 1-11, -1 for default.
func Compress(algo string, level int, data []byte) ([]byte, error) {
	switch algo {
	case AlgoNone, "":
		return data, nil
	case AlgoGzip:
		return compressGzip(level, data)
	case AlgoBrotli:
		return compressBrotli(level, data)
	case AlgoZstd:
		return compressZstd(data)
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", algo)
	}
}

// Decompress decompresses data compressed with the given algorithm.
func Decompress(algo string, data []byte) ([]byte, error) {
	switch algo {
	case AlgoNone, "":
		return data, nil
	case AlgoGzip:
		return decompressGzip(data)
	case AlgoBrotli:
		return decompressBrotli(data)
	case AlgoZstd:
		return decompressZstd(data)
	default:
		return nil, fmt.Errorf("unsupported decompression algorithm: %s", algo)
	}
}

// ContentEncoding returns the HTTP Content-Encoding header value for an algorithm.
func ContentEncoding(algo string) string {
	switch algo {
	case AlgoGzip:
		return "gzip"
	case AlgoBrotli:
		return "br"
	case AlgoZstd:
		return "zstd"
	default:
		return ""
	}
}

func compressGzip(level int, data []byte) ([]byte, error) {
	if level <= 0 {
		level = gzip.DefaultCompression
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(data); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func compressBrotli(level int, data []byte) ([]byte, error) {
	if level <= 0 {
		level = brotli.DefaultCompression
	}
	var buf bytes.Buffer
	w := brotli.NewWriterLevel(&buf, level)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressBrotli(data []byte) ([]byte, error) {
	r := brotli.NewReader(bytes.NewReader(data))
	return io.ReadAll(r)
}

func compressZstd(data []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}
	return enc.EncodeAll(data, nil), nil
}

func decompressZstd(data []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return dec.DecodeAll(data, nil)
}
