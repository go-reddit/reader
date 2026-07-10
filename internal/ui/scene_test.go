package ui

import (
	"strings"
	"testing"

	"github.com/go-reddit/reader/internal/settings"
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
			Flair:       map[bool]string{true: "News", false: ""}[i%2 == 0],
		}
	}
	return posts
}

func sizedScene(n int) *Scene {
	s := NewScene()
	// Two profiles so the profile-tab tests have something to click.
	s.SetProfiles([]settings.Profile{
		{Name: "Pro", Feeds: []string{"golang", "rust", "kubernetes"}},
		{Name: "Perso", Feeds: []string{"", "gaming", "movies"}},
	}, 0)
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
	if len(s.Profiles) != 1 || s.Profiles[0].Name != "Home" || s.Active != 0 {
		t.Errorf("profiles = %+v active=%d", s.Profiles, s.Active)
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
	s.SetFeed("rust", "new")
	if s.Subreddit != "rust" || s.Sort != "new" {
		t.Errorf("=> %q %q", s.Subreddit, s.Sort)
	}
}

func TestSetProfilesAndActive(t *testing.T) {
	s := NewScene()
	profs := []settings.Profile{{Name: "A", Feeds: []string{"x"}}, {Name: "B", Feeds: []string{"y", "z"}}}
	s.SetProfiles(profs, 1)
	if s.Active != 1 || len(s.Profiles) != 2 {
		t.Fatalf("SetProfiles => active=%d n=%d", s.Active, len(s.Profiles))
	}
	if got := s.ActiveFeeds(); len(got) != 2 || got[0] != "y" {
		t.Errorf("ActiveFeeds => %v", got)
	}
	// Out-of-range active clamps to 0.
	s.SetActive(99)
	if s.Active != 0 {
		t.Errorf("oob active => %d", s.Active)
	}
	// ActiveFeeds with an out-of-range index (forced) returns nil.
	s.Active = 42
	if s.ActiveFeeds() != nil {
		t.Error("oob ActiveFeeds should be nil")
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
	s.Scroll(1 << 20)
	s.Resize(1600, 1200)
	if s.W != 1600 || s.H != 1200 {
		t.Fatalf("resize => %dx%d", s.W, s.H)
	}
	if s.ScrollY > s.MaxScroll() {
		t.Errorf("scroll %d > max %d", s.ScrollY, s.MaxScroll())
	}
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
	s.SetPosts(samplePosts(1))
	s.Scroll(50)
	if s.MaxScroll() != 0 || s.ScrollY != 0 {
		t.Errorf("short content scrolled: max=%d y=%d", s.MaxScroll(), s.ScrollY)
	}
}

func TestHitTestSort(t *testing.T) {
	s := sizedScene(5)
	tab := s.tabs[1]
	hit := s.HitTest(tab.rect.X+tab.rect.W/2, tab.rect.H/2)
	if hit.Kind != HitSort || hit.Sort != "new" {
		t.Fatalf("sort hit = %+v", hit)
	}
	if h := s.HitTest(s.W-2, s.m.topbarH/2); h.Kind != HitNone {
		t.Errorf("empty topbar => %+v", h)
	}
}

func TestHitTestProfile(t *testing.T) {
	s := sizedScene(5)
	pt := s.profTabs[1] // "Perso"
	hit := s.HitTest(pt.rect.X+pt.rect.W/2, pt.rect.Y+pt.rect.H/2)
	if hit.Kind != HitProfile || hit.Profile != 1 {
		t.Fatalf("profile hit = %+v", hit)
	}
}

func TestHitTestSettings(t *testing.T) {
	s := sizedScene(5)
	hit := s.HitTest(s.settingsR.X+10, s.settingsR.Y+s.settingsR.H/2)
	if hit.Kind != HitSettings {
		t.Fatalf("settings hit = %+v", hit)
	}
}

func TestHitTestFeed(t *testing.T) {
	s := sizedScene(5)
	item := s.side[2]
	hit := s.HitTest(item.rect.X+10, item.rect.Y+item.rect.H/2)
	if hit.Kind != HitFeed || hit.Feed != s.ActiveFeeds()[2] {
		t.Fatalf("feed hit = %+v", hit)
	}
	// The "FEEDS" header row (between profile tabs and the first item) misses.
	headerY := s.m.topbarH + s.m.profileTabH + s.m.sideItemH/2
	if h := s.HitTest(10, headerY); h.Kind != HitNone {
		t.Errorf("feeds header => %+v", h)
	}
}

func TestHitTestPost(t *testing.T) {
	s := sizedScene(40)
	x := s.m.sidebarW + s.m.pad + 20
	y := s.m.topbarH + s.rows[0].top + s.m.rowH/2
	hit := s.HitTest(x, y)
	if hit.Kind != HitPost || hit.Post.ID != s.Posts[0].ID {
		t.Fatalf("post hit = %+v", hit)
	}
	gapY := s.m.topbarH + s.rows[0].top + s.m.rowH + s.m.rowGap/2
	if h := s.HitTest(x, gapY); h.Kind != HitNone {
		t.Errorf("row gap => %+v", h)
	}
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
	s.Status = "14 posts"
	buf := make([]byte, s.W*s.H*4)
	s.Draw(buf)
	if allZero(buf) {
		t.Fatal("Draw produced an all-zero buffer")
	}
	s.Scroll(400)
	s.Draw(buf)

	// Empty + dark + a non-active profile selected.
	e := NewScene()
	e.SetTheme(toolkit.DefaultDark())
	e.SetActive(1)
	e.Resize(900, 660)
	e.Draw(make([]byte, e.W*e.H*4))

	// A theme carrying Extra["OnAccent"].
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

func TestThemeFor(t *testing.T) {
	for _, os := range []string{OSMac, OSLinux, OSWindows, "unknown"} {
		for _, dark := range []bool{false, true} {
			th := ThemeFor(os, dark)
			if th == nil {
				t.Fatalf("ThemeFor(%q,%v) nil", os, dark)
			}
			// Constructed palettes must carry an OnAccent for the topbar text.
			if os == OSLinux || os == OSWindows {
				if _, ok := th.Extra["OnAccent"]; !ok {
					t.Errorf("%q dark=%v missing OnAccent", os, dark)
				}
			}
		}
	}
}

func TestRGB(t *testing.T) {
	c := rgb(0x3584E4)
	if c.R != 0x35 || c.G != 0x84 || c.B != 0xE4 || c.A != 0xFF {
		t.Errorf("rgb => %+v", c)
	}
}

func TestScalePxAndRpx(t *testing.T) {
	if scalePx(100, 2) != 200 {
		t.Error("scalePx basic")
	}
	if scalePx(1, 0.1) != 1 {
		t.Error("scalePx min clamp")
	}
	if computeMetrics(1.0).rpx(6) != 6 {
		t.Error("rpx@1")
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
		t.Error("step clamps")
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
	if wrapText(tf, "", 100, 2) != nil || wrapText(tf, "hi", 0, 2) != nil || wrapText(tf, "hi", 100, 0) != nil {
		t.Error("degenerate inputs")
	}
	if got := wrapText(tf, "hi there", 400, 2); len(got) != 1 {
		t.Errorf("one line => %v", got)
	}
	got := wrapText(tf, strings.Repeat("wordy ", 40), 120, 2)
	if len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("overflow => %v", got)
	}
	if got := wrapText(tf, "supercalifragilisticexpialidocious", 40, 3); len(got) == 0 {
		t.Errorf("long word => %v", got)
	}
	got = wrapText(tf, strings.Repeat("x", 200), 30, 2)
	if len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("long-word overflow => %v", got)
	}
	if got := wrapText(tf, "alpha beta gamma", 60, 3); len(got) < 1 {
		t.Errorf("wrap => %v", got)
	}
}

func TestHardCut(t *testing.T) {
	if got := hardCut(getFace(14, false), "wide", 1); len([]rune(got)) != 1 {
		t.Errorf("hardCut min one rune => %q", got)
	}
}

func TestMute(t *testing.T) {
	c := mute(toolkit.RGBA{R: 0}, toolkit.RGBA{R: 100})
	if c.A != 0xFF || c.R != 55 {
		t.Errorf("mute => %+v", c)
	}
}

func TestTextFaceHelpers(t *testing.T) {
	tf := getFace(20, true)
	if tf.width("") != 0 || tf.width("Wm") <= 0 || tf.height <= 0 || tf.ascent <= 0 {
		t.Errorf("face metrics: %+v", tf)
	}
	if getFace(20, true) != tf {
		t.Error("face not cached")
	}
	if getFace(0, false).height <= 0 {
		t.Error("zero px face")
	}
}

func TestHitTestAccount(t *testing.T) {
	s := sizedScene(5)
	// Logged out -> HitOpenLogin.
	if h := s.HitTest(10, s.accountR.Y+s.accountR.H/2); h.Kind != HitOpenLogin {
		t.Errorf("logged-out account => %+v", h)
	}
	// Logged in -> HitLogout.
	s.LoggedIn = true
	if h := s.HitTest(10, s.accountR.Y+s.accountR.H/2); h.Kind != HitLogout {
		t.Errorf("logged-in account => %+v", h)
	}
	// Settings entry still resolves.
	if h := s.HitTest(10, s.settingsR.Y+s.settingsR.H/2); h.Kind != HitSettings {
		t.Errorf("settings => %+v", h)
	}
}

func TestDrawLoggedInSidebar(t *testing.T) {
	s := sizedScene(3)
	s.LoggedIn = true // exercises the "Log out" label branch
	s.Draw(make([]byte, s.W*s.H*4))
}
