package theme

import "strings"

// ParseThemeTOML reads a theme.toml file and returns its color definitions.
// Lines of the form key = "value" or key = value are parsed. Comment lines (#)
// and blank lines are ignored. Unknown keys are silently ignored. Invalid hex
// values fall back to the corresponding default. Missing keys keep their default
// value. Any [accents] section is ignored; use ParseThemeTOMLFull to get accents.
func ParseThemeTOML(data []byte) Colors {
	c, _ := ParseThemeTOMLFull(data)
	return c
}

// ParseThemeTOMLFull reads a theme.toml file and returns both its color
// definitions and any accent color variants defined in an [accents] section.
// The [accents] section is optional. Each line in it should be of the form
// id = "#hex". The returned accent slice preserves file order.
func ParseThemeTOMLFull(data []byte) (Colors, []Accent) {
	c := DefaultColors
	var accents []Accent
	inAccents := false

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detect section headers.
		if strings.HasPrefix(line, "[") {
			inAccents = strings.TrimSpace(line) == "[accents]"
			continue
		}

		// Strip inline comments.
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if !IsHexColor(v) {
			continue
		}

		if inAccents {
			accents = append(accents, Accent{
				ID:   k,
				Name: titleCase(k),
				Hex:  v,
			})
			continue
		}

		switch k {
		case "accent":
			c.Accent = v
		case "background":
			c.Background = v
		case "surface":
			c.Surface = v
		case "surface_alt":
			c.SurfaceAlt = v
		case "text":
			c.Text = v
		case "text_dim":
			c.TextDim = v
		case "border":
			c.Border = v
		}
	}
	return c, accents
}

// titleCase uppercases the first byte of s. Only correct for ASCII strings,
// which is fine for accent IDs like "blue" or "sapphire".
func titleCase(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// IsHexColor returns true for valid CSS hex color strings: #rgb, #rrggbb, #rrggbbaa.
func IsHexColor(s string) bool {
	if s == "" || s[0] != '#' {
		return false
	}
	rest := s[1:]
	switch len(rest) {
	case 3, 6, 8:
	default:
		return false
	}
	for _, c := range rest {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
