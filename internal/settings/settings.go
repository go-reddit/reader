// Package settings is the reader's persisted preferences: named profiles
// (tabs) that each group a subset of subreddits — so "Pro" and "Perso" feeds
// stay separate — plus the chosen sort and theme. The model is plain data and
// compiles everywhere (including GOOS=js), so the wasm front-end decodes the
// same types the host serves; file persistence is a thin JSON layer used
// host-side.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Profile is a named tab grouping a subset of subreddits. An empty feed entry
// ("") is the front page.
type Profile struct {
	Name  string   `json:"name"`
	Feeds []string `json:"feeds"`
}

// Theme options. "system" follows the OS/browser (and the per-OS native look).
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// Settings is the whole persisted preference set.
type Settings struct {
	Profiles []Profile `json:"profiles"`
	Active   int       `json:"active"` // index into Profiles
	Sort     string    `json:"sort"`   // hot|new|top|rising
	Theme    string    `json:"theme"`  // system|light|dark
}

// Default returns the seed settings: a single starter profile. Users create
// their own profiles (e.g. separating "Pro" and "Perso" subreddits) from the
// in-app Settings panel — nothing here is meant to be authoritative.
func Default() Settings {
	return Settings{
		Profiles: []Profile{
			{Name: "Home", Feeds: []string{"", "golang", "programming", "worldnews"}},
		},
		Active: 0,
		Sort:   "hot",
		Theme:  ThemeSystem,
	}
}

// ActiveProfile returns the currently selected profile, or a safe fallback
// when the index or list is out of range.
func (s Settings) ActiveProfile() Profile {
	if s.Active >= 0 && s.Active < len(s.Profiles) {
		return s.Profiles[s.Active]
	}
	if len(s.Profiles) > 0 {
		return s.Profiles[0]
	}
	return Profile{Name: "All"}
}

// normalize repairs an out-of-range active index, blank sort/theme, and an
// empty profile list, so callers always get a usable value.
func (s *Settings) normalize() {
	if len(s.Profiles) == 0 {
		s.Profiles = Default().Profiles
	}
	if s.Active < 0 || s.Active >= len(s.Profiles) {
		s.Active = 0
	}
	if s.Sort == "" {
		s.Sort = "hot"
	}
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = ThemeSystem
	}
}

// Load reads settings from path. A missing file yields [Default] (not an
// error); a present-but-corrupt file is a hard error so the user notices
// rather than silently losing their profiles.
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	s.normalize()
	return s, nil
}

// Save writes settings to path, creating the parent directory as needed.
func (s Settings) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

// DefaultPath is the per-user settings file location
// (~/Library/Application Support/go-reddit-reader/settings.json on macOS,
// $XDG_CONFIG_HOME/... on Linux, %AppData%\... on Windows).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "go-reddit-reader", "settings.json"), nil
}

// Store is a file-backed settings persister. It satisfies the small
// load/save surface the HTTP layer depends on, keeping the server package free
// of any path knowledge.
type Store struct{ Path string }

// NewStore returns a Store rooted at path.
func NewStore(path string) *Store { return &Store{Path: path} }

// Load reads the settings (or defaults when the file is absent).
func (s *Store) Load() (Settings, error) { return Load(s.Path) }

// Save persists v.
func (s *Store) Save(v Settings) error { return v.Save(s.Path) }

// SanitizeFeeds trims "r/" prefixes and whitespace and drops duplicates while
// preserving order; used when accepting edited profiles from the UI.
func SanitizeFeeds(feeds []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(feeds))
	for _, f := range feeds {
		f = strings.TrimPrefix(strings.TrimSpace(f), "r/")
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}
