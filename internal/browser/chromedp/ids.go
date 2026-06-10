package chromedp

import (
	"strings"

	"github.com/google/uuid"
)

// newID returns a prefixed UUID-based ID (no dashes).
func newID(prefix string) string {
	id := uuid.New().String()
	id = strings.ReplaceAll(id, "-", "")
	return prefix + "_" + id
}
