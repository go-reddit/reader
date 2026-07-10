package ui

import (
	"image"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/go-reddit/reddit"
	"github.com/go-widgets/toolkit"
)

func samplePosts(n int) []reddit.Post {
	posts := make([]reddit.Post, n)
	for i := range posts {
		posts[i] = reddit.Post{
			ID:          "id" + string(rune('a'+i)),
			Title:       "Post number " + strings.Repeat("word ", i%8+1),
			Author:      "user" + string(rune('a'+i)),
			Subreddit:   "golang",
			Domain:      "go.dev",
			Score:       (i + 1) * 137,
			NumComments: i * 3,
			Permalink:   "/r/golang/comments/id/x/",
			Flair:       map[bool]string{true: "News", false: ""}[i%2 == 0],
		}
	}
	return posts
}

func TestNewSceneDefaults(t *testing.T) {
	s := NewScene()
	if s.W != SurfaceW || s.H != SurfaceH || s.theme == nil || s.Sort != "hot" {
		t.Fatalf("unexpected defaults: %+v", s)
	}
}

func TestSetters(t *testing.T) {
	s := NewScene()
	s.SetTheme(nil) // ignored
	dark := toolkit.DefaultDark()
	s.SetTheme(dark)
	if s.theme != dark {
		t.Error("SetTheme(dark) not applied")
	}
	s.SetFeed("r/golang", "")
	if s.Subreddit != "golang" || s.Sort != "hot" {
		t.Errorf("SetFeed => %q %q", s.Subreddit, s.Sort)
	}
	s.SetFeed("rust", "top")
	if s.Subreddit != "rust" || s.Sort != "top" {
		t.Errorf("SetFeed => %q %q", s.Subreddit, s.Sort)
	}
	s.ScrollY = 999
	s.SetPosts(samplePosts(3))
	if len(s.Posts) != 3 || s.ScrollY != 0 {
		t.Errorf("SetPosts didn't reset: %d posts, scroll %d", len(s.Posts), s.ScrollY)
	}
}

func TestFeedLabel(t *testing.T) {
	s := NewScene()
	if got := s.feedLabel(); got != "front page · hot" {
		t.Errorf("front page label = %q", got)
	}
	s.SetFeed("golang", "new")
	if got := s.feedLabel(); got != "r/golang · new" {
		t.Errorf("sub label = %q", got)
	}
	if s.FeedName() != s.feedLabel() {
		t.Error("FeedName should mirror feedLabel")
	}
}

func TestScrollClamp(t *testing.T) {
	s := NewScene()
	s.SetPosts(samplePosts(40)) // tall content
	if changed := s.Scroll(-10); changed {
		t.Error("scroll up from 0 should not change")
	}
	if !s.Scroll(100) {
		t.Error("scroll down should change")
	}
	if s.ScrollY != 100 {
		t.Errorf("ScrollY = %d", s.ScrollY)
	}
	// Overscroll clamps to MaxScroll.
	s.Scroll(1 << 20)
	if s.ScrollY != s.MaxScroll() {
		t.Errorf("overscroll not clamped: %d vs max %d", s.ScrollY, s.MaxScroll())
	}
	// Short content: MaxScroll is 0.
	s.SetPosts(samplePosts(1))
	s.Scroll(50)
	if s.MaxScroll() != 0 || s.ScrollY != 0 {
		t.Errorf("short content should not scroll: max=%d y=%d", s.MaxScroll(), s.ScrollY)
	}
}

func TestHitTest(t *testing.T) {
	s := NewScene()
	s.SetPosts(samplePosts(5))
	// Header/footer are misses.
	if _, ok := s.HitTest(10, 5); ok {
		t.Error("header should be a miss")
	}
	if _, ok := s.HitTest(10, s.H-1); ok {
		t.Error("footer should be a miss")
	}
	// First row center hits post 0.
	post, ok := s.HitTest(s.W/2, headerH+pad+rowH/2)
	if !ok || post.ID != s.Posts[0].ID {
		t.Errorf("expected post 0, got %v ok=%v", post.ID, ok)
	}
	// The gap between rows is a miss.
	if _, ok := s.HitTest(s.W/2, headerH+pad+rowH+rowGap/2); ok {
		t.Error("row gap should be a miss")
	}
	// Outside the horizontal card is a miss.
	if _, ok := s.HitTest(2, headerH+pad+rowH/2); ok {
		t.Error("left margin should be a miss")
	}
	// After scrolling (with enough content to move), the top of the
	// viewport maps to a later post.
	tall := NewScene()
	tall.SetPosts(samplePosts(40))
	tall.Scroll(rowH + rowGap)
	post, ok = tall.HitTest(tall.W/2, headerH+pad+rowH/2)
	if !ok || post.ID != tall.Posts[1].ID {
		t.Errorf("after scroll expected post 1, got %v ok=%v", post.ID, ok)
	}
}

func TestFormatScore(t *testing.T) {
	cases := map[int]string{0: "0", 42: "42", 999: "999", 1200: "1.2k", 15000: "15.0k", 250000: "250k"}
	for in, want := range cases {
		if got := formatScore(in); got != want {
			t.Errorf("formatScore(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestWrapText(t *testing.T) {
	if got := wrapText("", 10, 2); got != nil {
		t.Errorf("empty => %v", got)
	}
	if got := wrapText("hello", 0, 2); got != nil {
		t.Errorf("zero width => %v", got)
	}
	// Fits on one line.
	if got := wrapText("hi there", 20, 2); len(got) != 1 || got[0] != "hi there" {
		t.Errorf("single line => %v", got)
	}
	// Wraps to two lines.
	got := wrapText("alpha beta gamma delta", 11, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %v", got)
	}
	// Overflow beyond maxLines gets an ellipsis.
	got = wrapText("alpha beta gamma delta epsilon zeta eta", 11, 2)
	if len(got) != 2 || !strings.HasSuffix(got[len(got)-1], "…") {
		t.Errorf("overflow should ellipsize: %v", got)
	}
	// A single word longer than the line is hard-cut across lines.
	got = wrapText("supercalifragilisticexpialidocious", 8, 3)
	if len(got) == 0 || len(got[0]) != 8 {
		t.Errorf("long word cut wrong: %v", got)
	}
	// Long word that overflows all lines ellipsizes.
	got = wrapText("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 5, 2)
	if len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("long word overflow: %v", got)
	}
	// Word overflow where the last partial word triggers truncateLast via
	// the trailing-cur path.
	got = wrapText("hello worldlonger", 6, 1)
	if len(got) != 1 || !strings.HasSuffix(got[0], "…") {
		t.Errorf("trailing overflow: %v", got)
	}
}

func TestMetaLine(t *testing.T) {
	if got := metaLine(reddit.Post{Author: "a", NumComments: 3, Domain: "x.com"}); got != "u/a  ·  3 comments  ·  x.com" {
		t.Errorf("meta = %q", got)
	}
	if got := metaLine(reddit.Post{Author: "a", NumComments: 0}); got != "u/a  ·  0 comments" {
		t.Errorf("meta without domain = %q", got)
	}
}

func TestMute(t *testing.T) {
	c := mute(toolkit.RGBA{R: 0, G: 0, B: 0}, toolkit.RGBA{R: 100, G: 100, B: 100})
	if c.A != 0xFF || c.R != 55 {
		t.Errorf("mute blend wrong: %+v", c)
	}
}

// TestDrawSmoke renders both a populated feed and the empty state without
// panicking, and (when READER_PNG is set) dumps a PNG for visual inspection.
func TestDrawSmoke(t *testing.T) {
	s := NewScene()
	s.SetFeed("golang", "hot")
	s.SetPosts(samplePosts(12))
	s.Status = "12 posts · r/golang"
	buf := make([]byte, s.W*s.H*4)
	s.Draw(buf)
	if allZero(buf) {
		t.Fatal("Draw produced an all-zero buffer")
	}
	dumpPNG(t, buf, s.W, s.H)

	// Empty state + dark theme + scrolled.
	e := NewScene()
	e.SetTheme(toolkit.DefaultDark())
	e.Draw(make([]byte, e.W*e.H*4))

	// Scrolled render exercises the off-viewport `continue` skip.
	s.Scroll(400)
	s.Draw(buf)

	// A GTK-sourced theme carrying Extra["OnAccent"] exercises the
	// header's explicit on-accent colour path.
	g := NewScene()
	th := toolkit.DefaultLight()
	th.Extra = map[string]toolkit.RGBA{"OnAccent": {R: 255, G: 255, B: 255, A: 255}}
	g.SetTheme(th)
	g.SetPosts(samplePosts(2))
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

func dumpPNG(t *testing.T, buf []byte, w, h int) {
	path := os.Getenv("READER_PNG")
	if path == "" {
		return
	}
	img := &image.RGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}
