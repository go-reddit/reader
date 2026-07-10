// Package ui renders a Reddit feed into an RGBA pixel buffer using the
// go-widgets toolkit. It has no build tag so the layout + hit-testing logic
// is exercised by native `go test` against a plain []byte, while the wasm
// front-end (cmd/front) drives the same Scene against a browser <canvas>.
package ui

import (
	"fmt"
	"strings"

	"github.com/go-reddit/reddit"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Surface dimensions of the reader canvas, in logical pixels.
const (
	SurfaceW = 900
	SurfaceH = 660
)

// Layout constants.
const (
	headerH = 40
	footerH = 20
	pad     = 8
	scoreW  = 60 // left column width for the score badge
	lineH   = 12 // baseline step between text lines
	rowH    = 66 // fixed height of one post row
	rowGap  = 6
)

// Scene is the mutable reader state: which feed is shown, the posts in it,
// scroll position, and a transient status line. It is safe to construct with
// the zero value plus [NewScene]; all rendering goes through [Scene.Draw].
type Scene struct {
	W, H int

	theme     *toolkit.Theme
	Subreddit string // e.g. "golang" ("" => front page)
	Sort      string // e.g. "hot"
	Posts     []reddit.Post
	Status    string // footer message ("Loading…", error text, "42 posts")
	ScrollY   int

	// rows records each post's on-content rectangle (pre-scroll) so
	// HitTest can map a click back to a post without re-deriving layout.
	rows     []rowLayout
	contentH int
}

type rowLayout struct {
	top  int // y in content space (before ScrollY is applied)
	post reddit.Post
}

// NewScene returns a Scene sized to the standard surface with the default
// light theme applied.
func NewScene() *Scene {
	return &Scene{
		W:     SurfaceW,
		H:     SurfaceH,
		theme: toolkit.DefaultLight(),
		Sort:  "hot",
	}
}

// Minimum sensible surface, so a tiny window never produces a degenerate
// (zero/negative) viewport.
const (
	MinW = 320
	MinH = 240
)

// Display-zoom bounds and step. The front-end divides the canvas size by the
// zoom factor to get the logical surface it renders into, so a larger zoom
// yields a smaller logical buffer that the canvas magnifies — bigger text and
// cards, fewer visible at once.
const (
	MinZoom  = 0.5
	MaxZoom  = 3.0
	ZoomStep = 0.1
)

// ClampZoom constrains z to [MinZoom, MaxZoom].
func ClampZoom(z float64) float64 {
	switch {
	case z < MinZoom:
		return MinZoom
	case z > MaxZoom:
		return MaxZoom
	default:
		return z
	}
}

// StepZoom moves the zoom one step in the given direction (+1 in, -1 out, 0 no
// change), rounded to the step grid and clamped. Any other dir is treated as 0.
func StepZoom(z float64, dir int) float64 {
	switch {
	case dir > 0:
		z += ZoomStep
	case dir < 0:
		z -= ZoomStep
	}
	// Round to the nearest ZoomStep so repeated presses stay on a clean grid.
	z = float64(int(z/ZoomStep+0.5)) * ZoomStep
	return ClampZoom(z)
}

// LogicalSize maps a canvas dimension in device/CSS pixels to the logical
// surface dimension at the given zoom (canvasPx / zoom), never below 1.
func LogicalSize(canvasPx int, zoom float64) int {
	if zoom <= 0 {
		zoom = 1
	}
	n := int(float64(canvasPx)/zoom + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

// Resize sets the surface to w×h logical pixels (clamped to a minimum) and
// re-clamps the scroll position so it stays valid for the new viewport. The
// front-end calls this whenever the window/canvas changes size, then redraws;
// the layout adapts (wider cards, more rows visible) rather than stretching.
func (s *Scene) Resize(w, h int) {
	if w < MinW {
		w = MinW
	}
	if h < MinH {
		h = MinH
	}
	s.W, s.H = w, h
	s.layout()
	if m := s.MaxScroll(); s.ScrollY > m {
		s.ScrollY = m
	}
}

// SetTheme swaps the palette (e.g. DefaultDark). A nil theme is ignored.
func (s *Scene) SetTheme(t *toolkit.Theme) {
	if t != nil {
		s.theme = t
	}
}

// SetFeed records the current subreddit/sort selection so the header reflects
// it. subreddit "" renders as the front page.
func (s *Scene) SetFeed(subreddit, sort string) {
	s.Subreddit = strings.TrimPrefix(strings.TrimSpace(subreddit), "r/")
	if sort == "" {
		sort = "hot"
	}
	s.Sort = sort
}

// SetPosts replaces the visible posts and resets the scroll position.
func (s *Scene) SetPosts(posts []reddit.Post) {
	s.Posts = posts
	s.ScrollY = 0
}

// FeedName returns a human-readable label for the current feed, e.g.
// "r/golang · hot" or "front page · new". Used by the front-end status line.
func (s *Scene) FeedName() string { return s.feedLabel() }

// feedLabel is the human-readable name of the current feed.
func (s *Scene) feedLabel() string {
	if s.Subreddit == "" {
		return "front page · " + s.Sort
	}
	return "r/" + s.Subreddit + " · " + s.Sort
}

// viewportH is the height of the scrollable list area between header and
// footer.
func (s *Scene) viewportH() int { return s.H - headerH - footerH }

// layout (re)computes the per-row rectangles and total content height. Called
// at the top of Draw and by HitTest/Scroll so geometry stays in one place.
func (s *Scene) layout() {
	s.rows = s.rows[:0]
	y := pad
	for _, p := range s.Posts {
		s.rows = append(s.rows, rowLayout{top: y, post: p})
		y += rowH + rowGap
	}
	s.contentH = y
}

// MaxScroll is the largest valid ScrollY for the current content.
func (s *Scene) MaxScroll() int {
	if m := s.contentH - s.viewportH(); m > 0 {
		return m
	}
	return 0
}

// Scroll adjusts the vertical scroll by dy pixels, clamped to the content.
// Returns true if the position actually changed (so callers can skip a
// redraw when it didn't).
func (s *Scene) Scroll(dy int) bool {
	s.layout()
	old := s.ScrollY
	s.ScrollY += dy
	if s.ScrollY < 0 {
		s.ScrollY = 0
	}
	if m := s.MaxScroll(); s.ScrollY > m {
		s.ScrollY = m
	}
	return s.ScrollY != old
}

// HitTest maps a click at (x,y) to a post. It returns the post and true when
// the click lands on a row inside the viewport, or a zero Post and false
// otherwise (header, footer, gaps, empty space).
func (s *Scene) HitTest(x, y int) (reddit.Post, bool) {
	if y < headerH || y >= s.H-footerH {
		return reddit.Post{}, false
	}
	s.layout()
	cy := y - headerH + s.ScrollY // to content space
	for _, r := range s.rows {
		if cy >= r.top && cy < r.top+rowH && x >= pad && x < s.W-pad {
			return r.post, true
		}
	}
	return reddit.Post{}, false
}

// Draw paints the whole scene into buf, an RGBA buffer of s.W*s.H*4 bytes.
func (s *Scene) Draw(buf []byte) {
	s.layout()
	p := painter.NewPixelPainter(buf, s.W, s.H)
	th := s.theme

	// Background.
	p.FillRect(painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)

	// --- post list (drawn first; header/footer overpaint any overflow) ---
	listTop := headerH
	for _, r := range s.rows {
		screenY := listTop + r.top - s.ScrollY
		if screenY+rowH < headerH || screenY >= s.H-footerH {
			continue // fully outside the viewport
		}
		s.drawPost(p, r.post, screenY)
	}

	// Empty / loading state.
	if len(s.Posts) == 0 {
		msg := "No posts loaded."
		toolkit.DrawText(p, (s.W-toolkit.TextWidth(msg))/2, s.H/2, msg, th.OnBackground)
	}

	// --- header chrome ---
	p.FillRect(painter.Rect{X: 0, Y: 0, W: s.W, H: headerH}, th.Accent)
	onAccent := th.Background
	if v, ok := th.Extra["OnAccent"]; ok {
		onAccent = v
	}
	toolkit.DrawText(p, pad, (headerH-toolkit.GlyphHeight)/2, "go-reddit reader", onAccent)
	feed := s.feedLabel()
	toolkit.DrawText(p, s.W-pad-toolkit.TextWidth(feed), (headerH-toolkit.GlyphHeight)/2, feed, onAccent)

	// --- footer status bar ---
	fy := s.H - footerH
	p.FillRect(painter.Rect{X: 0, Y: fy, W: s.W, H: footerH}, th.SurfaceAlt)
	status := s.Status
	if status == "" {
		status = fmt.Sprintf("%d posts", len(s.Posts))
	}
	toolkit.DrawText(p, pad, fy+(footerH-toolkit.GlyphHeight)/2, status, th.OnSurface)
	hint := "click to open · scroll · Cmd +/- zoom"
	toolkit.DrawText(p, s.W-pad-toolkit.TextWidth(hint), fy+(footerH-toolkit.GlyphHeight)/2, hint, mute(th.OnSurface, th.SurfaceAlt))
}

// drawPost renders one post card at screen y.
func (s *Scene) drawPost(p *painter.PixelPainter, post reddit.Post, y int) {
	th := s.theme
	card := painter.Rect{X: pad, Y: y, W: s.W - 2*pad, H: rowH}
	p.FillRect(card, th.Surface)
	p.StrokeRect(card, th.Border, 1)

	// Score badge (left column).
	scoreStr := formatScore(post.Score)
	sx := card.X + (scoreW-toolkit.TextWidth(scoreStr))/2
	toolkit.DrawText(p, sx, y+14, scoreStr, th.Accent)
	pts := "pts"
	toolkit.DrawText(p, card.X+(scoreW-toolkit.TextWidth(pts))/2, y+26, pts, mute(th.OnSurface, th.Surface))

	// Title (up to two wrapped lines).
	tx := card.X + scoreW
	availW := card.W - scoreW - pad
	lines := wrapText(post.Title, availW/toolkit.GlyphAdvance, 2)
	for i, ln := range lines {
		toolkit.DrawText(p, tx, y+10+i*lineH, ln, th.OnSurface)
	}

	// Meta line.
	meta := metaLine(post)
	toolkit.DrawText(p, tx, y+rowH-16, meta, mute(th.OnSurface, th.Surface))
	if post.Flair != "" {
		flair := "[" + post.Flair + "]"
		toolkit.DrawText(p, card.X+card.W-pad-toolkit.TextWidth(flair), y+rowH-16, flair, th.Accent)
	}
}

// metaLine builds the "u/author · N comments · domain" descriptor.
func metaLine(p reddit.Post) string {
	parts := []string{"u/" + p.Author, fmt.Sprintf("%d comments", p.NumComments)}
	if p.Domain != "" {
		parts = append(parts, p.Domain)
	}
	return strings.Join(parts, "  ·  ")
}

// formatScore renders a vote count compactly (1200 -> "1.2k").
func formatScore(n int) string {
	switch {
	case n >= 100000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// wrapText greedily wraps text to at most maxLines lines of maxChars each.
// Words longer than a line are hard-cut (never producing a mid-word space).
// When content is dropped the final kept line ends in an ellipsis. The result
// never contains more than maxLines lines.
func wrapText(text string, maxChars, maxLines int) []string {
	text = strings.TrimSpace(text)
	if text == "" || maxChars < 1 || maxLines < 1 {
		return nil
	}
	words := strings.Fields(text)
	var lines []string
	line := ""
	for i := 0; i < len(words); {
		w := words[i]
		space := 0
		if line != "" {
			space = 1
		}
		if len(line)+space+len(w) <= maxChars {
			if line != "" {
				line += " "
			}
			line += w
			i++
			continue
		}
		if line == "" {
			// A word wider than a whole line: take a full line's worth
			// and leave the remainder to be processed next.
			line = w[:maxChars]
			words[i] = w[maxChars:]
		}
		lines = append(lines, line)
		line = ""
		if len(lines) == maxLines {
			return ellipsize(lines, maxChars)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// ellipsize trims the final line to fit maxChars including a trailing "…",
// signalling that content was dropped. lines must be non-empty.
func ellipsize(lines []string, maxChars int) []string {
	last := lines[len(lines)-1]
	if len(last) > maxChars-1 {
		last = last[:maxChars-1]
	}
	lines[len(lines)-1] = strings.TrimRight(last, " ") + "…"
	return lines
}

// mute blends fg 55% toward bg to produce a lower-contrast label colour for
// secondary text, without needing a dedicated theme field.
func mute(fg, bg toolkit.RGBA) toolkit.RGBA {
	blend := func(a, b uint8) uint8 { return uint8((int(a)*45 + int(b)*55) / 100) }
	return toolkit.RGBA{R: blend(fg.R, bg.R), G: blend(fg.G, bg.G), B: blend(fg.B, bg.B), A: 0xFF}
}
