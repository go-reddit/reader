package ui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// The in-canvas Reddit login form (go-widgets, no HTML). It collects a Reddit
// application's client id + secret (from reddit.com/prefs/apps); submitting
// runs the host-side Touch ID prompt and stores them in the Keychain. Two
// focusable Entry fields, a submit and a cancel Button — all persistent widgets;
// loginLayout positions them and records the Hit each one's HitTest resolves to
// (s.loginHits), so drawing and hit-testing share one geometry.

// OpenLogin enters the login view with empty fields.
func (s *Scene) OpenLogin() {
	s.Mode = ModeLogin
	s.loginIDEntry.SetText("")
	s.loginSecretEntry.SetText("")
	s.loginFocus, s.loginErr = 0, ""
}

// LoginCredentials returns the typed client id + secret.
func (s *Scene) LoginCredentials() (id, secret string) {
	return s.loginIDEntry.Value(), s.loginSecretEntry.Value()
}

// loginField returns the Entry that currently receives typed text.
func (s *Scene) loginField() *toolkit.Entry {
	if s.loginFocus == 1 {
		return s.loginSecretEntry
	}
	return s.loginIDEntry
}

// SetLoginError shows an error under the form (e.g. a cancelled Touch ID).
func (s *Scene) SetLoginError(msg string) { s.loginErr = msg }

// FocusLoginField selects which field receives typed text (0 id, 1 secret).
func (s *Scene) FocusLoginField(i int) {
	if i == 0 || i == 1 {
		s.loginFocus = i
	}
}

// NextLoginField moves focus to the other field (Tab).
func (s *Scene) NextLoginField() { s.loginFocus = 1 - s.loginFocus }

func (s *Scene) loginTypeRune(r rune) {
	s.loginField().OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: string(r)})
}

func (s *Scene) loginBackspace() {
	s.loginField().OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Backspace"})
}

// loginLayout positions the persistent field + button widgets and records the
// Hit each one's HitTest resolves to (s.loginHits). No rectangles are stored —
// the field bounds live on the Entry widgets themselves.
func (s *Scene) loginLayout() {
	toolkit.SetMetricScale(s.effScale())
	s.m = computeMetrics(s.effScale())
	m := s.m
	s.loginHits = s.loginHits[:0]

	fieldW := m.rpx(360)
	if fieldW > s.W-2*m.pad {
		fieldW = s.W - 2*m.pad
	}
	rowH := m.tab.height + m.rpx(10)
	fieldFont := ttFont(true, m.tab.px)

	// Cancel, pinned top-right in the topbar.
	cw := m.tab.width("Cancel") + m.rpx(24)
	s.loginCancelBtn.Font = fieldFont
	s.loginCancelBtn.SetBounds(toolkit.Rect{X: s.W - m.pad - cw, Y: (m.topbarH - rowH) / 2, W: cw, H: rowH})
	s.loginHits = append(s.loginHits, widgetHit{w: s.loginCancelBtn, hit: Hit{Kind: HitLoginCancel}})

	// The two fields (persistent Entry widgets) then the submit button.
	y := m.topbarH + m.pad*3 + m.side.height // below the intro text
	s.loginIDEntry.Font = fieldFont
	s.loginIDEntry.SetBounds(toolkit.Rect{X: m.pad, Y: y + m.side.height, W: fieldW, H: rowH})
	s.loginHits = append(s.loginHits, widgetHit{w: s.loginIDEntry, hit: Hit{Kind: HitLoginField, Profile: 0}})

	y = s.loginIDEntry.Bounds().Y + rowH + m.pad + m.side.height
	s.loginSecretEntry.Font = fieldFont
	s.loginSecretEntry.SetBounds(toolkit.Rect{X: m.pad, Y: y, W: fieldW, H: rowH})
	s.loginHits = append(s.loginHits, widgetHit{w: s.loginSecretEntry, hit: Hit{Kind: HitLoginField, Profile: 1}})

	by := s.loginSecretEntry.Bounds().Y + rowH + m.pad
	bw := m.tab.width("Log in with Touch ID") + m.rpx(24)
	s.loginSubmitBtn.Font = fieldFont
	s.loginSubmitBtn.SetBounds(toolkit.Rect{X: m.pad, Y: by, W: bw, H: rowH})
	s.loginHits = append(s.loginHits, widgetHit{w: s.loginSubmitBtn, hit: Hit{Kind: HitLoginSubmit}})
}

// drawLogin paints the login form from its persistent widgets.
func (s *Scene) drawLogin(buf []byte) {
	s.loginLayout()
	m := s.m
	p := painter.NewPixelPainter(buf, s.W, s.H)
	th := s.theme
	onAccent := th.Background
	if v, ok := th.Extra["OnAccent"]; ok {
		onAccent = v
	}
	muteS := mute(th.OnSurface, th.Surface)

	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)

	// Intro.
	introY := m.topbarH + m.pad*2
	m.side.labelAt(p, th, m.pad, introY, "Enter your Reddit app's client id + secret (reddit.com/prefs/apps).", muteS)

	// Field captions are stock toolkit.Label above each persistent Entry (the
	// secret one masked by the toolkit's own Mask).
	labelFont := ttFont(false, m.side.px)
	drawCaption := func(label string, r toolkit.Rect) {
		lbl := toolkit.NewLabel(label)
		lbl.Font, lbl.Ink = labelFont, muteS
		lbl.SetBounds(toolkit.Rect{X: r.X, Y: r.Y - m.side.height - m.rpx(2), W: r.W, H: m.side.height})
		lbl.Draw(p, th)
	}
	drawCaption("CLIENT ID", s.loginIDEntry.Bounds())
	drawCaption("CLIENT SECRET", s.loginSecretEntry.Bounds())
	s.loginIDEntry.SetFocused(s.loginFocus == 0)
	s.loginIDEntry.Draw(p, th)
	s.loginSecretEntry.SetFocused(s.loginFocus == 1)
	s.loginSecretEntry.Draw(p, th)

	// Submit button (accent-primary style, set at construction).
	s.loginSubmitBtn.Draw(p, th)
	if s.loginErr != "" {
		b := s.loginSubmitBtn.Bounds()
		m.side.labelAt(p, th, m.pad, b.Y+b.H+m.pad, s.loginErr, rgb(0xD03030))
	}

	// Topbar + Cancel (drawn last).
	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: m.topbarH}, th.Accent)
	m.header.labelAt(p, th, m.pad, (m.topbarH-m.header.height)/2, "Log in to Reddit", onAccent)
	s.loginCancelBtn.Draw(p, th)
}

// hitLogin maps a click in the login view to an action by asking each persistent
// widget's own HitTest — no rect math.
func (s *Scene) hitLogin(x, y int) Hit {
	s.loginLayout()
	for _, wh := range s.loginHits {
		if wh.w.HitTest(x, y) {
			return wh.hit
		}
	}
	return Hit{}
}
