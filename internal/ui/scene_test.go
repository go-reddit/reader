package ui

import (
	"strings"
	"testing"

	"github.com/go-reddit/reddit"
	"github.com/go-widgets/toolkit"
)

func samplePosts(n int) []reddit.Post {
	posts := make([]reddit.Post, n)
	for i := range posts {
		posts[i] = reddit.Post{
			ID:          "id" + string(rune('a'+i%26)),
			Title:       "Post " + strings.Repeat("word ", i%9+1),
			Author:      "user" + string(rune('a'+i%26)),
			Subreddit:   "golang",
			Domain:      map[bool]string{true: "go.dev", false: ""}[i%3 != 0],
			Score:       (i + 1) * 137,
			NumComments: i * 3,
			Permalink:   "/r/golang/comments/id/x/",
			Flair:       map[bool]string{true: "News", false: ""}[i%2 == 0],
		}
	}
	return posts
}

// sizedScene returns a laid-out scene of a comfortable size with n posts.
func sizedScene(n int) *Scene {
	s := NewScene()
	s.SetFeed("golang", "top")
	s.SetPosts(samplePosts(n))
	s.Resize(1000, 700)
	return s
}

func TestNewSceneDefaults(t *testing.T) {
	s := NewScene()
	if s.W != 900 || s.H != 660 || s.theme == nil || s.Sort != "hot" || s.Scale != 1 {
		t.Fatalf("defaults: %+v", s)
	}
	if len(s.Feeds) != len(DefaultFeeds) {
		t.Errorf("Feeds = %v", s.Feeds)
	}
}

func TestSetTheme(t *testing.T) {
	s := NewScene()
	s.SetTheme(nil)
	if s.theme == nil {
		t.Fatal("nil theme cleared the palette")
	}
	d := toolkit.DefaultDark()
	s.SetTheme(d)
	if s.theme != d {
		t.Error("SetTheme not applied")
	}
}

func TestSetFeed(t *testing.T) {
	s := NewScene()
	s.SetFeed("r/golang", "")
	if s.Subreddit != "golang" || s.Sort != "hot" {
		t.Errorf("=> %q %q", s.Subreddit, s.Sort)
	}
	// A brand-new subreddit is appended to the sidebar.
	s.SetFeed("kubernetes", "new")
	if !contains(s.Feeds, "kubernetes") {
		t.Error("new subreddit not added to Feeds")
	}
	// An existing one is not duplicated.
	before := len(s.Feeds)
	s.SetFeed("kubernetes", "top")
	if len(s.Feeds) != before {
		t.Error("subreddit duplicated in Feeds")
	}
	// Front page ("") is never added.
	s.SetFeed("", "hot")
	for _, f := range s.Feeds {
		if f == "" && countEmpty(s.Feeds) > 1 {
			t.Error("front page duplicated")
		}
	}
}

func countEmpty(ss []string) (n int) {
	for _, s := range ss {
		if s == "" {
			n++
		}
	}
	return n
}

func TestSetPostsResetsScroll(t *testing.T) {
	s := sizedScene(40)
	s.Scroll(500)
	s.SetPosts(samplePosts(3))
	if s.ScrollY != 0 || len(s.Posts) != 3 {
		t.Errorf("SetPosts: scroll=%d posts=%d", s.ScrollY, len(s.Posts))
	}
}

func TestFeedName(t *testing.T) {
	s := NewScene()
	if s.FeedName() != "Front page · hot" {
		t.Errorf("front = %q", s.FeedName())
	}
	s.SetFeed("golang", "new")
	if s.FeedName() != "r/golang · new" {
		t.Errorf("sub = %q", s.FeedName())
	}
}

func TestEffScale(t *testing.T) {
	s := NewScene()
	s.Scale = 0
	if s.effScale() != 1 {
		t.Error("zero scale should default to 1")
	}
	s.Scale = 2.5
	if s.effScale() != 2.5 {
		t.Error("positive scale should pass through")
	}
}

func TestResizeClampAndScroll(t *testing.T) {
	s := sizedScene(60)
	s.Scroll(1 << 20) // to the bottom
	s.Resize(1600, 1200)
	if s.W != 1600 || s.H != 1200 {
		t.Fatalf("resize => %dx%d", s.W, s.H)
	}
	if s.ScrollY > s.MaxScroll() {
		t.Errorf("scroll %d > max %d", s.ScrollY, s.MaxScroll())
	}
	// Clamp to the minimum surface.
	s.Resize(10, 10)
	if s.W != MinW || s.H != MinH {
		t.Errorf("min clamp => %dx%d", s.W, s.H)
	}
}

func TestScroll(t *testing.T) {
	s := sizedScene(60)
	if s.Scroll(-5) {
		t.Error("scroll up from 0 should not move")
	}
	if !s.Scroll(120) || s.ScrollY != 120 {
		t.Errorf("scroll down => %d", s.ScrollY)
	}
	s.Scroll(1 << 20)
	if s.ScrollY != s.MaxScroll() {
		t.Errorf("overscroll not clamped: %d vs %d", s.ScrollY, s.MaxScroll())
	}
	// Short content cannot scroll.
	s.SetPosts(samplePosts(1))
	s.Scroll(50)
	if s.MaxScroll() != 0 || s.ScrollY != 0 {
		t.Errorf("short content scrolled: max=%d y=%d", s.MaxScroll(), s.ScrollY)
	}
}

func TestHitTestSort(t *testing.T) {
	s := sizedScene(5)
	tab := s.tabs[1] // "new"
	hit := s.HitTest(tab.rect.X+tab.rect.W/2, tab.rect.H/2)
	if hit.Kind != HitSort || hit.Sort != "new" {
		t.Fatalf("sort hit = %+v", hit)
	}
	// Empty topbar space (far right, past the tabs) is a miss.
	if h := s.HitTest(s.W-2, s.m.topbarH/2); h.Kind != HitNone {
		t.Errorf("empty topbar => %+v", h)
	}
}

func TestHitTestFeed(t *testing.T) {
	s := sizedScene(5)
	item := s.side[2] // third bookmark
	hit := s.HitTest(item.rect.X+10, item.rect.Y+item.rect.H/2)
	if hit.Kind != HitFeed || hit.Feed != s.Feeds[2] {
		t.Fatalf("feed hit = %+v (want %q)", hit, s.Feeds[2])
	}
	// The "FEEDS" header row (above the first item) is a miss.
	if h := s.HitTest(10, s.m.topbarH+s.m.sideItemH/2); h.Kind != HitNone {
		t.Errorf("feeds header => %+v", h)
	}
}

func TestHitTestPost(t *testing.T) {
	s := sizedScene(40) // enough content to be scrollable
	// Centre of the first post row.
	x := s.m.sidebarW + s.m.pad + 20
	y := s.m.topbarH + s.rows[0].top + s.m.rowH/2
	hit := s.HitTest(x, y)
	if hit.Kind != HitPost || hit.Post.ID != s.Posts[0].ID {
		t.Fatalf("post hit = %+v", hit)
	}
	// The gap between rows is a miss.
	gapY := s.m.topbarH + s.rows[0].top + s.m.rowH + s.m.rowGap/2
	if h := s.HitTest(x, gapY); h.Kind != HitNone {
		t.Errorf("row gap => %+v", h)
	}
	// After scrolling, the same screen point maps to a later post.
	s.Scroll(s.m.rowH + s.m.rowGap)
	hit = s.HitTest(x, s.m.topbarH+s.rows[0].top+s.m.rowH/2)
	if hit.Kind != HitPost || hit.Post.ID != s.Posts[1].ID {
		t.Errorf("post hit after scroll = %+v", hit)
	}
}

func TestHitTestFooterIsMiss(t *testing.T) {
	s := sizedScene(5)
	if h := s.HitTest(s.W/2, s.H-1); h.Kind != HitNone {
		t.Errorf("footer => %+v", h)
	}
}

func TestDrawSmoke(t *testing.T) {
	s := sizedScene(14)
	s.Status = "14 posts · r/golang"
	buf := make([]byte, s.W*s.H*4)
	s.Draw(buf)
	if allZero(buf) {
		t.Fatal("Draw produced an all-zero buffer")
	}
	// Scrolled render exercises the off-viewport skip.
	s.Scroll(400)
	s.Draw(buf)

	// Empty + dark theme.
	e := NewScene()
	e.SetTheme(toolkit.DefaultDark())
	e.Resize(900, 660)
	e.Draw(make([]byte, e.W*e.H*4))

	// Theme carrying Extra["OnAccent"] exercises that colour path.
	g := sizedScene(2)
	th := toolkit.DefaultLight()
	th.Extra = map[string]toolkit.RGBA{"OnAccent": {R: 255, G: 255, B: 255, A: 255}}
	g.SetTheme(th)
	g.Draw(make([]byte, g.W*g.H*4))
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func TestScalePxAndRpx(t *testing.T) {
	if scalePx(100, 2) != 200 {
		t.Error("scalePx basic")
	}
	if scalePx(1, 0.1) != 1 { // rounds to 0 -> clamped to 1
		t.Error("scalePx min clamp")
	}
	m := computeMetrics(1.0)
	if m.rpx(6) != 6 {
		t.Errorf("rpx(6)@1 = %d", m.rpx(6))
	}
}

func TestClampZoom(t *testing.T) {
	if ClampZoom(0.1) != MinZoom || ClampZoom(9) != MaxZoom || ClampZoom(1.5) != 1.5 {
		t.Error("ClampZoom")
	}
}

func TestStepZoom(t *testing.T) {
	if got := StepZoom(1.0, +1); got < 1.09 || got > 1.11 {
		t.Errorf("zoom in => %v", got)
	}
	if got := StepZoom(1.0, -1); got < 0.89 || got > 0.91 {
		t.Errorf("zoom out => %v", got)
	}
	if StepZoom(1.5, 0) != 1.5 {
		t.Error("no-op step")
	}
	if StepZoom(MaxZoom, +1) != MaxZoom || StepZoom(MinZoom, -1) != MinZoom {
		t.Error("step clamps at extremes")
	}
}

func TestFormatScore(t *testing.T) {
	for in, want := range map[int]string{0: "0", 42: "42", 999: "999", 1200: "1.2k", 15000: "15.0k", 250000: "250k"} {
		if got := formatScore(in); got != want {
			t.Errorf("formatScore(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestMetaLine(t *testing.T) {
	if got := metaLine(reddit.Post{Author: "a", NumComments: 3, Domain: "x.com"}); got != "u/a  ·  3 comments  ·  x.com" {
		t.Errorf("meta = %q", got)
	}
	if got := metaLine(reddit.Post{Author: "a"}); got != "u/a  ·  0 comments" {
		t.Errorf("meta no domain = %q", got)
	}
}

func TestWrapText(t *testing.T) {
	tf := getFace(14, false)
	if wrapText(tf, "", 100, 2) != nil {
		t.Error("empty text")
	}
	if wrapText(tf, "hi", 0, 2) != nil {
		t.Error("zero width")
	}
	if wrapText(tf, "hi", 100, 0) != nil {
		t.Error("zero lines")
	}
	// Short text fits one line.
	if got := wrapText(tf, "hi there", 400, 2); len(got) != 1 {
		t.Errorf("one line => %v", got)
	}
	// Long text wraps to two, ellipsized when it overflows.
	got := wrapText(tf, strings.Repeat("wordy ", 40), 120, 2)
	if len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("overflow => %v", got)
	}
	// A single word wider than the line is hard-cut.
	got = wrapText(tf, "supercalifragilisticexpialidocious", 40, 3)
	if len(got) == 0 {
		t.Fatalf("long word => %v", got)
	}
	// A word that overflows every line ellipsizes the last.
	got = wrapText(tf, strings.Repeat("x", 200), 30, 2)
	if len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("long-word overflow => %v", got)
	}
	// Multi-line without overflow returns exactly the used lines.
	got = wrapText(tf, "alpha beta gamma", 60, 3)
	if len(got) < 1 {
		t.Errorf("wrap => %v", got)
	}
}

func TestHardCut(t *testing.T) {
	tf := getFace(14, false)
	// Always returns at least one rune, even when nothing fits.
	if got := hardCut(tf, "wide", 1); len([]rune(got)) != 1 {
		t.Errorf("hardCut min one rune => %q", got)
	}
}

func TestMute(t *testing.T) {
	c := mute(toolkit.RGBA{R: 0}, toolkit.RGBA{R: 100})
	if c.A != 0xFF || c.R != 55 {
		t.Errorf("mute => %+v", c)
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b"}, "b") || contains([]string{"a"}, "z") {
		t.Error("contains")
	}
}

func TestTextFaceHelpers(t *testing.T) {
	tf := getFace(20, true)
	if tf.width("") != 0 {
		t.Error("empty width should be 0")
	}
	if tf.width("Wm") <= 0 || tf.height <= 0 || tf.ascent <= 0 {
		t.Errorf("face metrics: %+v", tf)
	}
	// Second call is cached (same face value).
	if getFace(20, true) != tf {
		t.Error("face not cached")
	}
	// Zero/negative px is clamped to a usable face.
	if getFace(0, false).height <= 0 {
		t.Error("zero px face")
	}
}
