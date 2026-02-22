package theme

import "strings"

// ParseThemeTOML reads a theme.toml file. Lines of the form key = "value" or
// key = value are parsed. Comment lines (#) and blank lines are ignored.
// Unknown keys are silently ignored. Invalid hex values fall back to the
// corresponding default. Missing keys keep their default value.
func ParseThemeTOML(data []byte) Colors {
	c := DefaultColors
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
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
	return c
}

// IsHexColor returns true for valid CSS hex color strings: #rgb, #rrggbb, #rrggbbaa.
func IsHexColor(s string) bool {
	if len(s) == 0 || s[0] != '#' {
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
