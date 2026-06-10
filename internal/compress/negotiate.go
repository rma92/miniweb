package compress

import (
	"strings"
)

// Negotiate selects the best compression algorithm given the client's
// Accept-Encoding header and the server's allowed set.
func Negotiate(acceptEncoding string, allowed []string) string {
	if acceptEncoding == "" || len(allowed) == 0 {
		return AlgoNone
	}

	// Build a set of what the client accepts.
	clientAccepts := make(map[string]bool)
	for _, part := range strings.Split(acceptEncoding, ",") {
		tok := strings.TrimSpace(part)
		// Strip quality value (e.g. "gzip;q=0.9" → "gzip")
		if i := strings.IndexByte(tok, ';'); i >= 0 {
			tok = strings.TrimSpace(tok[:i])
		}
		clientAccepts[strings.ToLower(tok)] = true
	}

	// Prefer algorithms in order: brotli > gzip > none.
	preference := []string{AlgoBrotli, AlgoGzip, AlgoNone}

	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}

	for _, algo := range preference {
		if !allowedSet[algo] {
			continue
		}
		switch algo {
		case AlgoBrotli:
			if clientAccepts["br"] {
				return AlgoBrotli
			}
		case AlgoGzip:
			if clientAccepts["gzip"] || clientAccepts["*"] {
				return AlgoGzip
			}
		case AlgoNone:
			return AlgoNone
		}
	}
	return AlgoNone
}
