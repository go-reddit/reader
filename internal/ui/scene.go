// Package ui renders the reader — a Reddit-style topbar + sidebar + feed —
// into an RGBA pixel buffer. Chrome and cards are drawn with the go-widgets
// painter; text is anti-aliased TrueType (see text.go) so it stays clean at
// any zoom / Retina scale. The package has no build tag, so its layout, hit-
// testing and rendering are exercised by native `go test`; the wasm front-end
// (cmd/front) drives the same Scene against a browser <canvas>.
package ui

import (
	"fmt"
	"image"
	"strings"

	"github.com/go-reddit/reddit"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Minimum sensible surface, so a tiny window never produces a degenerate
// (zero/negative) viewport.
const (
	MinW = 360
	MinH = 240
)

// Display-zoom bounds and step, applied by the front-end as a scale factor on
// the whole UI (bigger fonts + metrics, not a stretched buffer).
const (
	MinZoom  = 0.5
	MaxZoom  = 3.0
	ZoomStep = 0.1
)

// Sorts is the ordered set of listing sorts shown as topbar tabs.
var Sorts = []string{"hot", "new", "top", "rising"}

// DefaultFeeds seeds the sidebar's bookmark list. "" is the front page.
var DefaultFeeds = []string{"", "golang", "rust", "programming", "webdev", "linux", "macos", "science"}

// HitKind classifies what a click landed on.
type HitKind int

const (
	HitNone HitKind = iota
	HitPost         // a feed post (open it)
	HitFeed         // a sidebar bookmark (switch subreddit)
	HitSort         // a topbar sort tab (switch sort)
)

// Hit is the result of [Scene.HitTest]: what was clicked and its payload.
type Hit struct {
	Kind HitKind
	Post reddit.Post // Kind == HitPost
	Feed string      // Kind == HitFeed ("" = front page)
	Sort string      // Kind == HitSort
}

// Scene is the mutable reader state.
type Scene struct {
	W, H int

	theme     *toolkit.Theme
	Subreddit string // selected subreddit ("" => front page)
	Sort      string
	Feeds     []string // sidebar bookmarks
	Posts     []reddit.Post
	Status    string
	ScrollY   int
	Scale     float64 // display scale (zoom × devicePixelRatio); 0 => 1

	m        metrics
	tabs     []tabHit
	side     []sideHit
	rows     []rowLayout
	contentH int
}

type rowLayout struct {
	top  int // y in feed-content space (before ScrollY)
	post reddit.Post
}
type tabHit struct {
	rect toolkit.Rect
	sort string
}
type sideHit struct {
	rect toolkit.Rect
	feed string
}

// NewScene returns a Scene at the default size with the light theme and the
// default bookmark list.
func NewScene() *Scene {
	return &Scene{
		W:     900,
		H:     660,
		theme: toolkit.DefaultLight(),
		Sort:  "hot",
		Scale: 1,
		Feeds: append([]string(nil), DefaultFeeds...),
	}
}

// SetTheme swaps the palette (e.g. DefaultDark). A nil theme is ignored.
func (s *Scene) SetTheme(t *toolkit.Theme) {
	if t != nil {
		s.theme = t
	}
}

// SetFeed records the current subreddit/sort selection (and makes sure the
// subreddit is present in the sidebar). subreddit "" is the front page.
func (s *Scene) SetFeed(subreddit, sort string) {
	s.Subreddit = strings.TrimPrefix(strings.TrimSpace(subreddit), "r/")
	if sort == "" {
		sort = "hot"
	}
	s.Sort = sort
	if s.Subreddit != "" && !contains(s.Feeds, s.Subreddit) {
		s.Feeds = append(s.Feeds, s.Subreddit)
	}
}

// SetPosts replaces the visible posts and resets the scroll position.
func (s *Scene) SetPosts(posts []reddit.Post) {
	s.Posts = posts
	s.ScrollY = 0
}

// FeedName returns a human-readable label for the current feed.
func (s *Scene) FeedName() string { return s.feedLabel() }

func (s *Scene) feedLabel() string {
	if s.Subreddit == "" {
		return "Front page · " + s.Sort
	}
	return "r/" + s.Subreddit + " · " + s.Sort
}

func (s *Scene) effScale() float64 {
	if s.Scale <= 0 {
		return 1
	}
	return s.Scale
}

// Resize sets the surface to w×h pixels (clamped) and re-clamps scroll.
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

func (s *Scene) viewportH() int { return s.H - s.m.topbarH - s.m.footerH }

// MaxScroll is the largest valid ScrollY for the current content.
func (s *Scene) MaxScroll() int {
	if m := s.contentH - s.viewportH(); m > 0 {
		return m
	}
	return 0
}

// Scroll adjusts the vertical scroll by dy, clamped. Returns whether it moved.
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

// layout recomputes metrics + the tab/sidebar/feed rectangles.
func (s *Scene) layout() {
	s.m = computeMetrics(s.effScale())
	m := s.m

	// Topbar tabs, laid out after the app title.
	s.tabs = s.tabs[:0]
	x := 2*m.pad + m.header.width(appTitle)
	for _, srt := range Sorts {
		w := m.tab.width(srt) + 2*m.tabPad
		s.tabs = append(s.tabs, tabHit{rect: toolkit.Rect{X: x, Y: 0, W: w, H: m.topbarH}, sort: srt})
		x += w
	}

	// Sidebar bookmarks, below a "FEEDS" header row.
	s.side = s.side[:0]
	sy := m.topbarH + m.sideItemH
	for _, f := range s.Feeds {
		s.side = append(s.side, sideHit{rect: toolkit.Rect{X: 0, Y: sy, W: m.sidebarW, H: m.sideItemH}, feed: f})
		sy += m.sideItemH
	}

	// Feed rows.
	s.rows = s.rows[:0]
	y := m.pad
	for _, p := range s.Posts {
		s.rows = append(s.rows, rowLayout{top: y, post: p})
		y += m.rowH + m.rowGap
	}
	s.contentH = y
}

// HitTest maps a click to an action (post/feed/sort) or HitNone.
func (s *Scene) HitTest(x, y int) Hit {
	s.layout()
	m := s.m
	switch {
	case y < m.topbarH:
		for _, t := range s.tabs {
			if t.rect.Contains(x, y) {
				return Hit{Kind: HitSort, Sort: t.sort}
			}
		}
		return Hit{}
	case y >= s.H-m.footerH:
		return Hit{}
	case x < m.sidebarW:
		for _, it := range s.side {
			if it.rect.Contains(x, y) {
				return Hit{Kind: HitFeed, Feed: it.feed}
			}
		}
		return Hit{}
	default:
		cy := y - m.topbarH + s.ScrollY
		for _, r := range s.rows {
			if cy >= r.top && cy < r.top+m.rowH && x >= m.sidebarW+m.pad && x < s.W-m.pad {
				return Hit{Kind: HitPost, Post: r.post}
			}
		}
		return Hit{}
	}
}

const appTitle = "go-reddit"

// Draw paints the whole scene into buf (s.W*s.H*4 RGBA bytes).
func (s *Scene) Draw(buf []byte) {
	s.layout()
	m := s.m
	p := painter.NewPixelPainter(buf, s.W, s.H)
	img := &image.RGBA{Pix: buf, Stride: s.W * 4, Rect: image.Rect(0, 0, s.W, s.H)}
	th := s.theme
	onAccent := th.Background
	if v, ok := th.Extra["OnAccent"]; ok {
		onAccent = v
	}
	muteS := mute(th.OnSurface, th.Surface)

	p.FillRect(painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)

	// --- feed (drawn first; chrome overpaints any scroll overflow) ---
	feedTop := m.topbarH
	for _, r := range s.rows {
		screenY := feedTop + r.top - s.ScrollY
		if screenY+m.rowH < feedTop || screenY >= s.H-m.footerH {
			continue
		}
		s.drawPost(p, img, r.post, screenY)
	}
	if len(s.Posts) == 0 {
		msg := "No posts loaded."
		cx := m.sidebarW + (s.W-m.sidebarW-m.title.width(msg))/2
		m.title.draw(img, cx, s.H/2, msg, th.OnBackground)
	}

	// --- sidebar ---
	p.FillRect(painter.Rect{X: 0, Y: m.topbarH, W: m.sidebarW, H: s.H - m.topbarH - m.footerH}, th.SurfaceAlt)
	m.side.draw(img, m.pad, m.topbarH+(m.sideItemH-m.side.height)/2, "FEEDS", muteS)
	for _, it := range s.side {
		label := "Front page"
		if it.feed != "" {
			label = "r/" + it.feed
		}
		selected := it.feed == s.Subreddit
		col := th.OnSurface
		if selected {
			p.FillRect(painter.Rect{X: it.rect.X, Y: it.rect.Y, W: it.rect.W, H: it.rect.H}, th.Surface)
			p.FillRect(painter.Rect{X: 0, Y: it.rect.Y, W: m.rpx(3), H: it.rect.H}, th.Accent)
			col = th.Accent
		}
		m.side.draw(img, m.pad, it.rect.Y+(m.sideItemH-m.side.height)/2, label, col)
	}
	p.FillRect(painter.Rect{X: m.sidebarW - 1, Y: m.topbarH, W: 1, H: s.H - m.topbarH - m.footerH}, th.Border)

	// --- topbar ---
	p.FillRect(painter.Rect{X: 0, Y: 0, W: s.W, H: m.topbarH}, th.Accent)
	m.header.draw(img, m.pad, (m.topbarH-m.header.height)/2, appTitle, onAccent)
	for _, t := range s.tabs {
		active := t.sort == s.Sort
		col := mute(onAccent, th.Accent)
		if active {
			col = onAccent
			p.FillRect(painter.Rect{X: t.rect.X, Y: m.topbarH - m.rpx(3), W: t.rect.W, H: m.rpx(3)}, onAccent)
		}
		m.tab.draw(img, t.rect.X+m.tabPad, (m.topbarH-m.tab.height)/2, t.sort, col)
	}
	m.header.drawRight(img, s.W-m.pad, (m.topbarH-m.header.height)/2, s.feedLabel(), onAccent)

	// --- footer ---
	fy := s.H - m.footerH
	p.FillRect(painter.Rect{X: 0, Y: fy, W: s.W, H: m.footerH}, th.SurfaceAlt)
	status := s.Status
	if status == "" {
		status = fmt.Sprintf("%d posts", len(s.Posts))
	}
	m.meta.draw(img, m.pad, fy+(m.footerH-m.meta.height)/2, status, th.OnSurface)
	m.meta.drawRight(img, s.W-m.pad, fy+(m.footerH-m.meta.height)/2, "click a feed · scroll · Cmd +/- zoom", muteS)
}

// drawPost renders one post card at screen y.
func (s *Scene) drawPost(p *painter.PixelPainter, img *image.RGBA, post reddit.Post, y int) {
	m := s.m
	th := s.theme
	rad := m.rpx(6)
	card := painter.Rect{X: m.sidebarW + m.pad, Y: y, W: s.W - m.sidebarW - 2*m.pad, H: m.rowH}
	p.FillRoundRect(card, rad, th.Surface)
	p.StrokeRoundRect(card, rad, th.Border, 1)

	// Score badge (left column).
	scoreStr := formatScore(post.Score)
	m.score.draw(img, card.X+(m.scoreW-m.score.width(scoreStr))/2, y+m.pad, scoreStr, th.Accent)
	m.meta.draw(img, card.X+(m.scoreW-m.meta.width("pts"))/2, y+m.pad+m.score.height, "pts", mute(th.OnSurface, th.Surface))

	// Title (up to two wrapped lines).
	tx := card.X + m.scoreW
	availW := card.W - m.scoreW - m.pad
	for i, ln := range wrapText(m.title, post.Title, availW, 2) {
		m.title.draw(img, tx, y+m.pad+i*m.title.height, ln, th.OnSurface)
	}

	// Meta line, with optional flair on the right.
	metaY := y + m.rowH - m.pad - m.meta.height
	m.meta.draw(img, tx, metaY, metaLine(post), mute(th.OnSurface, th.Surface))
	if post.Flair != "" {
		m.meta.drawRight(img, card.X+card.W-m.pad, metaY, "["+post.Flair+"]", th.Accent)
	}
}

// --- metrics ---------------------------------------------------------------

type metrics struct {
	scale                                                    float64
	pad, topbarH, sidebarW, footerH, rowH, rowGap, scoreW    int
	sideItemH, tabPad                                        int
	title, meta, score, header, side, tab                    textFace
}

// rpx scales a base pixel value by the metric scale (min 1).
func (m metrics) rpx(base float64) int { return scalePx(base, m.scale) }

func scalePx(base, scale float64) int {
	n := int(base*scale + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

func computeMetrics(scale float64) metrics {
	m := metrics{scale: scale}
	m.pad = scalePx(10, scale)
	m.title = getFace(scalePx(14, scale), false)
	m.meta = getFace(scalePx(11, scale), false)
	m.score = getFace(scalePx(14, scale), true)
	m.header = getFace(scalePx(15, scale), true)
	m.side = getFace(scalePx(13, scale), false)
	m.tab = getFace(scalePx(13, scale), true)
	m.scoreW = scalePx(56, scale)
	m.sidebarW = scalePx(170, scale)
	m.rowGap = scalePx(8, scale)
	m.tabPad = scalePx(10, scale)
	m.sideItemH = m.side.height + scalePx(10, scale)
	m.topbarH = m.header.height + 2*m.pad
	m.footerH = m.meta.height + m.pad
	m.rowH = 2*m.pad + 2*m.title.height + scalePx(4, scale) + m.meta.height
	return m
}

// --- helpers ---------------------------------------------------------------

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

// StepZoom moves zoom one step (+1 in, -1 out, 0 none), snapped to the grid.
func StepZoom(z float64, dir int) float64 {
	switch {
	case dir > 0:
		z += ZoomStep
	case dir < 0:
		z -= ZoomStep
	}
	z = float64(int(z/ZoomStep+0.5)) * ZoomStep
	return ClampZoom(z)
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

// wrapText greedily wraps text to at most maxLines lines no wider than maxW
// pixels in tf, hard-cutting over-long words and ellipsizing dropped content.
func wrapText(tf textFace, text string, maxW, maxLines int) []string {
	text = strings.TrimSpace(text)
	if text == "" || maxW < 1 || maxLines < 1 {
		return nil
	}
	words := strings.Fields(text)
	var lines []string
	line := ""
	for i := 0; i < len(words); {
		w := words[i]
		cand := w
		if line != "" {
			cand = line + " " + w
		}
		if tf.width(cand) <= maxW {
			line = cand
			i++
			continue
		}
		if line == "" {
			cut := hardCut(tf, w, maxW)
			line = cut
			words[i] = w[len(cut):]
		}
		lines = append(lines, line)
		line = ""
		if len(lines) == maxLines {
			return ellipsize(tf, lines, maxW)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// hardCut returns the largest rune-prefix of w that fits maxW (at least one).
func hardCut(tf textFace, w string, maxW int) string {
	r := []rune(w)
	for n := len(r); n >= 1; n-- {
		if tf.width(string(r[:n])) <= maxW {
			return string(r[:n])
		}
	}
	return string(r[:1])
}

// ellipsize trims the final line so it plus "…" fits maxW.
func ellipsize(tf textFace, lines []string, maxW int) []string {
	r := []rune(lines[len(lines)-1])
	for len(r) > 0 && tf.width(string(r)+"…") > maxW {
		r = r[:len(r)-1]
	}
	lines[len(lines)-1] = string(r) + "…"
	return lines
}

// mute blends fg 55% toward bg for secondary text.
func mute(fg, bg toolkit.RGBA) toolkit.RGBA {
	blend := func(a, b uint8) uint8 { return uint8((int(a)*45 + int(b)*55) / 100) }
	return toolkit.RGBA{R: blend(fg.R, bg.R), G: blend(fg.G, bg.G), B: blend(fg.B, bg.B), A: 0xFF}
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
