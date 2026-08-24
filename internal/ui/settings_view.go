package ui

import (
	"strings"

	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// The in-canvas preferences editor. It is drawn with the same go-widgets
// painter + anti-aliased text as the rest of the app (no separate HTML page):
// theme and sort pickers, a profile switcher, and per-profile subreddit chips
// you remove with a click or add with the text field. Every clickable element is
// a PERSISTENT toolkit widget (a Button pill, a Chip, an Entry): layoutSettings
// positions them and records each in s.settingsHits, so hit-testing is a loop
// over the widgets' own HitTest — there is no parallel rect slice to drift out
// of sync with the draw.

// ensureButtons grows/truncates bs to exactly n reusable buttons, so the
// variable-length pill rows (profile switcher) keep persistent widgets across
// frames instead of allocating a fresh set every draw.
func ensureButtons(bs []*toolkit.Button, n int) []*toolkit.Button {
	for len(bs) < n {
		bs = append(bs, toolkit.NewButton("", nil))
	}
	return bs[:n]
}

// ensureChips grows/truncates cs to exactly n reusable closable chips.
func ensureChips(cs []*toolkit.Chip, n int) []*toolkit.Chip {
	for len(cs) < n {
		c := toolkit.NewChip("")
		c.Closable = true
		cs = append(cs, c)
	}
	return cs[:n]
}

// OpenSettings enters the preferences view, editing the active profile.
func (s *Scene) OpenSettings() {
	s.Mode = ModeSettings
	s.selEdit = s.Active
	s.settingsEntry.SetText("")
}

// CloseSettings returns to the feed view.
func (s *Scene) CloseSettings() { s.Mode = ModeFeed }

// SetThemeName records the chosen theme ("system"|"light"|"dark").
func (s *Scene) SetThemeName(name string) { s.ThemeName = name }

// SelectEdit picks which profile the editor operates on (clamped).
func (s *Scene) SelectEdit(i int) {
	if i >= 0 && i < len(s.Profiles) {
		s.selEdit = i
	}
}

// TypeRune appends a printable rune to whichever text field has focus (the
// topbar filter, the add-subreddit field in settings, or a login field).
func (s *Scene) TypeRune(r rune) {
	switch {
	case s.Mode == ModeLogin:
		s.loginTypeRune(r)
	case s.Mode == ModeFeed && s.searchFocused:
		s.searchEntry.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: string(r)})
		s.ScrollY = 0
	default:
		s.settingsEntry.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: string(r)})
	}
}

// Backspace removes the last rune of the focused text field.
func (s *Scene) Backspace() {
	switch {
	case s.Mode == ModeLogin:
		s.loginBackspace()
	case s.Mode == ModeFeed && s.searchFocused:
		s.searchEntry.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Backspace"})
		s.ScrollY = 0
	default:
		s.settingsEntry.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Backspace"})
	}
}

// Input returns the current add-subreddit text (for the front-end display).
func (s *Scene) Input() string { return s.settingsEntry.Value() }

// AddInputFeed adds the typed subreddit to the edited profile and clears the
// field. Blank/duplicate entries are ignored.
func (s *Scene) AddInputFeed() {
	name := strings.TrimPrefix(strings.TrimSpace(s.settingsEntry.Value()), "r/")
	s.settingsEntry.SetText("")
	if name == "" || s.selEdit < 0 || s.selEdit >= len(s.Profiles) {
		return
	}
	p := &s.Profiles[s.selEdit]
	for _, f := range p.Feeds {
		if f == name {
			return
		}
	}
	p.Feeds = append(p.Feeds, name)
}

// RemoveFeed drops feed from profile i.
func (s *Scene) RemoveFeed(i int, feed string) {
	if i < 0 || i >= len(s.Profiles) {
		return
	}
	out := s.Profiles[i].Feeds[:0]
	for _, f := range s.Profiles[i].Feeds {
		if f != feed {
			out = append(out, f)
		}
	}
	s.Profiles[i].Feeds = out
}

// NewProfile appends a fresh profile and selects it for editing.
func (s *Scene) NewProfile() {
	s.Profiles = append(s.Profiles, settings.Profile{Name: "Profile " + itoa(len(s.Profiles)+1)})
	s.selEdit = len(s.Profiles) - 1
}

// DeleteProfile removes profile i (never below one profile) and re-clamps the
// active + edited indices.
func (s *Scene) DeleteProfile(i int) {
	if i < 0 || i >= len(s.Profiles) || len(s.Profiles) <= 1 {
		return
	}
	s.Profiles = append(s.Profiles[:i], s.Profiles[i+1:]...)
	s.SetActive(s.Active)
	if s.selEdit >= len(s.Profiles) {
		s.selEdit = len(s.Profiles) - 1
	}
}

// Settings snapshots the editor state for persistence.
func (s *Scene) Settings() settings.Settings {
	return settings.Settings{Profiles: s.Profiles, Active: s.Active, Sort: s.Sort, Theme: s.ThemeName}
}

// settingsEditable reports whether a valid profile is being edited (so the feed
// chips + add-input + delete-profile block is shown / hit-tested).
func (s *Scene) settingsEditable() bool {
	return s.selEdit >= 0 && s.selEdit < len(s.Profiles)
}

// layoutSettings positions every persistent preferences widget and records the
// Hit each one's own HitTest resolves to (s.settingsHits). It flows the pill
// rows + chip grid top-to-bottom, matching the section captions drawSettings
// paints. No rectangles are stored: the geometry lives on the widgets.
func (s *Scene) layoutSettings() {
	toolkit.SetMetricScale(s.effScale())
	s.m = computeMetrics(s.effScale())
	m := s.m
	s.settingsHits = s.settingsHits[:0]

	btnH := m.tab.height + m.rpx(8)
	gap := m.rpx(6)
	pillFont := ttFont(true, m.tab.px)

	// place positions a button sized to its own label and registers its Hit.
	place := func(b *toolkit.Button, x, y int, hit Hit) int {
		b.Font = pillFont
		w := m.tab.width(b.Label().Get()) + m.rpx(20)
		b.SetBounds(toolkit.Rect{X: x, Y: y, W: w, H: btnH})
		s.settingsHits = append(s.settingsHits, widgetHit{w: b, hit: hit})
		return x + w + gap
	}

	// Done, pinned top-right in the topbar.
	dw := m.tab.width("Done") + m.rpx(24)
	s.doneBtn.Font = pillFont
	s.doneBtn.SetBounds(toolkit.Rect{X: s.W - m.pad - dw, Y: (m.topbarH - btnH) / 2, W: dw, H: btnH})
	s.settingsHits = append(s.settingsHits, widgetHit{w: s.doneBtn, hit: Hit{Kind: HitCloseSettings}})

	// Rows flow top-to-bottom; each "section" advances y.
	y := m.topbarH + m.pad + m.side.height + gap // below the "Appearance" label

	// Theme buttons.
	x := m.pad
	themeNames := []string{"system", "light", "dark"}
	for i, b := range s.themeButtons {
		b.Selected().Set(s.ThemeName == themeNames[i])
		x = place(b, x, y, Hit{Kind: HitTheme, Value: themeNames[i]})
	}
	y += btnH + m.pad

	// Sort buttons.
	y += m.side.height + gap // below the "Default sort" label
	x = m.pad
	for i, b := range s.sortButtons {
		b.Selected().Set(s.Sort == Sorts[i])
		x = place(b, x, y, Hit{Kind: HitSort, Sort: Sorts[i]})
	}
	y += btnH + m.pad*2

	// Profile switcher row (+ New).
	y += m.side.height + gap // below the "Profiles" label
	x = m.pad
	s.profileButtons = ensureButtons(s.profileButtons, len(s.Profiles))
	for i, b := range s.profileButtons {
		b.Label().Set(s.Profiles[i].Name)
		b.Selected().Set(i == s.selEdit)
		x = place(b, x, y, Hit{Kind: HitSelectProfile, Profile: i})
	}
	place(s.newProfileBtn, x, y, Hit{Kind: HitNewProfile})
	y += btnH + m.pad

	// Edited-profile feed chips + the add-input / Add / Delete-profile row.
	if !s.settingsEditable() {
		s.feedChips = s.feedChips[:0]
		return
	}
	feeds := s.Profiles[s.selEdit].Feeds
	s.feedChips = ensureChips(s.feedChips, len(feeds))
	x = m.pad
	for i, f := range feeds {
		label := "Front page"
		if f != "" {
			label = "r/" + f
		}
		c := s.feedChips[i]
		c.Text, c.Font = label, pillFont
		chipW := m.tab.width(label+"  ×") + m.rpx(20)
		if x+chipW > s.W-m.pad {
			x = m.pad
			y += btnH + gap
		}
		c.SetBounds(toolkit.Rect{X: x, Y: y, W: chipW, H: btnH})
		s.settingsHits = append(s.settingsHits, widgetHit{w: c, hit: Hit{Kind: HitRemoveFeed, Value: f, Profile: s.selEdit}})
		x += chipW + gap
	}
	y += btnH + m.pad

	// Add-subreddit input (always focused) + Add + Delete-profile.
	s.settingsEntry.Font = pillFont
	s.settingsEntry.SetBounds(toolkit.Rect{X: m.pad, Y: y, W: m.rpx(220), H: btnH})
	bx := m.pad + m.rpx(220) + gap
	bx = place(s.addBtn, bx, y, Hit{Kind: HitAddFeed})
	place(s.deleteBtn, bx, y, Hit{Kind: HitDeleteProfile, Profile: s.selEdit})
}

// drawSettings paints the preferences editor from its persistent widgets.
func (s *Scene) drawSettings(buf []byte) {
	s.layoutSettings()
	m := s.m
	p := painter.NewPixelPainter(buf, s.W, s.H)
	th := s.theme
	onAccent := th.Background
	if v, ok := th.Extra["OnAccent"]; ok {
		onAccent = v
	}
	muteS := mute(th.OnSurface, th.Surface)

	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)

	// Section captions are toolkit.Label (muted ink), positioned to match
	// layoutSettings' y flow.
	labelFont := ttFont(false, m.side.px)
	drawLabel := func(x, y int, text string) {
		w := toolkit.NewLabel(text)
		w.Font, w.Ink = labelFont, muteS
		w.SetBounds(toolkit.Rect{X: x, Y: y, W: s.W - x, H: m.side.height})
		w.Draw(p, th)
	}
	labelY := m.topbarH + m.pad
	drawLabel(m.pad, labelY, "APPEARANCE")
	sortLabelY := labelY + m.side.height + m.rpx(6) + (m.tab.height + m.rpx(8)) + m.pad
	drawLabel(m.pad, sortLabelY, "DEFAULT SORT")
	profLabelY := sortLabelY + m.side.height + m.rpx(6) + (m.tab.height + m.rpx(8)) + m.pad*2
	drawLabel(m.pad, profLabelY, "PROFILES")

	// Body pills + chips (persistent widgets positioned by layoutSettings).
	for _, b := range s.themeButtons {
		b.Draw(p, th)
	}
	for _, b := range s.sortButtons {
		b.Draw(p, th)
	}
	for _, b := range s.profileButtons {
		b.Draw(p, th)
	}
	s.newProfileBtn.Draw(p, th)
	if s.settingsEditable() {
		for _, c := range s.feedChips {
			c.Draw(p, th)
		}
		// The add-subreddit Entry is focused since settings routes typed text
		// straight into it through OnEvent.
		s.settingsEntry.SetFocused(true)
		s.settingsEntry.Draw(p, th)
		s.addBtn.Draw(p, th)
		s.deleteBtn.Draw(p, th)
	}

	// Topbar: title + Done (drawn last so it overpaints any overflow).
	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: m.topbarH}, th.Accent)
	m.header.labelAt(p, th, m.pad, (m.topbarH-m.header.height)/2, "Settings", onAccent)
	s.doneBtn.Draw(p, th)
}

// hitSettings maps a click in the preferences view to an action by asking each
// persistent widget's own HitTest — no rect math.
func (s *Scene) hitSettings(x, y int) Hit {
	s.layoutSettings()
	for _, wh := range s.settingsHits {
		if wh.w.HitTest(x, y) {
			return wh.hit
		}
	}
	return Hit{}
}

// --- tiny helpers (no strconv dependency in the hot path) ------------------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
