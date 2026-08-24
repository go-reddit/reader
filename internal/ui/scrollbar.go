package ui

// A shared vertical scrollbar for the reader's scrollable panel (the feed card
// list). It is the go-widgets toolkit.Scrollbar widget — one reusable primitive
// drawn down the right edge of the view when its content overflows the viewport,
// so the reader can see where it sits in a feed that can grow arbitrarily tall,
// instead of the previous invisible scroll with no position indicator.

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// drawVScrollbar draws a vertical toolkit.Scrollbar for a panel when its content
// overflows the viewport (else nothing). The bar is inset a couple of pixels
// from the panel's right edge so it never paints over the neighbouring chrome.
func (s *Scene) drawVScrollbar(p *painter.PixelPainter, area toolkit.Rect, total, offset int) {
	view := area.H
	if view <= 0 || total <= view {
		return
	}
	m := s.m
	w := m.rpx(6)
	inset := m.rpx(2)
	right := area.X + area.W - inset
	sb := &toolkit.Scrollbar{Total: total, Viewport: view}
	sb.Offset().Set(offset)
	sb.SetBounds(toolkit.Rect{X: right - w, Y: area.Y + inset, W: w, H: area.H - 2*inset})
	sb.Draw(p, s.theme) // paints a clearly-visible muted-grey thumb
}
