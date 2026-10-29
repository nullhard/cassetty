package ui

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func sameColor(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8
}

func TestRampKeepsBothEnds(t *testing.T) {
	stops := []color.Color{c("#ff0000"), c("#ffff00"), c("#00ff00"), c("#0000ff"), c("#ff00ff")}
	for n := 2; n <= 12; n++ {
		got := ramp(n, stops...)
		if len(got) != n {
			t.Fatalf("ramp(%d): %d colors", n, len(got))
		}
		if !sameColor(got[0], stops[0]) || !sameColor(got[n-1], stops[len(stops)-1]) {
			t.Errorf("ramp(%d) runs from %v to %v", n, got[0], got[n-1])
		}
	}
	if got := ramp(3, stops...); !sameColor(got[1], stops[2]) {
		t.Errorf("ramp(3) middle = %v, want the middle stop", got[1])
	}
	if got := ramp(1, stops...); len(got) != 1 || !sameColor(got[0], stops[0]) {
		t.Errorf("ramp(1) = %v", got)
	}
}

func TestLightThemePaintsBackground(t *testing.T) {
	m := newTestModel(t, testRoot(t)) // PaintBackground is off
	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for i, th := range themes {
		m.setTheme(i)
		bg := m.View().BackgroundColor
		switch {
		case th.Light && (bg == nil || !sameColor(bg, th.Bg)):
			t.Errorf("%s: light theme left the background unpainted", th.Name)
		case !th.Light && bg != nil:
			t.Errorf("%s: painted the background although it is off", th.Name)
		}
	}
}

func TestThemeNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, th := range themes {
		if seen[th.Name] {
			t.Errorf("duplicate theme %q", th.Name)
		}
		seen[th.Name] = true
		for _, r := range [][]color.Color{th.Band, th.Meter} {
			if len(r) == 1 {
				t.Errorf("%s: a ramp needs at least two colors", th.Name)
			}
		}
	}
	def := themes[themeIndex(DefaultTheme)]
	if def.Name != DefaultTheme {
		t.Fatalf("default theme %q does not exist", DefaultTheme)
	}
	for _, name := range []string{"", "no-such-theme"} {
		if got := themes[themeIndex(name)].Name; got != DefaultTheme {
			t.Errorf("themeIndex(%q) = %s, want the default %s", name, got, DefaultTheme)
		}
	}
}
