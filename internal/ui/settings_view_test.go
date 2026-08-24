package ui

import (
	"testing"

	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-widgets/toolkit"
)

func settingsScene() *Scene {
	s := NewScene()
	s.SetProfiles([]settings.Profile{
		{Name: "Pro", Feeds: []string{"golang", "rust"}},
		{Name: "Perso", Feeds: []string{"", "gaming"}},
	}, 0)
	s.OpenSettings()
	s.Resize(1200, 800)
	return s
}

func TestOpenCloseSettings(t *testing.T) {
	s := NewScene()
	s.Active = 1
	s.OpenSettings()
	if s.Mode != ModeSettings || s.selEdit != 1 || s.Input() != "" {
		t.Fatalf("OpenSettings => %+v", s)
	}
	s.CloseSettings()
	if s.Mode != ModeFeed {
		t.Error("CloseSettings")
	}
}

func TestSettingsMutations(t *testing.T) {
	s := settingsScene()
	s.SetThemeName("dark")
	if s.ThemeName != "dark" {
		t.Error("SetThemeName")
	}

	// SelectEdit clamps.
	s.SelectEdit(1)
	if s.selEdit != 1 {
		t.Error("SelectEdit valid")
	}
	s.SelectEdit(99)
	if s.selEdit != 1 {
		t.Error("SelectEdit oob should be ignored")
	}
	s.selEdit = 0

	// Typing + add.
	s.TypeRune('g')
	s.TypeRune('o')
	if s.Input() != "go" {
		t.Errorf("input = %q", s.Input())
	}
	s.Backspace()
	if s.Input() != "g" {
		t.Errorf("backspace => %q", s.Input())
	}
	s.settingsEntry.SetText("r/newsub")
	before := len(s.Profiles[0].Feeds)
	s.AddInputFeed()
	if len(s.Profiles[0].Feeds) != before+1 || s.Profiles[0].Feeds[before] != "newsub" || s.Input() != "" {
		t.Errorf("AddInputFeed => %v", s.Profiles[0].Feeds)
	}
	// Duplicate + blank + oob are ignored.
	s.settingsEntry.SetText("newsub")
	s.AddInputFeed()
	if len(s.Profiles[0].Feeds) != before+1 {
		t.Error("duplicate feed added")
	}
	s.settingsEntry.SetText("  ")
	s.AddInputFeed()
	s.settingsEntry.SetText("x")
	s.selEdit = 99
	s.AddInputFeed() // oob
	s.selEdit = 0

	// Backspace on empty input is a no-op.
	s.settingsEntry.SetText("")
	s.Backspace()

	// RemoveFeed.
	s.RemoveFeed(0, "newsub")
	if len(s.Profiles[0].Feeds) != before {
		t.Error("RemoveFeed")
	}
	s.RemoveFeed(99, "x")          // oob ignored
	s.RemoveFeed(0, "not-present") // no-op
}

func TestNewAndDeleteProfile(t *testing.T) {
	s := settingsScene()
	n := len(s.Profiles)
	s.NewProfile()
	if len(s.Profiles) != n+1 || s.selEdit != n {
		t.Fatalf("NewProfile => %d selEdit=%d", len(s.Profiles), s.selEdit)
	}
	if s.Profiles[n].Name == "" {
		t.Error("new profile unnamed")
	}
	// Delete it.
	s.DeleteProfile(n)
	if len(s.Profiles) != n {
		t.Errorf("DeleteProfile => %d", len(s.Profiles))
	}
	// oob + last-profile guards.
	s.DeleteProfile(99)
	one := &Scene{Profiles: []settings.Profile{{Name: "solo"}}}
	one.DeleteProfile(0)
	if len(one.Profiles) != 1 {
		t.Error("must keep at least one profile")
	}
	// Deleting when selEdit points past the shrunk list re-clamps.
	s2 := settingsScene()
	s2.NewProfile()
	s2.selEdit = len(s2.Profiles) - 1
	s2.DeleteProfile(0)
	if s2.selEdit >= len(s2.Profiles) {
		t.Error("selEdit not re-clamped after delete")
	}
}

func TestSettingsSnapshot(t *testing.T) {
	s := settingsScene()
	s.Sort = "top"
	s.ThemeName = "dark"
	got := s.Settings()
	if got.Sort != "top" || got.Theme != "dark" || len(got.Profiles) != len(s.Profiles) {
		t.Errorf("Settings() => %+v", got)
	}
}

func TestHitSettings(t *testing.T) {
	s := settingsScene()
	s.layoutSettings()

	// Done in the topbar.
	m := s.m
	dw := m.tab.width("Done") + m.rpx(24)
	if h := s.HitTest(s.W-m.pad-dw/2, m.topbarH/2); h.Kind != HitCloseSettings {
		t.Errorf("Done => %+v", h)
	}

	// Every button kind resolves via its rect centre.
	kinds := map[HitKind]bool{}
	for _, b := range s.sButtons {
		h := s.HitTest(b.rect.X+b.rect.W/2, b.rect.Y+b.rect.H/2)
		kinds[h.Kind] = true
		if h.Kind == HitRemoveFeed && h.Value == "" && b.value != "" {
			t.Errorf("remove-feed lost its value: %+v", h)
		}
	}
	for _, want := range []HitKind{HitTheme, HitSort, HitSelectProfile, HitNewProfile, HitRemoveFeed, HitAddFeed, HitDeleteProfile} {
		if !kinds[want] {
			t.Errorf("no button produced kind %d", want)
		}
	}

	// Empty space misses.
	if h := s.HitTest(s.W-m.pad, s.H-m.pad); h.Kind != HitNone {
		t.Errorf("empty space => %+v", h)
	}
}

func TestDrawSettingsSmoke(t *testing.T) {
	// Populated editor with a caret.
	s := settingsScene()
	s.settingsEntry.SetText("typing")
	buf := make([]byte, s.W*s.H*4)
	s.Draw(buf)
	if allZero(buf) {
		t.Fatal("settings draw empty")
	}

	// Empty input (placeholder path) + many feeds forcing chip wrap.
	s2 := NewScene()
	s2.OpenSettings()
	long := make([]string, 20)
	for i := range long {
		long[i] = "subreddit" + itoa(i)
	}
	s2.Profiles[0].Feeds = long
	s2.Resize(700, 900) // narrow -> chips wrap
	s2.Draw(make([]byte, s2.W*s2.H*4))

	// selEdit out of range skips the chips/input block.
	s3 := settingsScene()
	s3.selEdit = 99
	s3.Draw(make([]byte, s3.W*s3.H*4))

	// A theme carrying Extra["OnAccent"] exercises that colour path.
	s4 := settingsScene()
	s4.SetTheme(toolkit.AdwaitaDark()) // constructed with Extra["OnAccent"]
	s4.Draw(make([]byte, s4.W*s4.H*4))
}

func TestItoa(t *testing.T) {
	for in, want := range map[int]string{0: "0", 7: "7", 42: "42", 100: "100"} {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q", in, got)
		}
	}
}

func TestTitleCase(t *testing.T) {
	if titleCase("") != "" || titleCase("hot") != "Hot" || titleCase("a") != "A" {
		t.Error("titleCase")
	}
}

func TestResolveTheme(t *testing.T) {
	if ResolveTheme("light", OSMac, true) == nil {
		t.Error("light nil")
	}
	if ResolveTheme("dark", OSLinux, false) == nil {
		t.Error("dark nil")
	}
	if ResolveTheme("system", OSWindows, true) == nil {
		t.Error("system nil")
	}
}
