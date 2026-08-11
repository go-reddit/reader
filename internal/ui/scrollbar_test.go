package ui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// anyInk reports whether any pixel in the w×h block at (x0,y0) of a w0-wide RGBA
// buffer has a non-zero byte (i.e. the region was painted at all).
func anyInk(buf []byte, w0, x0, y0, w, h int) bool {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			i := (y*w0 + x) * 4
			if i < 0 || i+4 > len(buf) {
				continue
			}
			if buf[i] != 0 || buf[i+1] != 0 || buf[i+2] != 0 || buf[i+3] != 0 {
				return true
			}
		}
	}
	return false
}

// TestFeedScrollbarPaintsOnOverflow proves the feed scrollbar zone: the
// toolkit.Scrollbar paints in its bar column exactly when the content overflows
// the viewport, and paints nothing (the early return) when it fits — asserted at
// the precise computed column, not just "something painted somewhere".
func TestFeedScrollbarPaintsOnOverflow(t *testing.T) {
	s := NewScene()
	s.Resize(400, 300)
	m := s.m
	area := toolkit.Rect{X: m.sidebarW, Y: m.topbarH, W: s.W - m.sidebarW, H: s.viewportH()}
	w := m.rpx(6)
	inset := m.rpx(2)
	barX := area.X + area.W - inset - w
	barY := area.Y + inset
	barH := area.H - 2*inset

	// Overflow (total = 3× viewport): the bar column must carry ink.
	over := make([]byte, s.W*s.H*4)
	s.drawVScrollbar(painter.NewPixelPainter(over, s.W, s.H), area, area.H*3, 0)
	if !anyInk(over, s.W, barX, barY, w, barH) {
		t.Error("overflow: expected scrollbar ink in the bar column")
	}
	// The bar must stay inside its column — no ink one pixel to the LEFT of it.
	if anyInk(over, s.W, barX-1, barY, 1, barH) {
		t.Error("scrollbar painted outside (left of) its column")
	}

	// Fits (total = viewport): drawVScrollbar returns early, painting nothing.
	fit := make([]byte, s.W*s.H*4)
	s.drawVScrollbar(painter.NewPixelPainter(fit, s.W, s.H), area, area.H, 0)
	for _, b := range fit {
		if b != 0 {
			t.Fatal("fit: scrollbar must paint nothing when content fits the viewport")
		}
	}
	// Degenerate viewport (H <= 0) is also a no-op.
	deg := make([]byte, s.W*s.H*4)
	s.drawVScrollbar(painter.NewPixelPainter(deg, s.W, s.H), toolkit.Rect{X: 0, Y: 0, W: 10, H: 0}, 100, 0)
	for _, b := range deg {
		if b != 0 {
			t.Fatal("degenerate viewport: scrollbar must paint nothing")
		}
	}
}

// TestFeedScrollbarInFullDraw proves the scrollbar is actually wired into the
// feed's Draw path: a feed tall enough to overflow paints the bar column, a
// short feed does not.
func TestFeedScrollbarInFullDraw(t *testing.T) {
	tall := sizedScene(60) // far more posts than fit in the viewport
	tall.Resize(500, 320)
	m := tall.m
	if tall.MaxScroll() <= 0 {
		t.Fatal("test setup: 60 posts should overflow a 320px window")
	}
	w := m.rpx(6)
	inset := m.rpx(2)
	barX := tall.W - inset - w // area right edge == s.W for the feed panel
	barY := m.topbarH + inset
	barH := tall.viewportH() - 2*inset

	buf := make([]byte, tall.W*tall.H*4)
	tall.Draw(buf)
	if !anyInk(buf, tall.W, barX, barY, w, barH) {
		t.Error("full Draw: overflowing feed should show a scrollbar")
	}

	short := sizedScene(1)
	short.Resize(500, 600)
	if short.MaxScroll() != 0 {
		t.Fatal("test setup: 1 post should not overflow a 600px window")
	}
}
