package theme

import (
	"strings"
	"testing"
)

func TestBuildThemeCSS_ContainsDefineColors(t *testing.T) {
	template := "/* template */\n.drawer { background: @z13-bg; }\n"
	css := BuildThemeCSS(DefaultColors, template)

	expected := []string{
		"@define-color z13-accent",
		"@define-color z13-bg",
		"@define-color z13-surface ",     // trailing space to distinguish from z13-surface-alt
		"@define-color z13-surface-alt",
		"@define-color z13-text ",         // trailing space to distinguish from z13-text-dim
		"@define-color z13-text-dim",
		"@define-color z13-border",
	}
	for _, exp := range expected {
		if !strings.Contains(css, exp) {
			t.Errorf("output missing %q", exp)
		}
	}
}

func TestBuildThemeCSS_ContainsColorValues(t *testing.T) {
	c := Colors{
		Accent:     "#ff0000",
		Background: "#111111",
		Surface:    "#222222",
		SurfaceAlt: "#333333",
		Text:       "#eeeeee",
		TextDim:    "#999999",
		Border:     "#555555",
	}
	css := BuildThemeCSS(c, "")
	for _, hex := range []string{"#ff0000", "#111111", "#222222", "#333333", "#eeeeee", "#999999", "#555555"} {
		if !strings.Contains(css, hex) {
			t.Errorf("output missing color value %s", hex)
		}
	}
}

func TestBuildThemeCSS_StripsTemplateDefineColors(t *testing.T) {
	template := "@define-color z13-accent #old;\n.drawer { color: @z13-text; }\n"
	css := BuildThemeCSS(DefaultColors, template)

	// The template's @define-color line should be stripped.
	lines := strings.Split(css, "\n")
	oldFound := false
	for _, l := range lines {
		if strings.Contains(l, "#old") {
			oldFound = true
		}
	}
	if oldFound {
		t.Error("template @define-color was not stripped")
	}

	// But the template rule should remain.
	if !strings.Contains(css, ".drawer") {
		t.Error("template rule was incorrectly stripped")
	}
}

func TestBuildThemeCSS_PreservesTemplateRules(t *testing.T) {
	template := ".drawer { background: @z13-bg; }\n.bottom-bar { margin: 4px; }\n"
	css := BuildThemeCSS(DefaultColors, template)
	if !strings.Contains(css, ".drawer") {
		t.Error("missing .drawer rule")
	}
	if !strings.Contains(css, ".bottom-bar") {
		t.Error("missing .bottom-bar rule")
	}
}

func TestStripDefineColors(t *testing.T) {
	input := "@define-color z13-accent #cc0000;\n.foo { color: red; }\n@define-color z13-bg #1a1a1a;\n.bar { }\n"
	got := StripDefineColors(input)
	if strings.Contains(got, "@define-color") {
		t.Error("@define-color lines were not stripped")
	}
	if !strings.Contains(got, ".foo") {
		t.Error(".foo rule was incorrectly stripped")
	}
	if !strings.Contains(got, ".bar") {
		t.Error(".bar rule was incorrectly stripped")
	}
}

func TestStripDefineColors_IndentedLine(t *testing.T) {
	input := "  @define-color z13-accent #cc0000;\n.foo { }\n"
	got := StripDefineColors(input)
	if strings.Contains(got, "@define-color") {
		t.Error("indented @define-color line was not stripped")
	}
}

func TestStripDefineColors_EmptyInput(t *testing.T) {
	got := StripDefineColors("")
	if got != "\n" { // strings.Split("", "\n") produces one empty element
		t.Errorf("unexpected output for empty input: %q", got)
	}
}

func TestBuildThemeCSS_EmptyTemplate(t *testing.T) {
	css := BuildThemeCSS(DefaultColors, "")
	if !strings.Contains(css, "@define-color z13-accent") {
		t.Error("even with empty template, @define-color lines should be present")
	}
}
