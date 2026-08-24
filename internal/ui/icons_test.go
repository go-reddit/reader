package ui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func testPainter(w, h int) painter.Painter {
	return painter.NewPixelPainter(make([]byte, w*h*4), w, h)
}

func TestIconInset(t *testing.T) {
	// Square slot: the inset is fraction% of the side, centred.
	got := iconInset(toolkit.Rect{X: 0, Y: 0, W: 100, H: 100}, 80)
	if got.W != 80 || got.H != 80 || got.X != 10 || got.Y != 10 {
		t.Errorf("square inset => %+v", got)
	}
	// Wider than tall: the shorter side (height) drives the square (r.H < d branch).
	got = iconInset(toolkit.Rect{X: 0, Y: 0, W: 200, H: 40}, 50)
	if got.W != 20 || got.H != 20 {
		t.Errorf("landscape inset => %+v", got)
	}
	// Degenerate slot: the size clamps up to at least one pixel (d < 1 branch).
	got = iconInset(toolkit.Rect{X: 5, Y: 5, W: 1, H: 1}, 50)
	if got.W != 1 || got.H != 1 {
		t.Errorf("min-clamp inset => %+v", got)
	}
}

func TestIconsResolve(t *testing.T) {
	p := testPainter(64, 64)
	r := toolkit.Rect{X: 0, Y: 0, W: 32, H: 32}
	for _, name := range []string{iconSearch, iconSettings, iconLogIn, iconLogOut} {
		if !drawIcon(p, r, name, toolkit.RGBA{R: 0, G: 0, B: 0, A: 255}) {
			t.Errorf("icon %q did not resolve", name)
		}
	}
	// An unknown name resolves to nothing (draws no glyph, reports false).
	if drawIcon(p, r, "definitely-not-an-icon", toolkit.RGBA{A: 255}) {
		t.Error("unknown icon should not resolve")
	}
	// The SearchEntry hook wraps drawIcon; exercise it directly.
	drawSearchIcon(p, r, toolkit.RGBA{A: 255})
}
