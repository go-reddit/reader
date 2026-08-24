package ui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// The shell's grounds, bands, dividers, pills, search field and post cards are
// composed from toolkit.Backdrop rather than hand-drawn painter rectangles, so
// every surface in the app is a widget. Each helper is a 1:1 stand-in for the
// painter call it replaces, drawing the exact same (optionally rounded,
// optionally stroked) rectangle.

// fillBox fills r with a solid colour — the widget form of p.FillRect.
func fillBox(p painter.Painter, th *toolkit.Theme, r toolkit.Rect, fill toolkit.RGBA) {
	b := &toolkit.Backdrop{Fill: fill}
	b.SetBounds(r)
	b.Draw(p, th)
}

// fillRoundBox fills r with a solid colour, corners rounded by radius — the
// widget form of p.FillRoundRect.
func fillRoundBox(p painter.Painter, th *toolkit.Theme, r toolkit.Rect, radius int, fill toolkit.RGBA) {
	b := &toolkit.Backdrop{Fill: fill, Radius: radius}
	b.SetBounds(r)
	b.Draw(p, th)
}
