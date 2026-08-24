package ui

// Real vector icons, drawn from the Iconoir set via github.com/go-iconoir/iconoir
// rather than hand-plotted glyphs or Unicode stand-ins. Every icon the reader
// paints — the search magnifier, the sidebar's account + settings entries — goes
// through iconoir.Draw so it stays crisp at any zoom / Retina scale and matches
// the sibling go-news-reader shell. Each helper is a thin wrapper that centres a
// square glyph in the widget slot it is handed and blits it in the given ink.

import (
	"github.com/go-iconoir/iconoir"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// iconInset returns the largest centred square that is fraction/100 of the
// shorter side of r, so a glyph never touches the edges of the slot it fills.
func iconInset(r toolkit.Rect, fraction int) toolkit.Rect {
	d := r.W
	if r.H < d {
		d = r.H
	}
	d = d * fraction / 100
	if d < 1 {
		d = 1
	}
	return toolkit.Rect{X: r.X + (r.W-d)/2, Y: r.Y + (r.H-d)/2, W: d, H: d}
}

// drawIcon centres the named Iconoir glyph in r and blits it in ink. It reports
// whether the name resolved (an unknown name draws nothing) so callers can fall
// back to text; every name the reader uses is a real Iconoir icon, verified by
// TestIconsResolve.
func drawIcon(p painter.Painter, r toolkit.Rect, name string, ink toolkit.RGBA) bool {
	return iconoir.Draw(p, iconInset(r, 82), name, ink)
}

// Icon names used across the reader (all Iconoir "regular" set members).
const (
	iconSearch   = "search"   // topbar filter field magnifier
	iconSettings = "settings" // sidebar Settings entry (gear)
	iconLogIn    = "log-in"   // sidebar account entry, logged out
	iconLogOut   = "log-out"  // sidebar account entry, logged in
)

// drawSearchIcon is the SearchEntry.Icon hook: it paints the magnifier in the
// widget's leading prefix slot, in the field's own ink.
func drawSearchIcon(p painter.Painter, r toolkit.Rect, ink toolkit.RGBA) {
	drawIcon(p, r, iconSearch, ink)
}
