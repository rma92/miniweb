package image

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"time"

	// Register WebP decode support.
	_ "golang.org/x/image/webp"
	xdraw "golang.org/x/image/draw"
)

var httpClient = &http.Client{Timeout: 8 * time.Second}

// Recompress fetches the image at srcURL, resizes if needed, and re-encodes
// to the format specified in policy. Returns the encoded bytes and MIME type.
func Recompress(srcURL string, policy Policy) ([]byte, string, error) {
	resp, err := httpClient.Get(srcURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MB max
	if err != nil {
		return nil, "", fmt.Errorf("read image body: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		// Return original bytes if we can't decode (e.g. SVG).
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/octet-stream"
		}
		return raw, ct, nil
	}

	// Resize if necessary.
	img = maybeResize(img, policy.MaxWidth, policy.MaxHeight)

	// Re-encode.
	return encode(img, policy)
}

func maybeResize(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()

	if (maxW <= 0 || w <= maxW) && (maxH <= 0 || h <= maxH) {
		return img
	}

	// Scale down proportionally.
	scaleW := 1.0
	if maxW > 0 && w > maxW {
		scaleW = float64(maxW) / float64(w)
	}
	scaleH := 1.0
	if maxH > 0 && h > maxH {
		scaleH = float64(maxH) / float64(h)
	}
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}

	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
	return dst
}

func encode(img image.Image, policy Policy) ([]byte, string, error) {
	var buf bytes.Buffer
	var mimeType string

	switch policy.Format {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", err
		}
		mimeType = "image/png"

	case "gif":
		if err := gif.Encode(&buf, img, nil); err != nil {
			return nil, "", err
		}
		mimeType = "image/gif"

	case "webp":
		// Pure-Go WebP encoding is not available in golang.org/x/image.
		// Fall back to JPEG until a cgo encoder is added in Phase 2.
		q := policy.Quality
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, "", err
		}
		mimeType = "image/jpeg" // TODO Phase 2: use chai2010/webp for true WebP output

	default: // "jpeg" and anything else
		q := policy.Quality
		if q <= 0 {
			q = 75
		}
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, "", err
		}
		mimeType = "image/jpeg"
	}

	return buf.Bytes(), mimeType, nil
}
