package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefault(t *testing.T) {
	d := Default()
	if len(d.Profiles) != 1 || d.Profiles[0].Name != "Home" {
		t.Fatalf("default profiles = %+v", d.Profiles)
	}
	if d.Active != 0 || d.Sort != "hot" || d.Theme != ThemeSystem {
		t.Errorf("default scalars = %+v", d)
	}
}

func TestActiveProfile(t *testing.T) {
	s := Settings{Profiles: []Profile{{Name: "Pro"}, {Name: "Perso"}}}
	if s.ActiveProfile().Name != "Pro" {
		t.Error("active 0 should be Pro")
	}
	s.Active = 1
	if s.ActiveProfile().Name != "Perso" {
		t.Error("active 1 should be Perso")
	}
	// Out of range falls back to the first profile.
	s.Active = 99
	if s.ActiveProfile().Name != "Pro" {
		t.Error("oob active should fall back to first")
	}
	// Empty list falls back to a synthetic profile.
	empty := Settings{}
	if empty.ActiveProfile().Name != "All" {
		t.Errorf("empty => %+v", empty.ActiveProfile())
	}
}

func TestNormalize(t *testing.T) {
	s := Settings{Active: -5}
	s.normalize()
	if len(s.Profiles) == 0 || s.Active != 0 || s.Sort != "hot" || s.Theme != ThemeSystem {
		t.Errorf("normalize => %+v", s)
	}
	// A valid theme is preserved.
	s2 := Settings{Profiles: []Profile{{Name: "x"}}, Theme: ThemeDark, Sort: "top", Active: 0}
	s2.normalize()
	if s2.Theme != ThemeDark || s2.Sort != "top" {
		t.Errorf("valid values changed: %+v", s2)
	}
	// An out-of-range active with a non-empty list clamps to 0.
	s3 := Settings{Profiles: []Profile{{Name: "a"}, {Name: "b"}}, Active: 7}
	s3.normalize()
	if s3.Active != 0 {
		t.Errorf("active clamp => %d", s3.Active)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Profiles) != 1 {
		t.Errorf("missing file should give defaults, got %+v", s)
	}
}

func TestLoadReadError(t *testing.T) {
	// A directory path is not a readable file, and it exists (not IsNotExist).
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Error("reading a directory should error")
	}
}

func TestLoadCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(p, []byte("{not json"), 0o600)
	if _, err := Load(p); err == nil {
		t.Error("corrupt file should error")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "settings.json")
	in := Settings{
		Profiles: []Profile{{Name: "Work", Feeds: []string{"golang", "sre"}}},
		Active:   0, Sort: "top", Theme: ThemeDark,
	}
	if err := in.Save(p); err != nil {
		t.Fatal(err)
	}
	out, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in.Profiles, out.Profiles) || out.Sort != "top" || out.Theme != ThemeDark {
		t.Errorf("round trip: in=%+v out=%+v", in, out)
	}
}

func TestSaveMkdirError(t *testing.T) {
	// Put a file where a parent directory would need to be created.
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o600)
	if err := (Settings{}).Save(filepath.Join(blocker, "settings.json")); err == nil {
		t.Error("mkdir over a file should error")
	}
}

func TestSaveOpenError(t *testing.T) {
	// A path that is an existing directory can't be opened for writing.
	dir := t.TempDir()
	if err := (Settings{}).Save(dir); err == nil {
		t.Error("saving onto a directory should error")
	}
}

func TestDefaultPathError(t *testing.T) {
	// Clear every var os.UserConfigDir consults so it fails on this platform.
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "AppData"} {
		t.Setenv(k, "")
	}
	if _, err := DefaultPath(); err == nil {
		t.Skip("UserConfigDir still resolved on this platform")
	}
}

func TestSaveNoDirPart(t *testing.T) {
	// A bare filename (dir "."): Save must still work.
	d := t.TempDir()
	cwd, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(cwd)
	if err := (Settings{Sort: "hot"}).Save("bare.json"); err != nil {
		t.Fatalf("save bare filename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d, "bare.json")); err != nil {
		t.Error("bare file not written")
	}
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "settings.json" || !filepath.IsAbs(p) {
		t.Errorf("DefaultPath = %q", p)
	}
}

func TestStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	st := NewStore(p)
	// Missing file loads defaults.
	got, err := st.Load()
	if err != nil || len(got.Profiles) != 1 {
		t.Fatalf("store load default: %+v %v", got, err)
	}
	got.Sort = "top"
	if err := st.Save(got); err != nil {
		t.Fatal(err)
	}
	back, err := st.Load()
	if err != nil || back.Sort != "top" {
		t.Fatalf("store round trip: %+v %v", back, err)
	}
}

func TestSanitizeFeeds(t *testing.T) {
	got := SanitizeFeeds([]string{"r/golang", " rust ", "golang", "", "r/golang"})
	want := []string{"golang", "rust", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SanitizeFeeds => %v, want %v", got, want)
	}
}
