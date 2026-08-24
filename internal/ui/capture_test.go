package ui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-reddit/reddit"
)

// TestCapture renders the three migrated zones (feed, settings, login) to PNG
// files under $CAPTURE_DIR when that variable is set, so a before/after diff of
// the widget migration can be inspected programmatically. It is a no-op in
// normal CI runs (CAPTURE_DIR unset), so it neither slows the suite nor asserts
// anything the other tests do not already cover.
func TestCapture(t *testing.T) {
	dir := os.Getenv("CAPTURE_DIR")
	if dir == "" {
		t.Skip("set CAPTURE_DIR to render before/after PNGs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A feed with mixed-script + emoji titles so text shaping (Arabic joining,
	// Devanagari reordering, CJK, ZWJ emoji) is exercised in the capture.
	posts := []reddit.Post{
		{Title: "Pure-Go rendering hits 60fps on the reference bench", Author: "gopher", NumComments: 128, Domain: "github.com", Score: 2400, Flair: "Show"},
		{Title: "مرحبا بالعالم — Arabic joins and shapes correctly now", Author: "shaper", NumComments: 42, Domain: "example.com", Score: 900},
		{Title: "नमस्ते दुनिया · Devanagari clusters reorder", Author: "indic", NumComments: 7, Domain: "example.org", Score: 88},
		{Title: "中文标题 with 日本語 and emoji 🧑‍🚀🎉", Author: "cjk", NumComments: 15, Domain: "reddit.com", Score: 1200},
	}

	render := func(name string, cfg func(*Scene)) {
		s := NewScene()
		s.SetProfiles([]settings.Profile{
			{Name: "Pro", Feeds: []string{"", "golang", "programming"}},
			{Name: "Perso", Feeds: []string{"pics", "worldnews"}},
		}, 0)
		s.SetPosts(posts)
		cfg(s)
		buf := make([]byte, s.W*s.H*4)
		s.Draw(buf)
		img := &image.RGBA{Pix: buf, Stride: s.W * 4, Rect: image.Rect(0, 0, s.W, s.H)}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	render("feed", func(s *Scene) {})
	render("feed_scroll", func(s *Scene) {
		many := make([]reddit.Post, 40)
		for i := range many {
			many[i] = reddit.Post{Title: "Scrollable post number " + itoa(i+1), Author: "user", NumComments: i, Domain: "example.com", Score: 100 + i}
		}
		s.SetPosts(many)
		s.Resize(700, 480)
		s.Scroll(600) // scroll down so the thumb sits mid-track
	})
	render("settings", func(s *Scene) { s.OpenSettings() })
	render("login", func(s *Scene) {
		s.OpenLogin()
		s.loginIDEntry.SetText("aBcD1234xyz")
		s.loginSecretEntry.SetText("s3cr3t-value")
		s.FocusLoginField(1)
	})
}
