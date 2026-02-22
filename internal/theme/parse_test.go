package theme

import "testing"

func TestIsHexColor(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#abc", true},
		{"#ABC", true},
		{"#aabbcc", true},
		{"#AABBCC", true},
		{"#aabbccdd", true},
		{"#AABBCCDD", true},
		{"#123", true},
		{"#112233", true},
		{"#11223344", true},
		{"", false},
		{"abc", false},
		{"#", false},
		{"#ab", false},
		{"#abcd", false},
		{"#abcde", false},
		{"#abcdefg", false},
		{"#gggggg", false},
		{"#zzzzzz", false},
		{"#abcdef ", false}, // trailing space makes length wrong
		{" #abcdef", false}, // leading space
		{"#12345", false},
	}
	for _, tt := range tests {
		got := IsHexColor(tt.input)
		if got != tt.want {
			t.Errorf("IsHexColor(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseThemeTOML_Full(t *testing.T) {
	data := []byte(`
accent = "#ff0000"
background = "#111111"
surface = "#222222"
surface_alt = "#333333"
text = "#eeeeee"
text_dim = "#999999"
border = "#555555"
`)
	c := ParseThemeTOML(data)
	if c.Accent != "#ff0000" {
		t.Errorf("Accent = %q, want #ff0000", c.Accent)
	}
	if c.Background != "#111111" {
		t.Errorf("Background = %q, want #111111", c.Background)
	}
	if c.Surface != "#222222" {
		t.Errorf("Surface = %q, want #222222", c.Surface)
	}
	if c.SurfaceAlt != "#333333" {
		t.Errorf("SurfaceAlt = %q, want #333333", c.SurfaceAlt)
	}
	if c.Text != "#eeeeee" {
		t.Errorf("Text = %q, want #eeeeee", c.Text)
	}
	if c.TextDim != "#999999" {
		t.Errorf("TextDim = %q, want #999999", c.TextDim)
	}
	if c.Border != "#555555" {
		t.Errorf("Border = %q, want #555555", c.Border)
	}
}

func TestParseThemeTOML_MissingKeysKeepDefaults(t *testing.T) {
	data := []byte(`accent = "#ff0000"`)
	c := ParseThemeTOML(data)
	if c.Accent != "#ff0000" {
		t.Errorf("Accent = %q, want #ff0000", c.Accent)
	}
	if c.Background != DefaultColors.Background {
		t.Errorf("Background = %q, want default %q", c.Background, DefaultColors.Background)
	}
}

func TestParseThemeTOML_InvalidHexIgnored(t *testing.T) {
	data := []byte(`accent = "not-a-color"`)
	c := ParseThemeTOML(data)
	if c.Accent != DefaultColors.Accent {
		t.Errorf("Accent = %q, want default %q", c.Accent, DefaultColors.Accent)
	}
}

func TestParseThemeTOML_CommentsAndBlanks(t *testing.T) {
	data := []byte(`
# This is a comment
accent = "#aabbcc"

# Another comment
background = "#112233"
`)
	c := ParseThemeTOML(data)
	if c.Accent != "#aabbcc" {
		t.Errorf("Accent = %q, want #aabbcc", c.Accent)
	}
	if c.Background != "#112233" {
		t.Errorf("Background = %q, want #112233", c.Background)
	}
}

func TestParseThemeTOML_InlineComments(t *testing.T) {
	data := []byte(`accent = "#aabbcc" # the accent color`)
	c := ParseThemeTOML(data)
	if c.Accent != "#aabbcc" {
		t.Errorf("Accent = %q, want #aabbcc", c.Accent)
	}
}

func TestParseThemeTOML_QuotedValues(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"double-quoted", `accent = "#aabbcc"`, "#aabbcc"},
		{"single-quoted", `accent = '#aabbcc'`, "#aabbcc"},
		// Unquoted hex values with # are ambiguous: ` #aabbcc` matches the
		// inline-comment heuristic, so the parser strips the value. Values
		// must be quoted to survive the inline-comment stripping.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ParseThemeTOML([]byte(tt.input))
			if c.Accent != tt.want {
				t.Errorf("Accent = %q, want %q", c.Accent, tt.want)
			}
		})
	}
}

func TestParseThemeTOML_UnknownKeysIgnored(t *testing.T) {
	data := []byte(`
accent = "#ff0000"
unknown_key = "#00ff00"
another_thing = "hello"
`)
	c := ParseThemeTOML(data)
	if c.Accent != "#ff0000" {
		t.Errorf("Accent = %q, want #ff0000", c.Accent)
	}
	// No panic, unknown keys silently ignored.
}

func TestParseThemeTOML_Empty(t *testing.T) {
	c := ParseThemeTOML([]byte(""))
	if c != DefaultColors {
		t.Error("empty input should return DefaultColors")
	}
}

func TestParseThemeTOML_NoEquals(t *testing.T) {
	data := []byte("this is not valid toml\n")
	c := ParseThemeTOML(data)
	if c != DefaultColors {
		t.Error("invalid lines should be ignored, returning DefaultColors")
	}
}
