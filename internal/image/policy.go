package image

// Policy controls how images are recompressed before delivery.
type Policy struct {
	Format    string // "jpeg", "webp", "png", "gif"
	Quality   int    // 1-100; derived from quality name
	MaxWidth  int
	MaxHeight int
}

// QualityValue maps a quality name to a JPEG/WebP quality integer.
func QualityValue(name string) int {
	switch name {
	case "high":
		return 85
	case "medium":
		return 65
	case "low":
		return 40
	default:
		return 65
	}
}

// DefaultPolicy returns a Policy with sensible defaults.
func DefaultPolicy() Policy {
	return Policy{
		Format:    "jpeg",
		Quality:   QualityValue("medium"),
		MaxWidth:  800,
		MaxHeight: 1200,
	}
}

// FromSettings builds a Policy from user-facing quality/format strings.
func FromSettings(format, quality string, maxW, maxH int) Policy {
	p := Policy{
		Format:    format,
		Quality:   QualityValue(quality),
		MaxWidth:  maxW,
		MaxHeight: maxH,
	}
	if p.Format == "" {
		p.Format = "jpeg"
	}
	if p.MaxWidth <= 0 {
		p.MaxWidth = 800
	}
	if p.MaxHeight <= 0 {
		p.MaxHeight = 1200
	}
	return p
}
