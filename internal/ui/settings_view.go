package ui

import (
	"image"
	"strings"

	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// The in-canvas preferences editor. It is drawn with the same go-widgets
// painter + anti-aliased text as the rest of the app (no separate HTML page):
// theme and sort pickers, a profile switcher, and per-profile subreddit chips
// you remove with a click or add with the text field. Every clickable region
// is an sButton so hit-testing and drawing stay in lock-step.

type sButton struct {
	rect   toolkit.Rect
	label  string
	kind   HitKind
	value  string
	index  int
	active bool
	danger bool
}

// OpenSettings enters the preferences view, editing the active profile.
func (s *Scene) OpenSettings() {
	s.Mode = ModeSettings
	s.selEdit = s.Active
	s.input = ""
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
		s.search += string(r)
		s.ScrollY = 0
	default:
		s.input += string(r)
	}
}

// Backspace removes the last rune of the focused text field.
func (s *Scene) Backspace() {
	switch {
	case s.Mode == ModeLogin:
		s.loginBackspace()
	case s.Mode == ModeFeed && s.searchFocused:
		s.search = trimLastRune(s.search)
		s.ScrollY = 0
	default:
		s.input = trimLastRune(s.input)
	}
}

func trimLastRune(str string) string {
	rs := []rune(str)
	if len(rs) == 0 {
		return str
	}
	return string(rs[:len(rs)-1])
}

// Input returns the current add-subreddit text (for the front-end display).
func (s *Scene) Input() string { return s.input }

// AddInputFeed adds the typed subreddit to the edited profile and clears the
// field. Blank/duplicate entries are ignored.
func (s *Scene) AddInputFeed() {
	name := strings.TrimPrefix(strings.TrimSpace(s.input), "r/")
	s.input = ""
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

// layoutSettings computes every button + the input field rectangle.
func (s *Scene) layoutSettings() {
	s.m = computeMetrics(s.effScale())
	m := s.m
	s.sButtons = s.sButtons[:0]

	btnH := m.tab.height + m.rpx(8)
	gap := m.rpx(6)

	add := func(x, y int, label string, kind HitKind, value string, index int, active, danger bool) int {
		w := m.tab.width(label) + m.rpx(20)
		s.sButtons = append(s.sButtons, sButton{
			rect: toolkit.Rect{X: x, Y: y, W: w, H: btnH}, label: label,
			kind: kind, value: value, index: index, active: active, danger: danger,
		})
		return x + w + gap
	}

	// Rows flow top-to-bottom; each "section" advances y.
	y := m.topbarH + m.pad + m.side.height + gap // below the "Appearance" label

	// Theme buttons.
	x := m.pad
	for _, tn := range []string{"system", "light", "dark"} {
		x = add(x, y, titleCase(tn), HitTheme, tn, 0, s.ThemeName == tn, false)
	}
	y += btnH + m.pad

	// Sort buttons.
	y += m.side.height + gap // below the "Default sort" label
	x = m.pad
	for _, srt := range Sorts {
		x = add(x, y, srt, HitSort, srt, 0, s.Sort == srt, false)
	}
	y += btnH + m.pad*2

	// Profile switcher row (+ New).
	y += m.side.height + gap // below the "Profiles" label
	x = m.pad
	for i, p := range s.Profiles {
		x = add(x, y, p.Name, HitSelectProfile, "", i, i == s.selEdit, false)
	}
	x = add(x, y, "+ New", HitNewProfile, "", 0, false, false)
	y += btnH + m.pad

	// Edited-profile feed chips.
	if s.selEdit >= 0 && s.selEdit < len(s.Profiles) {
		x = m.pad
		for _, f := range s.Profiles[s.selEdit].Feeds {
			label := "Front page"
			if f != "" {
				label = "r/" + f
			}
			chipW := m.tab.width(label+"  ×") + m.rpx(20)
			if x+chipW > s.W-m.pad {
				x = m.pad
				y += btnH + gap
			}
			s.sButtons = append(s.sButtons, sButton{
				rect: toolkit.Rect{X: x, Y: y, W: chipW, H: btnH}, label: label + "  ×",
				kind: HitRemoveFeed, value: f, index: s.selEdit,
			})
			x += chipW + gap
		}
		y += btnH + m.pad

		// Add-subreddit input + button + delete-profile.
		s.sInputR = toolkit.Rect{X: m.pad, Y: y, W: m.rpx(220), H: btnH}
		bx := m.pad + s.sInputR.W + gap
		bx = add(bx, y, "Add", HitAddFeed, "", 0, false, false)
		add(bx, y, "Delete profile", HitDeleteProfile, "", s.selEdit, false, true)
	}
}

// drawSettings paints the preferences editor.
func (s *Scene) drawSettings(buf []byte) {
	s.layoutSettings()
	m := s.m
	p := painter.NewPixelPainter(buf, s.W, s.H)
	img := &image.RGBA{Pix: buf, Stride: s.W * 4, Rect: image.Rect(0, 0, s.W, s.H)}
	th := s.theme
	onAccent := th.Background
	if v, ok := th.Extra["OnAccent"]; ok {
		onAccent = v
	}
	muteS := mute(th.OnSurface, th.Surface)

	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)

	// Every body element is a stock go-widgets widget carrying the reader's
	// shaped fallback font — no hand-drawn pills / fields. Section captions are
	// toolkit.Label (muted ink), positioned to match layoutSettings' y flow.
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

	// Pills are toolkit.Button (Selected = active choice, ButtonDanger =
	// destructive); feed chips are toolkit.Chip with a Closable ✕ affordance.
	pillFont := ttFont(true, m.tab.px)
	for _, b := range s.sButtons {
		if b.kind == HitRemoveFeed {
			w := toolkit.NewChip(strings.TrimSuffix(b.label, "  ×"))
			w.Closable, w.Font = true, pillFont
			w.SetBounds(b.rect)
			w.Draw(p, th)
			continue
		}
		w := &toolkit.Button{Label: b.label, Selected: b.active}
		w.Font = pillFont
		if b.danger {
			w.Style = toolkit.ButtonDanger
		}
		w.SetBounds(b.rect)
		w.Draw(p, th)
	}

	// Add-subreddit input field is a generic toolkit.Entry (placeholder from the
	// toolkit's own Entry.Placeholder), focused since settings routes typed text
	// straight into it, with the caret parked at the end.
	if s.sInputR.W > 0 {
		w := &toolkit.Entry{Text: s.input, Placeholder: "add subreddit…", Cursor: len([]rune(s.input))}
		w.SetFocused(true)
		w.Font = pillFont
		w.SetBounds(s.sInputR)
		w.Draw(p, th)
	}

	// Topbar: title + Done (drawn last so it overpaints any overflow).
	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: m.topbarH}, th.Accent)
	m.header.draw(img, m.pad, (m.topbarH-m.header.height)/2, "Settings", onAccent)
	done := "Done"
	dw := m.tab.width(done) + m.rpx(24)
	dr := toolkit.Rect{X: s.W - m.pad - dw, Y: (m.topbarH - (m.tab.height + m.rpx(8))) / 2, W: dw, H: m.tab.height + m.rpx(8)}
	fillRoundBox(p, th, dr, m.rpx(6), onAccent)
	m.tab.draw(img, dr.X+m.rpx(12), dr.Y+(dr.H-m.tab.height)/2, done, th.Accent)
}

// hitSettings maps a click in the preferences view to an action.
func (s *Scene) hitSettings(x, y int) Hit {
	s.layoutSettings()
	// Done lives in the topbar; recompute its rect the same way drawSettings does.
	m := s.m
	dw := m.tab.width("Done") + m.rpx(24)
	done := toolkit.Rect{X: s.W - m.pad - dw, Y: (m.topbarH - (m.tab.height + m.rpx(8))) / 2, W: dw, H: m.tab.height + m.rpx(8)}
	if done.Contains(x, y) {
		return Hit{Kind: HitCloseSettings}
	}
	for _, b := range s.sButtons {
		if b.rect.Contains(x, y) {
			return Hit{Kind: b.kind, Value: b.value, Sort: b.value, Profile: b.index}
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
